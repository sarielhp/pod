package podcast

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/types"
	"pod/pkg/util"
)

// renameLibrary builds one podcast folder: an episode with every kind of sidecar, a
// plain one, one with no identity, an orphaned transcript and a queue naming two of them.
func renameLibrary(t *testing.T) (PodcastDirEntry, backend.FeedEpisode, backend.FeedEpisode) {
	t.Helper()
	dir := t.TempDir()
	must(t, config.SavePodcastConfig(dir, config.PodcastConfig{ID: "shw01"}))
	a := feedEpisode("guid-a", "The Very Long Title of Episode A", pub)
	b := feedEpisode("guid-b", "Episode B", pub.AddDate(0, 0, -7))

	stemA := FormatEpisodeFilename(pub, "", a.Title)
	stemA = strings.TrimSuffix(stemA, ".mp3")
	for suffix, body := range map[string]string{
		".mp3": "cut audio", ".mp3.precut": "original audio", ".transcript.json": `{"text":"a"}`,
		".cuts.json": `{"cuts":[]}`, ".fp": "fingerprint", ".mp3.lock": "",
	} {
		writeFile(t, filepath.Join(dir, stemA+suffix), body)
	}
	audioA := filepath.Join(dir, stemA+".mp3")
	must(t, RecordFeedEpisode(audioA, a, "https://example.test/feed.xml"))
	must(t, episode.UpdateEpisodeStatus(audioA, func(st *types.EpisodeStatusFile) {
		st.MediaFile = stemA + ".mp3"
		st.Original.Filename = stemA + ".mp3.precut"
		st.Cleaned.Filename = stemA + ".mp3"
	}))

	stemB := strings.TrimSuffix(FormatEpisodeFilename(pub.AddDate(0, 0, -7), "", b.Title), ".mp3")
	audioB := writeAudio(t, dir, stemB+".mp3")
	writeFile(t, filepath.Join(dir, stemB+".transcript.json"), `{"text":"b"}`)
	must(t, RecordFeedEpisode(audioB, b, ""))

	writeAudio(t, dir, "no_identity.mp3")
	writeFile(t, filepath.Join(dir, "pruned_long_ago.transcript.json"), `{"text":"orphan"}`)
	must(t, episode.UpdateQueue(dir, func([]string) []string { return []string{stemA + ".mp3", "no_identity.mp3"} }))
	return PodcastDirEntry{Dir: dir, Title: "Show", ShortID: "shw01"}, a, b
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// snapshot maps every file under dir to a hash of its content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range listFileNames(dir) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		must(t, err)
		sum := sha256.Sum256(data)
		out[name] = hex.EncodeToString(sum[:8])
	}
	return out
}

