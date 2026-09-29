package podcast

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/util"
)

// pruneLibrary builds a library of two podcasts, each with seven episodes whose
// modification times run opposite to their names, so "newest" must be decided by
// time and not by sorting the names.
func pruneLibrary(t *testing.T) (*Library, string, string) {
	t.Helper()
	root := t.TempDir()
	regular, favorite := filepath.Join(root, "Regular"), filepath.Join(root, "Loved")
	for _, dir := range []string{regular, favorite} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeSevenEpisodes(t, dir)
	}
	if err := config.SavePodcastConfig(favorite, func() config.PodcastConfig {
		c := config.PodcastConfig{}
		c.SetFavorite(true)
		return c
	}()); err != nil {
		t.Fatal(err)
	}
	lib := &Library{cfg: Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "subs.json")}}
	return lib, regular, favorite
}

func writeSevenEpisodes(t *testing.T, dir string) {
	t.Helper()
	now := time.Now()
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("ep%d", i)
		age := time.Duration(i) * time.Hour
		for _, file := range []string{name + ".mp3", name + ".mp3.precut", name + ".transcript.json", name + ".cuts.json"} {
			path := filepath.Join(dir, file)
			if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPlanKeepLatestUsesTimeAndSparesFavoritesWhenAsked(t *testing.T) {
	lib, regular, _ := pruneLibrary(t)
	plans := lib.PlanKeepLatest(lib.Podcasts(), PruneOptions{Keep: 5, SkipFavorites: true})

	byTitle := map[string]PrunePlan{}
	for _, p := range plans {
		byTitle[filepath.Base(p.Dir)] = p
	}
	if got := byTitle["Loved"]; got.Skipped != "favorite" || len(got.Delete) != 0 {
		t.Errorf("a favorite must be left alone when asked, got %+v", got)
	}
	reg := byTitle["Regular"]
	if len(reg.Delete) != 2 || reg.Kept != 5 {
		t.Fatalf("want 2 deletions and 5 kept, got %+v", reg)
	}
	for _, ep := range reg.Delete {
		base := filepath.Base(ep.Audio)
		if base != "ep5.mp3" && base != "ep6.mp3" {
			t.Errorf("the two oldest by time are ep5 and ep6, got %s (dir %s)", base, regular)
		}
		if len(ep.Files) != 2 {
			t.Errorf("audio and its uncut original should both go, got %v", ep.Files)
		}
	}
	if plans2 := lib.PlanKeepLatest(lib.Podcasts(), PruneOptions{Keep: 5}); len(plans2[0].Delete)+len(plans2[1].Delete) != 4 {
		t.Error("without SkipFavorites both podcasts are pruned")
	}
}

func TestApplyPruneKeepsTranscriptsAndCleansTheQueue(t *testing.T) {
	lib, regular, favorite := pruneLibrary(t)
	if err := episode.UpdateQueue(regular, func([]string) []string { return []string{"ep0.mp3", "ep6.mp3"} }); err != nil {
		t.Fatal(err)
	}
	res := ApplyPrune(lib.PlanKeepLatest(lib.Podcasts(), PruneOptions{Keep: 5, SkipFavorites: true}))
	if len(res.Failures) != 0 || res.Episodes != 2 || res.Podcasts != 1 || res.Bytes == 0 {
		t.Fatalf("unexpected result %+v", res)
	}
	for _, gone := range []string{"ep5.mp3", "ep5.mp3.precut", "ep6.mp3", "ep6.mp3.precut"} {
		if util.FileExists(filepath.Join(regular, gone)) {
			t.Errorf("%s should be gone", gone)
		}
	}
	for _, kept := range []string{"ep5.transcript.json", "ep6.cuts.json", "ep0.mp3", "ep4.mp3.precut"} {
		if !util.FileExists(filepath.Join(regular, kept)) {
			t.Errorf("%s must survive", kept)
		}
	}
	if !util.FileExists(filepath.Join(favorite, "ep6.mp3")) {
		t.Error("the favorite's audio must survive")
	}
	if queue, _ := episode.ReadQueue(regular); len(queue) != 1 || queue[0] != "ep0.mp3" {
		t.Errorf("a deleted episode must leave the queue, got %v", queue)
	}
}

func TestPlanKeepLatestOnAShortPodcastDeletesNothing(t *testing.T) {
	lib, _, _ := pruneLibrary(t)
	for _, p := range lib.PlanKeepLatest(lib.Podcasts(), PruneOptions{Keep: 7}) {
		if len(p.Delete) != 0 {
			t.Errorf("a podcast at its limit loses nothing, got %+v", p)
		}
	}
}
