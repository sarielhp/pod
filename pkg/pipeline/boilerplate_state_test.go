package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/config"
)

func analyzed(t *testing.T, dir string, opts AnalyzeOptions) AnalyzeResult {
	t.Helper()
	res, err := AnalyzePodcast(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAnalyzePassesOverAPodcastNothingChangedIn(t *testing.T) {
	dir := showWith(t, 12, 8)
	first := analyzed(t, dir, AnalyzeOptions{})
	if first.Unchanged {
		t.Fatal("the first analysis has nothing to be unchanged from")
	}
	second := analyzed(t, dir, AnalyzeOptions{})
	if !second.Unchanged || len(second.Phrases) != len(first.Phrases) || second.Added != 0 || second.Dropped != 0 {
		t.Fatalf("a rerun over the same transcripts should be skipped and report the same phrases: %+v", second)
	}
}

func TestAnalyzeRereadsWhenTranscriptsOptionsOrBoilerplateChange(t *testing.T) {
	dir := showWith(t, 12, 8)
	analyzed(t, dir, AnalyzeOptions{})

	writeEpisode(t, dir, 12, true)
	if analyzed(t, dir, AnalyzeOptions{}).Unchanged {
		t.Error("a new transcript must trigger a fresh analysis")
	}
	if !analyzed(t, dir, AnalyzeOptions{}).Unchanged {
		t.Fatal("the analysis after a change should be remembered in turn")
	}

	writeEpisode(t, dir, 0, false)
	if analyzed(t, dir, AnalyzeOptions{}).Unchanged {
		t.Error("a rewritten transcript must trigger a fresh analysis")
	}

	if analyzed(t, dir, AnalyzeOptions{MinEpisodes: 3}).Unchanged {
		t.Error("different thresholds must not reuse the old result")
	}

	cfg := config.LoadPodcastConfig(dir, config.PodcastConfig{})
	cfg.Boilerplate[0].Disabled = true
	if err := config.SavePodcastConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if analyzed(t, dir, AnalyzeOptions{MinEpisodes: 3}).Unchanged {
		t.Error("an edit to the recorded boilerplate must trigger a fresh analysis")
	}
}

func TestAnalyzeDryRunRemembersNothing(t *testing.T) {
	dir := showWith(t, 12, 8)
	analyzed(t, dir, AnalyzeOptions{DryRun: true})
	if _, err := os.Stat(filepath.Join(dir, boilerplateStateFile)); err == nil {
		t.Fatal("a dry run must not write the state file")
	}
	if analyzed(t, dir, AnalyzeOptions{}).Unchanged {
		t.Fatal("after a dry run the real analysis still has to run")
	}
}

func TestAnalyzeIgnoresStateFromAnotherVersion(t *testing.T) {
	dir := showWith(t, 12, 8)
	analyzed(t, dir, AnalyzeOptions{})
	if err := os.WriteFile(filepath.Join(dir, boilerplateStateFile), []byte(`{"version":0,"analyze":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if analyzed(t, dir, AnalyzeOptions{}).Unchanged {
		t.Fatal("state written by another version must not be trusted")
	}
}
