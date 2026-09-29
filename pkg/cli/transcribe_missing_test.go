package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranscribeMissingDryRunListsOnlyWhatLacksATranscript(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "Show")
	if err := os.MkdirAll(show, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"a.mp3": "audio", "b.mp3": "audio", "b.transcript.json": `{"text":"x"}`,
		"c.mp3": "cut", "c.mp3.precut": "original",
	} {
		if err := os.WriteFile(filepath.Join(show, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	cli := CLIOptions{Out: &out}
	cli.TranscribeMissing, cli.DryRun = true, true
	cfg := Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}
	if err := runTranscribeCommand(cfg, cli); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"a.mp3", "c.mp3", "2 episode(s) without a transcript", "1 of them are already cut", "[dry-run] Nothing was transcribed."} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "b.mp3") {
		t.Errorf("b.mp3 has a transcript and must not be listed:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(show, "a.transcript.json")); err == nil {
		t.Error("a dry run must not write a transcript")
	}
}

func TestTranscribeMissingSaysWhenNothingIsLeft(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "Show")
	_ = os.MkdirAll(show, 0o755)
	_ = os.WriteFile(filepath.Join(show, "a.mp3"), []byte("audio"), 0o644)
	_ = os.WriteFile(filepath.Join(show, "a.transcript.json"), []byte(`{"text":"x"}`), 0o644)
	var out bytes.Buffer
	cli := CLIOptions{Out: &out}
	cli.TranscribeMissing = true
	if err := runTranscribeCommand(Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}, cli); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already has a transcript") {
		t.Errorf("unexpected output:\n%s", out.String())
	}
}

func TestTranscribeWithoutPathsOrMissingIsAnError(t *testing.T) {
	if err := runTranscribeCommand(Config{}, CLIOptions{}); err == nil || !strings.Contains(err.Error(), "--missing") {
		t.Errorf("want a hint about --missing, got %v", err)
	}
}