func names(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestPlanRenamesIdentifiedEpisodesAndLeavesTheRest(t *testing.T) {
	t.Parallel()
	entry, _, _ := renameLibrary(t)
	plan := PlanEpisodeRenames(entry)
	if len(plan.Renames) != 2 || plan.Unidentified != 1 || plan.Orphans != 1 {
		t.Fatalf("want 2 renames, 1 unidentified, 1 orphan; got %+v", plan)
	}
	for _, r := range plan.Renames {
		if len(r.NewStem) != len("2026-09-28_")+10 || strings.Contains(r.NewStem, "Title") {
			t.Errorf("new stems should be short and opaque, got %q", r.NewStem)
		}
		for _, f := range r.Files {
			if strings.HasSuffix(f.From, ".lock") {
				t.Errorf("lock files are not carried along: %s", f.From)
			}
		}
	}
	var first EpisodeRename
	for _, r := range plan.Renames {
		if r.GUID == "guid-a" {
			first = r
		}
	}
	if first.OldStem == "" || len(first.Files) < 5 {
		t.Errorf("an episode's files must be planned together, got %+v", first)
	}
}

func TestRenameMovesEveryFileTogetherAndKeepsTheEpisodeFindable(t *testing.T) {
	entry, a, b := renameLibrary(t)
	audioBefore := func() string { p, _ := NewLocalEpisodes(entry.Dir).Find(a); return p }()
	idBefore := GetOrSetEpisodeShortID(entry.Dir, entry.ShortID, audioBefore)

	plan := PlanEpisodeRenames(entry)
	backup := t.TempDir()
	if _, err := BackupMetadata([]PodcastDirEntry{entry}, nil, backup); err != nil {
		t.Fatal(err)
	}
	res := ApplyEpisodeRenames([]RenamePlan{plan}, backup)
	if len(res.Failures) != 0 || res.Episodes != 2 {
		t.Fatalf("unexpected result %+v", res)
	}

	newA, ok := NewLocalEpisodes(entry.Dir).Find(a)
	if !ok || strings.Contains(newA, "Title") {
		t.Fatalf("episode A should be found by GUID under its new name, got %q %v", newA, ok)
	}
	stem := strings.TrimSuffix(newA, ".mp3")
	for _, suffix := range []string{".mp3.precut", ".transcript.json", ".cuts.json", ".fp", ".mp3.json"} {
		if !util.FileExists(stem + suffix) {
			t.Errorf("%s was left behind", suffix)
		}
	}
	if got, _ := os.ReadFile(stem + ".mp3.precut"); string(got) != "original audio" {
		t.Errorf("content must survive the move, got %q", got)
	}
	if _, ok := NewLocalEpisodes(entry.Dir).Find(b); !ok {
		t.Error("episode B should still be found")
	}
	if idAfter := GetOrSetEpisodeShortID(entry.Dir, entry.ShortID, newA); idAfter != idBefore {
		t.Errorf("the short ID must not change: %s then %s", idBefore, idAfter)
	}
	if got := episode.EpisodeTitleFromPath(newA); got != a.Title {
		t.Errorf("the title comes from the identity, got %q", got)
	}
	st := episode.GetOrCreateEpisodeStatus(newA)
	if st.MediaFile != filepath.Base(newA) || st.Original.Filename != filepath.Base(stem)+".mp3.precut" {
		t.Errorf("the status file must name the new files: %+v", st)
	}
	queue, _ := episode.ReadQueue(entry.Dir)
	if len(queue) != 2 || queue[0] != filepath.Base(newA) || queue[1] != "no_identity.mp3" {
		t.Errorf("the queue must follow the rename and keep the rest, got %v", queue)
	}
	for _, untouched := range []string{"no_identity.mp3", "pruned_long_ago.transcript.json"} {
		if !util.FileExists(filepath.Join(entry.Dir, untouched)) {
			t.Errorf("%s must not be touched", untouched)
		}
	}
}

func TestUndoRestoresTheOriginalTreeExactly(t *testing.T) {
	entry, _, _ := renameLibrary(t)
	before := snapshot(t, entry.Dir)
	for name := range before {
		if strings.HasSuffix(name, ".lock") {
			delete(before, name)
		}
	}

	backup := t.TempDir()
	if _, err := BackupMetadata([]PodcastDirEntry{entry}, nil, backup); err != nil {
		t.Fatal(err)
	}
	res := ApplyEpisodeRenames([]RenamePlan{PlanEpisodeRenames(entry)}, backup)
	if res.Episodes != 2 {
		t.Fatalf("setup: %+v", res)
	}
	if reflectEqual(names(before), names(snapshot(t, entry.Dir))) {
		t.Fatal("setup: the rename changed nothing")
	}

	undone, err := UndoEpisodeRenames(backup)
	if err != nil || undone.Renamed == 0 {
		t.Fatalf("undo failed: %+v %v", undone, err)
	}
	after := snapshot(t, entry.Dir)
	for name := range after {
		if strings.HasSuffix(name, ".lock") {
			delete(after, name)
		}
	}
	if !reflectEqual(before, after) {
		for k, v := range before {
			if after[k] != v {
				t.Errorf("%s: before %q, after %q", k, v, after[k])
			}
		}
		for k := range after {
			if _, ok := before[k]; !ok {
				t.Errorf("%s appeared", k)
			}
		}
	}
}

func reflectEqual(a, b any) bool {
	switch x := a.(type) {
	case []string:
		y := b.([]string)
		return strings.Join(x, "\x00") == strings.Join(y, "\x00")
	case map[string]string:
		y := b.(map[string]string)
		if len(x) != len(y) {
			return false
		}
		for k, v := range x {
			if y[k] != v {
				return false
			}
		}
		return true
	}
	return false
}

func TestABrokenStepRollsBackThatEpisodeAndCarriesOn(t *testing.T) {
	entry, a, b := renameLibrary(t)
	plan := PlanEpisodeRenames(entry)
	// Make the last file of the first episode impossible to move.
	first := plan.Renames[0]
	last := first.Files[len(first.Files)-1]
	must(t, os.MkdirAll(filepath.Join(last.To, "blocker"), 0o755))
	before := snapshot(t, entry.Dir)

	backup := t.TempDir()
	must(t, func() error { _, err := BackupMetadata([]PodcastDirEntry{entry}, nil, backup); return err }())
	res := ApplyEpisodeRenames([]RenamePlan{plan}, backup)
	if len(res.Failures) != 1 || res.Episodes != 1 {
		t.Fatalf("want one failure and one success, got %+v", res)
	}
	for _, f := range first.Files {
		if !util.FileExists(f.From) {
			t.Errorf("%s should have been put back", f.From)
		}
	}
	_ = before
	_, _ = a, b
}

func TestABusyEpisodeIsLeftAlone(t *testing.T) {
	entry, _, _ := renameLibrary(t)
	plan := PlanEpisodeRenames(entry)
	audio := plan.Renames[0].Audio().From
	lock, err := util.AcquireFileLock(audio)
	if err != nil || lock == nil {
		t.Fatalf("test lock: %v", err)
	}
	defer lock.Release()

	backup := t.TempDir()
	must(t, func() error { _, err := BackupMetadata([]PodcastDirEntry{entry}, nil, backup); return err }())
	res := ApplyEpisodeRenames([]RenamePlan{plan}, backup)
	if res.Busy != 1 || res.Episodes != 1 || len(res.Failures) != 0 {
		t.Errorf("want one busy, one renamed, no failures; got %+v", res)
	}
	if !util.FileExists(audio) {
		t.Error("a busy episode must not be moved from under its worker")
	}
}

func TestDuplicateDownloadsOfOneEpisodeGetDifferentNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fe := feedEpisode("same-guid", "Twice", pub)
	for _, n := range []string{"first.mp3", "second.mp3"} {
		must(t, RecordFeedEpisode(writeAudio(t, dir, n), fe, ""))
	}
	plan := PlanEpisodeRenames(PodcastDirEntry{Dir: dir, Title: "Show"})
	if len(plan.Renames) != 2 || plan.Renames[0].NewStem == plan.Renames[1].NewStem {
		t.Errorf("two files must never be given one name: %+v", plan.Renames)
	}
}

