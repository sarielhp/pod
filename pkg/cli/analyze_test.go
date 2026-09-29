package cli

import (
	"bytes"
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
