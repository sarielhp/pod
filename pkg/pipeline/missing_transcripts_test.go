package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAt(t *testing.T, path, body string, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestFindMissingTranscripts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "has.mp3"), "audio", 5*time.Hour)
	writeAt(t, filepath.Join(dir, "has.transcript.json"), `{"text":"x"}`, 5*time.Hour)
	writeAt(t, filepath.Join(dir, "plain.mp3"), "audio", 3*time.Hour)
	writeAt(t, filepath.Join(dir, "cut.mp3"), "cut audio", time.Hour)
	writeAt(t, filepath.Join(dir, "cut.mp3.precut"), "the original audio", time.Hour)
	writeAt(t, filepath.Join(dir, "broken.mp3"), "audio", 2*time.Hour)
	writeAt(t, filepath.Join(dir, "broken.transcript.json"), "", 2*time.Hour)

	got := FindMissingTranscripts([]string{dir, dir})
	if len(got) != 3 {
		t.Fatalf("want plain, cut and broken (an empty transcript is missing); got %+v", got)
	}
	names := []string{filepath.Base(got[0].Audio), filepath.Base(got[1].Audio), filepath.Base(got[2].Audio)}
	if names[0] != "cut.mp3" || names[1] != "broken.mp3" || names[2] != "plain.mp3" {
		t.Errorf("want newest first, got %v", names)
	}
	cut := got[0]
	if cut.Source != cut.Audio+".precut" || cut.Transcript != filepath.Join(dir, "cut.transcript.json") {
		t.Errorf("a cut episode is transcribed from its original but named after the episode: %+v", cut)
	}
	if cut.Bytes != int64(len("the original audio")) {
		t.Errorf("size should be that of the audio actually transcribed, got %d", cut.Bytes)
	}
	if got[2].Source != got[2].Audio {
		t.Errorf("an uncut episode is its own source: %+v", got[2])
	}
}

func TestOutputBaseNamesTheTranscriptAfterTheEpisode(t *testing.T) {
	t.Parallel()
	req := TranscribeRequest{Path: "/pods/show/ep.mp3.precut", OutputBase: "/pods/show/ep"}
	if got := transcriptOutputPath(req, ".transcript.json"); got != "/pods/show/ep.transcript.json" {
		t.Errorf("got %s", got)
	}
	plain := TranscribeRequest{Path: "/pods/show/lecture.mkv"}
	if got := transcriptOutputPath(plain, ".transcript.json"); got != "/pods/show/lecture.transcript.json" {
		t.Errorf("without OutputBase the name follows the input, got %s", got)
	}
}
