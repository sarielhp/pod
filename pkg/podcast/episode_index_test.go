package podcast

import (
	"testing"

	"pod/pkg/backend"
)

func TestParseRSSFeedCapturesChannelMarkers(t *testing.T) {
	t.Parallel()
	raw := []byte(feedXML("Mon, 01 Sep 2026 10:00:00 -0000",
		feedItem("Ep 2", "g2", "Tue, 02 Sep 2026 10:00:00 -0000"),
		feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000")))

	doc, err := parseRSSFeed(raw)
	if err != nil {
		t.Fatalf("parseRSSFeed failed: %v", err)
	}
	if doc.Title != "Show" {
		t.Errorf("title: got %q", doc.Title)
	}
	if doc.LastBuildDate != "Mon, 01 Sep 2026 10:00:00 -0000" {
		t.Errorf("lastBuildDate: got %q", doc.LastBuildDate)
	}
	if len(doc.Episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(doc.Episodes))
	}
	if guid := doc.LatestGUID(); guid != "g2" {
		t.Errorf("LatestGUID should follow the newest pubDate, got %q", guid)
	}
}

func TestParseRSSFeedLatestGUIDIgnoresFeedOrder(t *testing.T) {
	t.Parallel()
	// Some publishers list episodes oldest first, so the newest entry cannot
	// be assumed to be the first one.
	raw := []byte(feedXML("",
		feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000"),
		feedItem("Ep 3", "g3", "Wed, 03 Sep 2026 10:00:00 -0000")))

	doc, err := parseRSSFeed(raw)
	if err != nil {
		t.Fatalf("parseRSSFeed failed: %v", err)
	}
	if guid := doc.LatestGUID(); guid != "g3" {
		t.Errorf("expected g3, got %q", guid)
	}
}

func TestEpisodeIndexMatchesAcrossIdentifiers(t *testing.T) {
	t.Parallel()
	pods := []backend.Podcast{testPodcast("p1", "Show",
		"https://example.com/feed.xml",
		backend.Episode{GUID: "g1", Title: "First Episode", EnclosureURL: "http://cdn.example.com/1.mp3"},
	)}
	index := buildEpisodeIndexFromPodcasts(nil, pods)
	idx := index["p1"]

	cases := []struct {
		name string
		ep   backend.FeedEpisode
	}{
		{"by guid", backend.FeedEpisode{GUID: "g1"}},
		{"by title", backend.FeedEpisode{Title: "first episode"}},
		// The parser upgrades enclosure URLs to https, so a catalog URL
		// recorded as http must still compare equal.
		{"by https-upgraded url", backend.FeedEpisode{EnclosureURL: "https://cdn.example.com/1.mp3"}},
	}
	for _, tc := range cases {
		if !idx.Knows(tc.ep) {
			t.Errorf("%s: expected the catalog to recognise the episode", tc.name)
		}
	}
	if idx.Knows(backend.FeedEpisode{GUID: "g9", Title: "Unheard"}) {
		t.Error("an unrelated episode must not match")
	}
	if unknown := index.Unknown("p1", []backend.FeedEpisode{{GUID: "g1"}, {GUID: "g9"}}); len(unknown) != 1 {
		t.Errorf("expected exactly one unknown episode, got %d", len(unknown))
	}
}

func TestEpisodeIndexPendingCountsMissingAudio(t *testing.T) {
	t.Parallel()
	pods := []backend.Podcast{testPodcast("p1", "Show", "https://example.com/feed.xml",
		backend.Episode{GUID: "g1", Title: "Downloaded", AudioFile: &backend.PodcastAudioFile{Duration: 1}},
		backend.Episode{GUID: "g2", Title: "Catalogued Only"},
	)}

	pending := buildEpisodeIndexFromPodcasts(nil, pods)["p1"]
	if pending.Total() != 2 || pending.Pending() != 1 {
		t.Errorf("total=%d pending=%d, want 2 and 1", pending.Total(), pending.Pending())
	}
}

func TestEpisodeIndexUnknownPodcastTreatsEverythingAsNew(t *testing.T) {
	t.Parallel()
	index := buildEpisodeIndexFromPodcasts(nil, nil)
	unknown := index.Unknown("missing", []backend.FeedEpisode{{GUID: "g1"}, {GUID: "g2"}})
	if len(unknown) != 2 {
		t.Errorf("an unindexed podcast must look entirely new, got %d of 2", len(unknown))
	}
}

func TestBuildEpisodeIndexPrefersBackendCatalog(t *testing.T) {
	t.Parallel()
	pods := []backend.Podcast{testPodcast("p1", "Show", "https://example.com/feed.xml")}
	index := BuildEpisodeIndex(catalogBackend{}, pods)
	idx := index["p1"]

	if !idx.Knows(backend.FeedEpisode{GUID: "catalog-1"}) {
		t.Error("expected the bulk catalog to be indexed")
	}
	if idx.Pending() != 1 {
		t.Errorf("pending=%d, want 1", idx.Pending())
	}
}

// stubBackend embeds the interface so the tests can name just the one or two
// methods each case exercises.
type stubBackend struct{ backend.Backend }

func (stubBackend) Name() string { return "stub" }

type catalogBackend struct{ stubBackend }

func (catalogBackend) CatalogEpisodes() ([]backend.CatalogEpisode, error) {
	return []backend.CatalogEpisode{
		{PodcastID: "p1", GUID: "catalog-1", Title: "Downloaded", Downloaded: true},
		{PodcastID: "p1", GUID: "catalog-2", Title: "Pending"},
	}, nil
}
