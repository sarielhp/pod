package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranscribeMinutes(t *testing.T) {
	t.Parallel()
	cases := map[string]float64{"": 0, "0": 0, "-5": 0, "abc": 0, "5": 5, " 2.5 ": 2.5}
	for in, want := range cases {
		if got := transcribeMinutes(in); got != want {
			t.Errorf("transcribeMinutes(%q) = %v, want %v", in, got, want)
		}
	}
}

// A directory expands to the media inside it; a file named explicitly is taken
// as given, whatever its extension, because ffmpeg decides what it can read.
func TestExpandTranscribeTargets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, n := range []string{"a.mkv", "b.mp3", "c.txt", "d.webm", ".hidden.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := expandTranscribeTargets([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, p := range got {
		names[filepath.Base(p)] = true
	}
	for _, want := range []string{"a.mkv", "b.mp3", "d.webm"} {
		if !names[want] {
			t.Errorf("directory scan missed %s", want)
		}
	}
	if names["c.txt"] {
		t.Error("a .txt file is not media")
	}
	if names[".hidden.mp4"] {
		t.Error("hidden files should be skipped")
	}
	if len(got) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(got), got)
	}

	// An explicitly named file is attempted regardless of extension.
	odd := filepath.Join(dir, "c.txt")
	got, err = expandTranscribeTargets([]string{odd})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != odd {
		t.Errorf("an explicitly named file should be kept: %v", got)
	}
}

func TestExpandTranscribeTargetsDeduplicates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := expandTranscribeTargets([]string{p, dir, p})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("the same file named three ways should appear once, got %v", got)
	}
}

func TestExpandTranscribeTargetsReportsMissingPaths(t *testing.T) {
	t.Parallel()
	if _, err := expandTranscribeTargets([]string{filepath.Join(t.TempDir(), "absent")}); err == nil {
		t.Error("a missing path should be an error, not silently skipped")
	}
}
