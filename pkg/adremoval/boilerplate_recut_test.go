package adremoval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/types"
	"pod/pkg/util"
)

const bpRead = "this show is brought to you by acme widgets the finest widgets money can buy visit acme dot com slash pod and use code pod for ten percent off your first order today"

func bpUnique(tag string, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("%s%d", tag, i)
	}
	return strings.Join(words, " ")
}

// bpEpisode writes a podcast folder holding one processed episode: a 110s
// transcript with the sponsor read at 40-70s, one earlier ad cut, and the
// recorded boilerplate. Nothing here is real audio, so only planning is tested.
func bpEpisode(t *testing.T) (dir, mp3, precut, base string) {
	t.Helper()
	dir = t.TempDir()
	cfg := config.PodcastConfig{Boilerplate: []config.BoilerplatePhrase{{Text: bpRead, Episodes: 9}}}
	if err := config.SavePodcastConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	mp3, base = filepath.Join(dir, "ep.mp3"), filepath.Join(dir, "ep")
	precut = mp3 + ".precut"
	td := types.TranscriptionData{Text: bpRead + " " + bpUnique("open", 40), Segments: []types.TranscriptionSegment{
		{Start: 0, End: 40, Text: bpUnique("open", 40)},
		{Start: 40, End: 70, Text: bpRead},
		{Start: 70, End: 110, Text: bpUnique("close", 40)},
	}}
	data, _ := json.Marshal(td)
	for path, content := range map[string][]byte{base + ".transcript.json": data, mp3: []byte("cut"), precut: []byte("original")} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	format.SaveCutsJSON(mp3, 110, []types.AdSegment{{Start: 5, End: 15, Reason: "earlier ad"}}, &types.LLMProfile{Name: "Model", Model: "m"}, true)
	return dir, mp3, precut, base
}

func TestPlanAddsTheBoilerplateAndKeepsEarlierCuts(t *testing.T) {
	t.Parallel()
	_, mp3, precut, base := bpEpisode(t)
	plan, skip := planBoilerplateRecut(mp3, precut, base, 110)
	if skip != "" || len(plan.cuts) != 1 {
		t.Fatalf("want one boilerplate cut, skipped=%q plan=%+v", skip, plan)
	}
	if plan.preview.AddedSec < 25 || plan.preview.AddedSec > 35 {
		t.Errorf("the read runs about 30s, got +%.0fs", plan.preview.AddedSec)
	}

	format.SaveDetectedCutsJSON(mp3, 110, plan.cuts, nil, true, false)
	data, _ := os.ReadFile(base + ".cuts.json")
	var saved types.CutsData
	_ = json.Unmarshal(data, &saved)
	if saved.LLMUsed != "Model (m)" {
		t.Errorf("the model that made the earlier cuts must stay on record, got %q", saved.LLMUsed)
	}
	if len(saved.MergedCutIntervals) != 2 {
		t.Errorf("the earlier ad cut and the boilerplate should both be cut, got %+v", saved.MergedCutIntervals)
	}
	if _, skip := planBoilerplateRecut(mp3, precut, base, 110); skip == "" {
		t.Error("a second refresh has nothing to add and must be skipped")
	}
}

func TestPlanRefusesWhatCouldNotBeRecutSafely(t *testing.T) {
	t.Parallel()
	cases := map[string]func(dir, mp3, precut, base string){
		"no cuts file":                  func(_, _, _, base string) { _ = os.Remove(base + ".cuts.json") },
		"no transcript":                 func(_, _, _, base string) { _ = os.Remove(base + ".transcript.json") },
		"original gone":                 func(_, mp3, precut, _ string) { _ = os.Remove(precut); markDone(t, mp3) },
		"no recorded boilerplate":       func(dir, _, _, _ string) { _ = config.SavePodcastConfig(dir, config.PodcastConfig{}) },
		"phrase disabled":               func(dir, _, _, _ string) { disableAll(t, dir) },
		"processing remotely":           func(_, mp3, _, _ string) { markStatus(t, mp3, "transcribing_remotely") },
		"transcript from the cut audio": nil,
	}
	for name, mutate := range cases {
		dir, mp3, precut, base := bpEpisode(t)
		duration := 110.0
		if mutate != nil {
			mutate(dir, mp3, precut, base)
		} else {
			duration = 400 // the audio is much longer than the transcript reaches
		}
		if _, skip := planBoilerplateRecut(mp3, precut, base, duration); skip == "" {
			t.Errorf("%s: should be skipped", name)
		}
	}
}

func markDone(t *testing.T, mp3 string) { markStatus(t, mp3, "done") }

func markStatus(t *testing.T, mp3, status string) {
	t.Helper()
	body := fmt.Sprintf(`{"version":1,"status":%q}`, status)
	if err := os.WriteFile(mp3+".json", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func disableAll(t *testing.T, dir string) {
	t.Helper()
	cfg := config.LoadPodcastConfig(dir, config.PodcastConfig{})
	for i := range cfg.Boilerplate {
		cfg.Boilerplate[i].Disabled = true
	}
	if err := config.SavePodcastConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestPlanIgnoresBoilerplateAlreadyInsideAnEarlierCut(t *testing.T) {
	t.Parallel()
	_, mp3, precut, base := bpEpisode(t)
	format.SaveCutsJSON(mp3, 110, []types.AdSegment{{Start: 39.5, End: 70.5, Reason: "ad the model found"}}, nil, true)
	if _, skip := planBoilerplateRecut(mp3, precut, base, 110); skip == "" {
		t.Error("boilerplate that an earlier cut already covers to within a second must not trigger a recut")
	}
}

func TestPlanRecutsAnUncutEpisodeWithoutAPrecut(t *testing.T) {
	t.Parallel()
	_, mp3, precut, base := bpEpisode(t)
	_ = os.Remove(precut)
	_ = os.Remove(base + ".cuts.json")
	format.SaveCutsJSON(mp3, 110, nil, nil, true)
	if err := os.WriteFile(base+".cuts.json", []byte(`{"version":1,"cut_intervals":[],"merged_cut_intervals":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	markDone(t, mp3)
	if _, skip := planBoilerplateRecut(mp3, precut, base, 110); skip != "" {
		t.Errorf("nothing was ever cut, so the audio is its own original; skipped: %s", skip)
	}
}

func TestRestoreCutsPutsBackTheOldFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ep.cuts.json")
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	restoreCuts(path, []byte("old"))
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Errorf("want the old cuts back, got %q", got)
	}
	restoreCuts(path, nil)
	if !util.FileExists(path) {
		t.Error("with nothing to restore the file must be left alone")
	}
}
