package podcast

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEpisodeFilesCollectsEverySidecarAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"2026-01-01_aaa.mp3", "2026-01-01_aaa.mp3.json", "2026-01-01_aaa.mp3.precut", "2026-01-01_aaa.cuts.json",
		"2026-01-01_aaa.transcript.json", "2026-01-01_aaa.transcript.md", "2026-01-01_aaa.srt", "2026-01-01_aaa.mp3.lock",
		"2026-01-01_aaab.mp3", "2026-01-01_aaab.transcript.json", "podcast.json", "feed.xml",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := EpisodeFiles(filepath.Join(dir, "2026-01-01_aaa.mp3"))
	var names []string
	for _, p := range got {
		names = append(names, filepath.Base(p))
	}
	want := []string{"2026-01-01_aaa.cuts.json", "2026-01-01_aaa.mp3", "2026-01-01_aaa.mp3.json", "2026-01-01_aaa.mp3.precut", "2026-01-01_aaa.srt", "2026-01-01_aaa.transcript.json", "2026-01-01_aaa.transcript.md"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	if !filepath.IsAbs(got[0]) {
		t.Fatalf("paths must be full: %s", got[0])
	}
}

func TestEpisodeFilesOfAnEpisodeWhoseAudioWasPruned(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2026-02-02_bbb.mp3.json", "2026-02-02_bbb.transcript.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := EpisodeFiles(filepath.Join(dir, "2026-02-02_bbb.mp3")); len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
