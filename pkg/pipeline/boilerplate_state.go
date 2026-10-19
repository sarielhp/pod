package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/util"
)

const (
	boilerplateStateFile = ".boilerplate_state.json"

	// boilerplateStateVersion invalidates every recorded result. Raise it when
	// the analysis or the recut planning changes what it concludes from the
	// same files.
	boilerplateStateVersion = 1
)

// boilerplateState is what a podcast directory remembers between runs, so that
// a run with nothing new can pass over it without reading its transcripts.
type boilerplateState struct {
	Version int `json:"version"`
	// Analyze fingerprints the inputs of the last analysis, and the boilerplate
	// it left in podcast.json.
	Analyze string `json:"analyze,omitempty"`
	// Recut maps an episode to the fingerprint of its files and the reason a
	// recut found nothing to do for them.
	Recut map[string]recutVerdict `json:"recut,omitempty"`
}

type recutVerdict struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

func loadBoilerplateState(dir string) boilerplateState {
	fresh := boilerplateState{Version: boilerplateStateVersion}
	data, err := os.ReadFile(filepath.Join(dir, boilerplateStateFile))
	if err != nil {
		return fresh
	}
	var st boilerplateState
	if json.Unmarshal(data, &st) != nil || st.Version != boilerplateStateVersion {
		return fresh
	}
	return st
}

func saveBoilerplateState(dir string, st boilerplateState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = util.WriteFileAtomicIfChanged(filepath.Join(dir, boilerplateStateFile), data, 0o644)
	return err
}

type fingerprint struct{ h hash.Hash }

func newFingerprint(parts ...string) fingerprint {
	f := fingerprint{h: sha256.New()}
	f.add(strconv.Itoa(boilerplateStateVersion))
	f.add(parts...)
	return f
}

func (f fingerprint) add(parts ...string) {
	for _, p := range parts {
		f.h.Write([]byte(p))
		f.h.Write([]byte{0})
	}
}

// addFile folds in a file's name, size and modification time, or that it is
// absent. Reading the file would defeat the purpose.
func (f fingerprint) addFile(path string) {
	fi, err := os.Stat(path)
	if err != nil {
		f.add(filepath.Base(path), "-")
		return
	}
	f.add(filepath.Base(path), strconv.FormatInt(fi.Size(), 10), strconv.FormatInt(fi.ModTime().UnixNano(), 10))
}

func (f fingerprint) sum() string { return hex.EncodeToString(f.h.Sum(nil)) }

func analysisFingerprint(transcripts []string, opts AnalyzeOptions, boilerplate []config.BoilerplatePhrase) string {
	f := newFingerprint("analyze", strconv.Itoa(opts.MinEpisodes), strconv.Itoa(opts.MinWords))
	sorted := append([]string(nil), transcripts...)
	sort.Strings(sorted)
	for _, path := range sorted {
		f.addFile(path)
	}
	recorded, _ := json.Marshal(boilerplate)
	f.add(string(recorded))
	return f.sum()
}

func recordAnalysis(dir string, transcripts []string, opts AnalyzeOptions, st boilerplateState) error {
	cfg, err := config.LoadPodcastConfigErr(dir, config.PodcastConfig{})
	if err != nil {
		return err
	}
	st.Analyze = analysisFingerprint(transcripts, opts, cfg.Boilerplate)
	return saveBoilerplateState(dir, st)
}

// RecutMemo remembers, per episode, that a boilerplate recut examined it and had
// nothing to do, so the next run can skip it while nothing it depends on has
// changed: the episode's files and the podcast's recorded boilerplate. Looking
// an episode up costs a few stat calls, where examining it means probing the
// audio and parsing its transcript.
type RecutMemo struct {
	states    map[string]*boilerplateState
	dirty     map[string]bool
	boilerHex map[string]string
}

func NewRecutMemo() *RecutMemo {
	return &RecutMemo{
		states:    map[string]*boilerplateState{},
		dirty:     map[string]bool{},
		boilerHex: map[string]string{},
	}
}

func (m *RecutMemo) state(dir string) *boilerplateState {
	if st, ok := m.states[dir]; ok {
		return st
	}
	st := loadBoilerplateState(dir)
	m.states[dir] = &st
	return &st
}

func (m *RecutMemo) boilerplateDigest(dir string) string {
	if digest, ok := m.boilerHex[dir]; ok {
		return digest
	}
	cfg, _ := config.LoadPodcastConfigErr(dir, config.PodcastConfig{})
	recorded, _ := json.Marshal(cfg.Boilerplate)
	sum := sha256.Sum256(recorded)
	digest := hex.EncodeToString(sum[:])
	m.boilerHex[dir] = digest
	return digest
}

// Key fingerprints everything a recut's decision about one episode depends on.
func (m *RecutMemo) Key(mainMP3, precut string) string {
	dir := filepath.Dir(mainMP3)
	base := util.StripExt(mainMP3)
	f := newFingerprint("recut", m.boilerplateDigest(dir))
	for _, path := range []string{mainMP3, precut, base + ".transcript.json", base + ".cuts.json", episode.StatusPathFor(mainMP3)} {
		f.addFile(path)
	}
	return f.sum()
}

func episodeStateName(mainMP3 string) string { return filepath.Base(util.StripExt(mainMP3)) }

// Settled reports the reason a recut found nothing to do for the episode last
// time, when its files still fingerprint to key.
func (m *RecutMemo) Settled(mainMP3, key string) (string, bool) {
	verdict, ok := m.state(filepath.Dir(mainMP3)).Recut[episodeStateName(mainMP3)]
	if !ok || verdict.Key != key {
		return "", false
	}
	return verdict.Reason, true
}

func (m *RecutMemo) Settle(mainMP3, key, reason string) {
	dir := filepath.Dir(mainMP3)
	st := m.state(dir)
	if st.Recut == nil {
		st.Recut = map[string]recutVerdict{}
	}
	st.Recut[episodeStateName(mainMP3)] = recutVerdict{Key: key, Reason: reason}
	m.dirty[dir] = true
}

// Save writes the podcasts that gained a verdict. A podcast that cannot be
// written does not stop the others; the first failure is returned.
func (m *RecutMemo) Save() error {
	var first error
	for dir := range m.dirty {
		if err := saveBoilerplateState(dir, *m.states[dir]); err != nil && first == nil {
			first = fmt.Errorf("%s: %w", strings.TrimSpace(dir), err)
		}
	}
	return first
}
