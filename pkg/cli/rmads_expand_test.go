package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"pod/pkg/types"
)

// writeEpisode lays out one episode the way podfetch does:
// <library>/<podcast>/<episode>/podcast.mp3, with podcast.json one level
// above the audio rather than beside it.
func writeEpisode(t *testing.T, lib, show, episode string) string {
	t.Helper()
	dir := filepath.Join(lib, show, episode)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "podcast.mp3")
	if err := os.WriteFile(p, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writePodcastConfig(t *testing.T, lib, show, adRemoval string) {
	t.Helper()
	dir := filepath.Join(lib, show)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"id":"testid","ad_removal":"` + adRemoval + `","download_policy":"all","download_k":3}`
	if err := os.WriteFile(filepath.Join(dir, "podcast.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExpandDirectoryArgsFindsNestedPodfetchEpisodes(t *testing.T) {
	t.Parallel()
	lib := t.TempDir()
	writePodcastConfig(t, lib, "Show", "all")
	wantA := writeEpisode(t, lib, "Show", "Episode A")
	wantB := writeEpisode(t, lib, "Show", "Episode B")
	// A re-download lands in a sibling folder suffixed "-1" and is skipped.
	writeEpisode(t, lib, "Show", "Episode B-1")
	// A podcast that opted out stays out.
	writePodcastConfig(t, lib, "Muted", "none")
	writeEpisode(t, lib, "Muted", "Episode C")

	got := expandDirectoryArgs([]string{lib}, types.ProcOptions{Quiet: true, DryRun: true}, types.Config{})
	sort.Strings(got)
	want := []string{wantA, wantB}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("expected %d episodes, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("episode %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

func TestExpandDirectoryArgsAppliesLatestPolicyPerPodcast(t *testing.T) {
	t.Parallel()
	// "latest" has to compare a podcast's episodes against each other, which
	// only works when they are grouped by podcast rather than by episode.
	lib := t.TempDir()
	writePodcastConfig(t, lib, "Show", "latest")
	writeEpisode(t, lib, "Show", "Episode A")
	writeEpisode(t, lib, "Show", "Episode B")
	writeEpisode(t, lib, "Show", "Episode C")

	got := expandDirectoryArgs([]string{lib}, types.ProcOptions{Quiet: true, DryRun: true}, types.Config{})
	if len(got) != 1 {
		t.Fatalf("expected the latest episode only, got %d: %v", len(got), got)
	}
}

func TestExpandDirectoryArgsProcessesPodcastWithNoConfig(t *testing.T) {
	t.Parallel()
	// No podcast.json must mean "use the defaults", not "silently skip".
	lib := t.TempDir()
	want := writeEpisode(t, lib, "Unconfigured", "Episode A")

	got := expandDirectoryArgs([]string{lib}, types.ProcOptions{Quiet: true, DryRun: true}, types.Config{})
	if len(got) != 1 || got[0] != want {
		t.Fatalf("expected the unconfigured podcast's episode, got %v", got)
	}
}

func TestExpandDirectoryArgsHonoursAppDefaultAdRemoval(t *testing.T) {
	t.Parallel()
	// An app-wide default of "none" opts every unconfigured podcast out.
	lib := t.TempDir()
	writeEpisode(t, lib, "Unconfigured", "Episode A")

	cfg := types.Config{}
	cfg.DefaultAdRemoval = "none"
	if got := expandDirectoryArgs([]string{lib}, types.ProcOptions{Quiet: true, DryRun: true}, cfg); len(got) != 0 {
		t.Fatalf("expected nothing with default_ad_policy=none, got %v", got)
	}
}

func TestExpandDirectoryArgsWritesMissingPodcastConfig(t *testing.T) {
	t.Parallel()
	lib := t.TempDir()
	writeEpisode(t, lib, "Unconfigured", "Episode A")
	cfgPath := filepath.Join(lib, "Unconfigured", "podcast.json")

	// A dry run must not write anything.
	expandDirectoryArgs([]string{lib}, types.ProcOptions{Quiet: true, DryRun: true}, types.Config{})
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("dry run wrote a config file")
	}

	// A real run materializes the defaults it just acted on.
	expandDirectoryArgs([]string{lib}, types.ProcOptions{Quiet: true}, types.Config{})
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("expected a default config to be created: %v", err)
	}
	var written struct {
		ID        string `json:"id"`
		AdRemoval string `json:"ad_removal"`
	}
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("created config is not valid JSON: %v", err)
	}
	if written.AdRemoval != "all" {
		t.Errorf("ad_removal=%q, want \"all\"", written.AdRemoval)
	}
	if written.ID == "" {
		t.Error("created config has no podcast id")
	}
}

func TestExpandDirectoryArgsKeepsExistingPodcastConfig(t *testing.T) {
	t.Parallel()
	// An existing file is never overwritten by the defaults.
	lib := t.TempDir()
	writePodcastConfig(t, lib, "Show", "latest")
	writeEpisode(t, lib, "Show", "Episode A")
	cfgPath := filepath.Join(lib, "Show", "podcast.json")
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	expandDirectoryArgs([]string{lib}, types.ProcOptions{Quiet: true}, types.Config{})

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("existing config was rewritten:\nbefore: %s\nafter:  %s", before, after)
	}
}
