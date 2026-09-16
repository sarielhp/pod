package podcast

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
)

func writeCacheFile(t *testing.T, entries map[string]*FeedCacheEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feed_cache.json")
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func readCacheFile(t *testing.T, path string) map[string]*FeedCacheEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var entries map[string]*FeedCacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return entries
}

func TestFeedCacheDropsEntriesPastRetention(t *testing.T) {
	t.Parallel()
	path := writeCacheFile(t, map[string]*FeedCacheEntry{
		"https://fresh.example.com/feed.xml": {
			FeedURL: "https://fresh.example.com/feed.xml", ETag: `"v1"`,
			LastChecked: time.Now().Add(-7 * 24 * time.Hour),
		},
		"https://abandoned.example.com/feed.xml": {
			FeedURL:     "https://abandoned.example.com/feed.xml",
			LastChecked: time.Now().Add(-FeedCacheRetention - time.Hour),
		},
		"https://undated.example.com/feed.xml": {
			FeedURL: "https://undated.example.com/feed.xml",
		},
	})

	mgr := newFeedCacheManager(path)
	if mgr.Get("https://fresh.example.com/feed.xml") == nil {
		t.Error("an entry inside the retention window must survive")
	}
	if mgr.Get("https://abandoned.example.com/feed.xml") != nil {
		t.Error("an entry past the retention window must be dropped")
	}
	if mgr.Get("https://undated.example.com/feed.xml") != nil {
		t.Error("an entry with no last-checked time must be dropped")
	}

	// The prune has to reach disk, or a read-only run would leave it forever.
	onDisk := readCacheFile(t, path)
	if len(onDisk) != 1 {
		t.Errorf("expected 1 entry persisted, got %d", len(onDisk))
	}
}

func TestFeedCacheMigratesLegacyEpisodeRecords(t *testing.T) {
	t.Parallel()
	url := "https://legacy.example.com/feed.xml"
	path := writeCacheFile(t, map[string]*FeedCacheEntry{
		url: {
			FeedURL:     url,
			LastChecked: time.Now(),
			Episodes: []backend.FeedEpisode{
				{Title: "Ep 1", PublishedAt: 1000, DescriptionPlain: "a long description nobody reads back"},
				{Title: "Ep 2", PublishedAt: 2000, DescriptionPlain: "another one"},
			},
		},
	})

	entry := newFeedCacheManager(path).Get(url)
	if len(entry.PubDates) != 2 {
		t.Fatalf("expected 2 publication records, got %d", len(entry.PubDates))
	}
	if entry.PubDates[0].Title != "Ep 1" || entry.PubDates[0].PublishedAt != 1000 {
		t.Errorf("publication record not carried across: %+v", entry.PubDates[0])
	}
	if len(entry.Episodes) != 0 {
		t.Error("the legacy whole-episode records should be dropped once folded in")
	}

	onDisk := readCacheFile(t, path)[url]
	if len(onDisk.Episodes) != 0 || len(onDisk.PubDates) != 2 {
		t.Errorf("migration not persisted: episodes=%d pubdates=%d", len(onDisk.Episodes), len(onDisk.PubDates))
	}
}

func TestFeedCacheEntryFeedEpisodesPrefersPubDates(t *testing.T) {
	t.Parallel()
	entry := &FeedCacheEntry{PubDates: []FeedCachePubDate{{Title: "New", PublishedAt: 7}}}
	eps := entry.FeedEpisodes()
	if len(eps) != 1 || eps[0].Title != "New" || eps[0].PublishedAt != 7 {
		t.Fatalf("unexpected episodes: %+v", eps)
	}

	// A cache written before PubDates existed must still read back.
	legacy := &FeedCacheEntry{Episodes: []backend.FeedEpisode{{Title: "Old", PublishedAt: 3}}}
	if eps := legacy.FeedEpisodes(); len(eps) != 1 || eps[0].Title != "Old" {
		t.Fatalf("legacy read-back failed: %+v", eps)
	}
	if (*FeedCacheEntry)(nil).FeedEpisodes() != nil {
		t.Error("a nil entry should report no episodes")
	}
}

func TestFeedCacheUnchangedFileIsNotRewritten(t *testing.T) {
	t.Parallel()
	url := "https://fresh.example.com/feed.xml"
	path := writeCacheFile(t, map[string]*FeedCacheEntry{
		url: {FeedURL: url, ETag: `"v1"`, LastChecked: time.Now()},
	})
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	mgr := newFeedCacheManager(path)
	if mgr.Get(url) == nil {
		t.Fatal("entry should have loaded")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("a cache needing no prune should not be rewritten")
	}
}

// The sweep replaces an entry wholesale, so it must not discard the
// publication history the frequency analysis keeps there.
func TestFeedSweepPreservesPublicationHistory(t *testing.T) {
	t.Parallel()
	body := feedXML("Mon, 01 Sep 2026 10:00:00 -0000", feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000"))
	srv, _ := etagServer(t, `"v1"`, body)

	cache := newTestCache(t)
	cache.Put(srv.URL, &FeedCacheEntry{
		FeedURL:     srv.URL,
		LastChecked: time.Now(),
		PubDates:    []FeedCachePubDate{{Title: "Ep 1", PublishedAt: 1000}},
	})

	pods := []backend.Podcast{testPodcast("p1", "Show", srv.URL)}
	CheckFeedsForUpdates(pods, buildEpisodeIndexFromPodcasts(nil, pods), checkOpts(cache))

	entry := cache.Get(srv.URL)
	if entry == nil || len(entry.PubDates) != 1 {
		t.Fatalf("publication history lost by the sweep: %+v", entry)
	}
	if entry.ETag == "" {
		t.Error("the sweep should still have recorded the validator")
	}
}
