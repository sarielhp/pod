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

func TestAnalyzeWithoutArgumentsCoversTheLibraryCompactly(t *testing.T) {
	root := t.TempDir()
	big := writeShowInto(t, filepath.Join(root, "Big_Show"), 12, 8)
	small := writeShowInto(t, filepath.Join(root, "Small_Show"), 3, 3)
	for _, dir := range []string{big, small} {
		if err := config.SavePodcastConfig(dir, config.PodcastConfig{}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}

	var out bytes.Buffer
	dry := CLIOptions{Out: &out}
	dry.DryRun = true
	if err := runAnalyzeCommand(cfg, dry); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Big Show", "12 transcripts", "1 phrase ", "Analysed 1 podcast(s): would record 1 boilerplate", "1 podcast(s) have fewer than 10 transcripts"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "brought to you by") || strings.Contains(text, "Small Show") {
		t.Errorf("phrases and skipped names are listed only with --verbose:\n%s", text)
	}
	if got := config.LoadPodcastConfig(big, config.PodcastConfig{}).Boilerplate; len(got) != 0 {
		t.Fatalf("a dry run wrote %+v", got)
	}

	out.Reset()
	if err := runAnalyzeCommand(cfg, CLIOptions{Out: &out}); err != nil {
		t.Fatal(err)
	}
	if got := config.LoadPodcastConfig(big, config.PodcastConfig{}).Boilerplate; len(got) != 1 {
		t.Errorf("the real run should record the big show's phrase, got %+v", got)
	}
	if got := config.LoadPodcastConfig(small, config.PodcastConfig{}).Boilerplate; len(got) != 0 {
		t.Errorf("a show with too few transcripts records nothing, got %+v", got)
	}
}
