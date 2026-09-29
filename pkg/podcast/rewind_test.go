package podcast

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/episode"
	"pod/pkg/progress"
	"pod/pkg/util"
)

func TestParseRewindWindow(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"24h", 24 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1.5d", 36 * time.Hour, false},
		{"24", 0, true},
		{"", 0, true},
		{"0h", 0, true},
		{"-2h", 0, true},
		{"xd", 0, true},
	}
	for _, c := range cases {
		got, err := ParseRewindWindow(c.in)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("ParseRewindWindow(%q) = %v, %v; want %v, err=%v", c.in, got, err, c.want, c.wantErr)
		}
	}
}

type rewindFixture struct {
	lib    *Library
	sub    Subscription
	podDir string
	now    time.Time
}

func newRewindFixture(t *testing.T) rewindFixture {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	sub := Subscription{ID: "show1", Title: "Show One", FeedURL: "https://example.test/one.xml", Folder: "Show_One"}
	podDir := filepath.Join(root, "lib", sub.Folder)
	if err := os.MkdirAll(filepath.Join(podDir, ".work"), 0o755); err != nil {
		t.Fatal(err)
	}
	cache := newFeedCacheManager(filepath.Join(root, "feed_cache.json"))
	cache.Put(sub.FeedURL, &FeedCacheEntry{
		FeedURL: sub.FeedURL, ETag: "abc", LastModified: "yesterday", LastChecked: time.Now(),
		LatestGUID: "g1", LastBuildDate: "today", EpisodeCount: 5, ImageURL: "cover.jpg",
		PubDates: []FeedCachePubDate{{Title: "Kept", PublishedAt: 1}},
	})
	lib := &Library{
		cfg:       Config{PodcastsDir: filepath.Join(root, "lib"), SubscriptionsFile: filepath.Join(root, "subs.json")},
		feedCache: cache,
		progress:  progress.Discard,
	}
	return rewindFixture{lib: lib, sub: sub, podDir: podDir, now: time.Now()}
}

func (f rewindFixture) write(t *testing.T, rel string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(f.podDir, rel)
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := f.now.Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f rewindFixture) exists(rel string) bool {
	return util.FileExists(filepath.Join(f.podDir, rel))
}

func TestPlanRewindSelectsByAgeAndCollectsSidecars(t *testing.T) {
	f := newRewindFixture(t)
	for _, name := range []string{"new.mp3", "new.mp3.precut", "new.mp3.json", "new.cuts.json", "new.transcript.json", "new.srt", "new.txt", ".work/new.wav"} {
		f.write(t, name, time.Hour)
	}
	f.write(t, "new2.mp3", time.Hour)
	f.write(t, "old.mp3", 48*time.Hour)
	f.write(t, "old.transcript.json", 48*time.Hour)

	plans := f.lib.PlanRewind([]Subscription{f.sub}, RewindOptions{Since: 24 * time.Hour, Now: f.now})
	if len(plans) != 1 || len(plans[0].Episodes) != 2 {
		t.Fatalf("want one plan with 2 episodes, got %+v", plans)
	}
	byName := map[string]RewindEpisode{}
	for _, ep := range plans[0].Episodes {
		byName[filepath.Base(ep.Audio)] = ep
	}
	if _, ok := byName["old.mp3"]; ok {
		t.Fatal("old.mp3 is outside the window and must not be planned")
	}
	if got := len(byName["new.mp3"].Files); got != 8 {
		t.Errorf("new.mp3 should carry 8 files, got %d: %v", got, byName["new.mp3"].Files)
	}
	if got := len(byName["new2.mp3"].Files); got != 1 {
		t.Errorf("new2.mp3 must not absorb new.* sidecars, got %v", byName["new2.mp3"].Files)
	}
	if !f.exists("new.mp3") {
		t.Fatal("planning must not delete anything")
	}
}

func TestPlanRewindTargetFiltersSubscriptions(t *testing.T) {
	f := newRewindFixture(t)
	f.write(t, "new.mp3", time.Hour)
	plans := f.lib.PlanRewind([]Subscription{f.sub}, RewindOptions{Since: 24 * time.Hour, Target: "nonexistent", Now: f.now})
	if len(plans) != 0 {
		t.Fatalf("target that matches nothing should plan nothing, got %+v", plans)
	}
}

func TestApplyRewindRestoresFetchableState(t *testing.T) {
	f := newRewindFixture(t)
	f.write(t, "new.mp3", time.Hour)
	f.write(t, "new.transcript.json", time.Hour)
	f.write(t, "new.mp3.precut", time.Hour)
	f.write(t, ".work/new.wav", time.Hour)
	f.write(t, "new2.mp3", time.Hour)
	f.write(t, "old.mp3", 48*time.Hour)
	f.write(t, "old.transcript.json", 48*time.Hour)
	if err := episode.UpdateQueue(f.podDir, func([]string) []string { return []string{"new.mp3", "old.mp3"} }); err != nil {
		t.Fatal(err)
	}

	plans := f.lib.PlanRewind([]Subscription{f.sub}, RewindOptions{Since: 24 * time.Hour, Now: f.now})
	res := f.lib.ApplyRewind(plans)
	if len(res.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", res.Failures)
	}
	if res.Episodes != 2 || res.Podcasts != 1 {
		t.Errorf("want 2 episodes in 1 podcast, got %+v", res)
	}
	for _, gone := range []string{"new.mp3", "new.transcript.json", "new.mp3.precut", ".work/new.wav", "new2.mp3", "new.mp3.lock"} {
		if f.exists(gone) {
			t.Errorf("%s should have been removed", gone)
		}
	}
	for _, kept := range []string{"old.mp3", "old.transcript.json"} {
		if !f.exists(kept) {
			t.Errorf("%s is outside the window and must survive", kept)
		}
	}

	queue, err := episode.ReadQueue(f.podDir)
	if err != nil || len(queue) != 1 || queue[0] != "old.mp3" {
		t.Errorf("queue should keep only old.mp3, got %v (err %v)", queue, err)
	}

	entry := f.lib.feedCache.Get(f.sub.FeedURL)
	if entry.ETag != "" || entry.LastModified != "" || entry.LatestGUID != "" || entry.EpisodeCount != 0 || entry.LastBuildDate != "" {
		t.Errorf("freshness markers should be reset, got %+v", entry)
	}
	if entry.ImageURL != "cover.jpg" || len(entry.PubDates) != 1 {
		t.Errorf("cover and publication history must be kept, got %+v", entry)
	}

	isDownloaded := downloadedEpisodeChecker(f.podDir)
	if isDownloaded(backend.FeedEpisode{Title: "new"}) {
		t.Error("rewound episode should read as not downloaded")
	}
	if !util.FileExists(filepath.Join(f.podDir, "feed.xml")) {
		t.Error("the affected podcast should be republished")
	}
}

func TestApplyRewindWithNothingInWindowLeavesFeedsAlone(t *testing.T) {
	f := newRewindFixture(t)
	f.write(t, "old.mp3", 48*time.Hour)
	plans := f.lib.PlanRewind([]Subscription{f.sub}, RewindOptions{Since: time.Hour, Now: f.now})
	res := f.lib.ApplyRewind(plans)
	if res.Episodes != 0 || len(res.Failures) != 0 {
		t.Fatalf("nothing should be rewound, got %+v", res)
	}
	if f.lib.feedCache.Get(f.sub.FeedURL).ETag != "abc" {
		t.Error("feed markers must not be reset when no episode was rewound")
	}
	if !f.exists("old.mp3") {
		t.Error("old.mp3 must survive")
	}
}
