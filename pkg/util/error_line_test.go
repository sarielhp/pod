package util

import (
	"bytes"
	"os"
	"testing"
)

func TestFprintErrorStartsOnItsOwnLineAfterABlankLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	FprintError(&buf, "could not save %s: %v\n\n", "ep.json", "disk full")
	if got, want := buf.String(), "\ncould not save ep.json: disk full\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFprintErrorIgnoresLeadingNewlinesAndEmptyMessages(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	FprintError(&buf, "\n\nError: %s\n", "boom")
	if got := buf.String(); got != "\nError: boom\n" {
		t.Errorf("got %q", got)
	}
	buf.Reset()
	FprintError(&buf, "\n\n")
	if buf.Len() != 0 {
		t.Errorf("an empty message prints nothing, got %q", buf.String())
	}
}

func TestErrorsAreOnlyColouredOnARealTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	var buf bytes.Buffer
	if wantsColor(&buf) {
		t.Error("a buffer is not a terminal and must get plain text")
	}
	dir := t.TempDir()
	f, err := os.Create(dir + "/log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if wantsColor(f) {
		t.Error("a regular file must get plain text")
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if !wantsColor(null) {
		t.Error("a character device stands in for a terminal here and should get colour")
	}
	t.Setenv("NO_COLOR", "1")
	if wantsColor(null) {
		t.Error("NO_COLOR must switch colour off")
	}
}
