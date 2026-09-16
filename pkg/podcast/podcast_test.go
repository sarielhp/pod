package podcast

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
)

func TestFeedCache(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "feed_cache.json")
	mgr := newFeedCacheManager(cachePath)

	feedURL := "https://example.com/feed.xml"
	entry := &FeedCacheEntry{
		FeedURL:     feedURL,
		ETag:        "\"12345\"",
		LastChecked: time.Now(),
		Episodes: []backend.FeedEpisode{
			{Title: "Ep 1", GUID: "guid-1"},
		},
	}

	mgr.Put(feedURL, entry)
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got := mgr.Get(feedURL)
	if got == nil || got.ETag != "\"12345\"" || len(got.Episodes) != 1 {
		t.Fatalf("Unexpected cached entry: %+v", got)
	}

	mgr2 := newFeedCacheManager(cachePath)
	got2 := mgr2.Get(feedURL)
	if got2 == nil || got2.ETag != "\"12345\"" {
		t.Fatalf("Reloaded cache missing entry")
	}

	if entry.IsExpired(24 * time.Hour) {
		t.Errorf("expected entry not to be expired")
	}
	oldEntry := &FeedCacheEntry{LastChecked: time.Now().Add(-48 * time.Hour)}
	if !oldEntry.IsExpired(24 * time.Hour) {
		t.Errorf("expected old entry to be expired")
	}
}

func TestPodcastShortID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		title    string
		want5Len bool
	}{
		{"The Daily", true},
		{"Planet Money", true},
		{"Radiolab", true},
		{"99% Invisible", true},
		{"", true},
	}

	for _, tt := range tests {
		id := GeneratePodcastShortID(tt.title)
		if len(id) != 5 {
			t.Errorf("GeneratePodcastShortID(%q) = %q (len %d, want 5)", tt.title, id, len(id))
		}
	}
}

func TestEpisodeShortID(t *testing.T) {
	t.Parallel()
	podShort := "plntm"
	epKey := "123_Inflation.mp3"
	id := generateEpisodeShortID(podShort, epKey)
	if len(id) != 6 || id[0] != 'e' {
		t.Fatalf("unexpected episode short ID: %s", id)
	}

	title := EpisodeTitleFromPath("/tmp/podcast/Episode 42.mp3")
	if title != "Episode 42" {
		t.Errorf("expected 'Episode 42', got %s", title)
	}
}

func TestDownloadQueue(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	qPath := filepath.Join(tmpDir, "dl_queue.json")
	q := NewDownloadQueue(qPath)

	item := DownloadQueueItem{
		PodcastTitle: "My Podcast",
		EpisodeTitle: "Ep 1",
		GUID:         "guid-101",
		EnclosureURL: "https://example.com/101.mp3",
	}

	ok, status := q.Enqueue(item)
	if !ok || status != "queued" {
		t.Fatalf("Enqueue failed: ok=%v, status=%s", ok, status)
	}

	if !q.IsEpisodeInQueue("guid-101", "", "Ep 1") {
		t.Errorf("expected episode to be in queue")
	}

	items := q.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	claimed, ok, err := q.Claim()
	if err != nil || !ok {
		t.Fatalf("Claim failed: %v", err)
	}
	if claimed.Status != "downloading" {
		t.Errorf("expected claimed status to be downloading, got %s", claimed.Status)
	}

	if err := q.Finalize(claimed.ID, nil); err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}
	items = q.Items()
	if items[0].Status != "completed" {
		t.Errorf("expected status completed, got %s", items[0].Status)
	}

	if !q.Remove(claimed.ID) {
		t.Errorf("expected item to be removed")
	}
	if len(q.Items()) != 0 {
		t.Errorf("expected queue to be empty")
	}
}

