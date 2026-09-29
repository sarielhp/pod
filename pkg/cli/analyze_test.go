package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/config"
)

func TestAnalyzeWritesBoilerplateAndDryRunDoesNot(t *testing.T) {
	dir := writeShowOf(t, 12, 8)

	var out bytes.Buffer
	dry := CLIOptions{Args: []string{dir}, Out: &out}
	dry.DryRun = true
	if err := runAnalyzeCommand(Config{}, dry); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "would record 1 boilerplate") || !strings.Contains(out.String(), "brought to you by acme") {
		t.Errorf("unexpected dry-run output:\n%s", out.String())
	}
	if got := config.LoadPodcastConfig(dir, config.PodcastConfig{}).Boilerplate; len(got) != 0 {
		t.Fatalf("dry run wrote %+v", got)
	}

	out.Reset()
	if err := runAnalyzeCommand(Config{}, CLIOptions{Args: []string{dir}, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if got := config.LoadPodcastConfig(dir, config.PodcastConfig{}).Boilerplate; len(got) != 1 {
		t.Fatalf("want one recorded phrase, got %+v", got)
	}
}

func TestShowBoilerplateIsReadOnlyAndShowsLengthAndFlags(t *testing.T) {
	dir := writeShowOf(t, 12, 8)
	if err := runAnalyzeCommand(Config{}, CLIOptions{Args: []string{dir}, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	cfg := config.LoadPodcastConfig(dir, config.PodcastConfig{})
	if len(cfg.Boilerplate) != 1 || cfg.Boilerplate[0].Seconds <= 0 {
		t.Fatalf("the phrase's measured length should be recorded, got %+v", cfg.Boilerplate)
	}
	cfg.Boilerplate = append(cfg.Boilerplate, config.BoilerplatePhrase{
		Text: "a legacy entry with no stored length at all here", Episodes: 7, Disabled: true,
	})
	if err := config.SavePodcastConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, config.PodcastConfigFileName))

	var out bytes.Buffer
	show := CLIOptions{Args: []string{dir}, Out: &out}
	show.ShowBoilerplate = true
	if err := runAnalyzeCommand(Config{}, show); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"2 boilerplate phrase(s)", "8 eps", "7 eps", "brought to you by acme", "━━━", "┈┈┈", "~00:04", "[disabled]"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(dir, config.PodcastConfigFileName)); !bytes.Equal(before, after) {
		t.Error("--show-bp must not write podcast.json")
	}
}

func TestShowBoilerplateOnAnUnanalysedPodcast(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	show := CLIOptions{Args: []string{dir}, Out: &out}
	show.ShowBoilerplate = true
	if err := runAnalyzeCommand(Config{}, show); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no boilerplate recorded") {
		t.Errorf("want a hint to run analyze, got:\n%s", out.String())
	}
}