func TestBackupHoldsMetadataButNotAudio(t *testing.T) {
	t.Parallel()
	entry, _, _ := renameLibrary(t)
	backup := t.TempDir()
	m, err := BackupMetadata([]PodcastDirEntry{entry}, nil, backup)
	if err != nil || m.Files == 0 {
		t.Fatalf("backup failed: %+v %v", m, err)
	}
	inBackup := listFileNames(filepath.Join(backup, filepath.Base(entry.Dir)))
	joined := strings.Join(inBackup, " ")
	for _, want := range []string{".transcript.json", ".cuts.json", ".mp3.json", "queue.json", "pruned_long_ago.transcript.json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the backup should hold %s, got %v", want, inBackup)
		}
	}
	for _, name := range inBackup {
		if strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".precut") || strings.HasSuffix(name, ".fp") || strings.HasSuffix(name, ".lock") {
			t.Errorf("audio and regenerable files stay out of the backup, found %s", name)
		}
	}
}

func TestRewritePathsInUpdatesPlayerState(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "play_queue.json")
	writeFile(t, file, `{"current":"/lib/show/old.mp3","queue":["/lib/show/old.mp3","/lib/show/other.mp3"]}`)
	changed, err := RewritePathsIn(file, map[string]string{"/lib/show/old.mp3": "/lib/show/new.mp3"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got, _ := os.ReadFile(file)
	if strings.Contains(string(got), "old.mp3") || !strings.Contains(string(got), "other.mp3") {
		t.Errorf("only the renamed path should change: %s", got)
	}
	if changed, _ := RewritePathsIn(file, map[string]string{"/nowhere": "/x"}); changed {
		t.Error("nothing to change means nothing rewritten")
	}
}