func TestDownloadPolicy(t *testing.T) {
	t.Parallel()
	catalog := []backend.FeedEpisode{
		{Title: "Ep 1", EnclosureURL: "https://example.com/1.mp3"},
		{Title: "Ep 2", EnclosureURL: "https://example.com/2.mp3"},
		{Title: "Ep 3", EnclosureURL: "https://example.com/3.mp3"},
	}

	isDl := func(ep backend.FeedEpisode) bool {
		return ep.Title == "Ep 1"
	}

	eps, _ := selectEpisodesByDownloadPolicy(catalog, isDl, config.DownloadPolicyNone, 0, false)
	if len(eps) != 0 {
		t.Errorf("expected 0 episodes for none policy, got %d", len(eps))
	}

	eps, _ = selectEpisodesByDownloadPolicy(catalog, isDl, config.DownloadPolicyLatest, 0, false)
	if len(eps) != 1 || eps[0].Title != "Ep 3" {
		t.Errorf("expected Ep 3 for latest policy, got %+v", eps)
	}

	eps, _ = selectEpisodesByDownloadPolicy(catalog, isDl, config.DownloadPolicyAll, 0, false)
	if len(eps) != 2 {
		t.Errorf("expected 2 undownloaded episodes for all policy, got %d", len(eps))
	}
}

func TestOrphanPodcasts(t *testing.T) {
	t.Parallel()
	podcasts := []backend.Podcast{
		{
			ID: "p1",
			Media: backend.PodcastMedia{
				Metadata: backend.PodcastMetadata{Title: "Orphan No Feed", FeedURL: ""},
			},
		},
		{
			ID: "p2",
			Media: backend.PodcastMedia{
				Metadata: backend.PodcastMetadata{Title: "Duplicate Feed A", FeedURL: "https://example.com/rss/"},
			},
		},
		{
			ID: "p3",
			Media: backend.PodcastMedia{
				Metadata: backend.PodcastMetadata{Title: "Duplicate Feed B", FeedURL: "https://example.com/rss"},
			},
		},
	}

	orphans := findOrphanPodcasts(podcasts)
	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans (1 empty feed + 1 duplicate), got %d", len(orphans))
	}
}

func TestPodcastCache(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	podDir := filepath.Join(tmpDir, "TestPodcast")
	_ = os.MkdirAll(podDir, 0755)

	index := &CachedPodcastIndex{
		PodcastName: "Test Podcast",
		PodcastDir:  podDir,
		Episodes: []CachedEpisodeSummary{
			{EpisodeFile: EpisodeFile{Title: "Ep 1", Filename: "ep1.mp3"}},
		},
	}

	if err := SavePodcastCache(podDir, index); err != nil {
		t.Fatalf("SavePodcastCache failed: %v", err)
	}

	loaded, err := LoadPodcastCache(podDir)
	if err != nil {
		t.Fatalf("LoadPodcastCache failed: %v", err)
	}
	if loaded.PodcastName != "Test Podcast" || len(loaded.Episodes) != 1 {
		t.Errorf("unexpected loaded cache: %+v", loaded)
	}
}

func TestSelectNewEpisodes(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	catalog := []backend.FeedEpisode{
		{Title: "Old 1", PublishedAt: t0.UnixMilli(), EnclosureURL: "http://example.com/1.mp3"},
		{Title: "Old 2", PublishedAt: t1.UnixMilli(), EnclosureURL: "http://example.com/2.mp3"},
		{Title: "New 3", PublishedAt: t2.UnixMilli(), EnclosureURL: "http://example.com/3.mp3"},
	}

	// 1. With existing downloaded episode (index 1 is downloaded): only newer episode (index 2) should be selected
	downloaded := func(ep backend.FeedEpisode) bool {
		return ep.Title == "Old 2"
	}
	eps, reasons := selectNewEpisodes(catalog, []int{1}, downloaded, nil)
	if len(eps) != 1 || eps[0].Title != "New 3" {
		t.Fatalf("expected only 'New 3' selected, got %d eps: %v (reasons: %v)", len(eps), eps, reasons)
	}

	// 2. With 0 downloaded episodes and favoriteSince set to t2: only episodes >= t2 should be selected
	eps2, _ := selectNewEpisodes(catalog, nil, func(ep backend.FeedEpisode) bool { return false }, &t2)
	if len(eps2) != 1 || eps2[0].Title != "New 3" {
		t.Fatalf("expected only 'New 3' selected for cutoff t2, got: %v", eps2)
	}

	// 3. With 0 downloaded episodes and favoriteSince set after all episodes: 0 should be selected
	tFuture := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	eps3, _ := selectNewEpisodes(catalog, nil, func(ep backend.FeedEpisode) bool { return false }, &tFuture)
	if len(eps3) != 0 {
		t.Fatalf("expected 0 episodes selected for future cutoff, got: %v", eps3)
	}
}
