package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
)

func TestBoilerplateRecutGoesNewestFirstAcrossPodcasts(t *testing.T) {
	root := t.TempDir()
	day := func(month time.Month) int64 { return time.Date(2026, month, 1, 6, 0, 0, 0, time.UTC).UnixMilli() }
	names := map[string]struct {
		show string
		when int64
	}{
		"a-oldest.mp3": {"Alpha", day(1)},
		"b-newest.mp3": {"Bravo", day(4)},
		"c-middle.mp3": {"Alpha", day(2)},
		"d-second.mp3": {"Bravo", day(3)},
	}
	for name, ep := range names {
		dir := filepath.Join(root, ep.show)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := config.SavePodcastConfig(dir, config.PodcastConfig{}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		fe := backend.FeedEpisode{GUID: name, Title: name, PublishedAt: ep.when}
		if err := podcast.RecordFeedEpisode(path, fe, ""); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}

	targets, err := boilerplateRecutTargets(cfg, CLIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b-newest.mp3", "d-second.mp3", "c-middle.mp3", "a-oldest.mp3"}
	if len(targets) != len(want) {
		t.Fatalf("want %d episodes, got %v", len(want), targets)
	}
	for i, path := range targets {
		if filepath.Base(path) != want[i] {
			t.Errorf("position %d: want %s, got %s (whole order: %v)", i, want[i], filepath.Base(path), targets)
		}
	}
}
