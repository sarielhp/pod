package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/types"
)

// The output name replaces the source extension rather than appending to it,
// so a video yields "lecture.transcript.json" and not
// "lecture.mkv.transcript.json".
func TestTranscriptOutputPathReplacesTheExtension(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, suffix, want string }{
		{"/m/lecture.mkv", ".transcript.json", "/m/lecture.transcript.json"},
		{"/m/talk.mp4", ".srt", "/m/talk.srt"},
		{"/m/show.mp3", ".txt", "/m/show.txt"},
		{"/m/no-ext", ".transcript.json", "/m/no-ext.transcript.json"},
		{"/m/dotted.name.mkv", ".srt", "/m/dotted.name.srt"},
	}
	for _, c := range cases {
		if got := transcriptOutputPath(TranscribeRequest{Path: c.in}, c.suffix); got != c.want {
			t.Errorf("transcriptOutputPath(%q, %q) = %q, want %q", c.in, c.suffix, got, c.want)
		}
	}
}

func TestTranscriptOutputPathHonoursOutputDir(t *testing.T) {
	t.Parallel()
	got := transcriptOutputPath(TranscribeRequest{Path: "/m/lecture.mkv", OutputDir: "/out"}, ".srt")
	if got != "/out/lecture.srt" {
		t.Errorf("got %q, want /out/lecture.srt", got)
	}
}

func TestTranscribeFileRejectsMissingAndDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if _, err := TranscribeFile(TranscribeRequest{Path: filepath.Join(dir, "absent.mkv")},
		types.Config{}, types.ProcOptions{Quiet: true}, nil); err == nil {
		t.Error("a missing file should be an error")
	}
	if _, err := TranscribeFile(TranscribeRequest{Path: dir},
		types.Config{}, types.ProcOptions{Quiet: true}, nil); err == nil {
		t.Error("a directory should be an error")
	}
}

// A file ffmpeg cannot read reports that, rather than producing an empty
// transcript or panicking.
func TestTranscribeFileRejectsUnreadableMedia(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notmedia.mp4")
	if err := os.WriteFile(path, []byte("this is not a video"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := TranscribeFile(TranscribeRequest{Path: path},
		types.Config{}, types.ProcOptions{Quiet: true}, nil)
	if err == nil {
		t.Fatal("expected an error for a file with no audio track")
	}
	if !strings.Contains(err.Error(), "no readable audio track") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func TestWriteTranscriptFormatsRejectsUnknownFormat(t *testing.T) {
	t.Parallel()
	_, err := writeTranscriptFormats(
		TranscribeRequest{Path: filepath.Join(t.TempDir(), "x.mp3"), Formats: []string{"pdf"}},
		&types.TranscriptionData{}, 1, true)
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("expected an unknown-format error, got %v", err)
	}
}

// Nothing about the podcast library is touched: a one-off transcription must
// not leave a status file, a queue entry, or a short ID behind.
func TestTranscribeFileLeavesNoLibraryState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "lecture.mp4")
	if err := os.WriteFile(path, []byte("not really a video"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _ = TranscribeFile(TranscribeRequest{Path: path}, types.Config{},
		types.ProcOptions{Quiet: true}, nil)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch {
		case e.Name() == "lecture.mp4":
		case e.Name() == "queue.json", e.Name() == "podcast.json":
			t.Errorf("transcription created library state: %s", e.Name())
		case strings.HasSuffix(e.Name(), ".status.json"):
			t.Errorf("transcription created an episode status file: %s", e.Name())
		}
	}
}
