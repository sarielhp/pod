package podcast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
)

func feedXML(lastBuild string, items ...string) string {
	body := "<rss><channel><title>Show</title>"
	if lastBuild != "" {
		body += "<lastBuildDate>" + lastBuild + "</lastBuildDate>"
	}
	for _, it := range items {
		body += it
	}
	return body + "</channel></rss>"
}

func feedItem(title, guid, pubDate string) string {
	return fmt.Sprintf(
		"<item><title>%s</title><guid>%s</guid><pubDate>%s</pubDate>"+
			`<enclosure url="https://cdn.example.com/%s.mp3" type="audio/mpeg"/></item>`,
		title, guid, pubDate, guid)
}

func testPodcast(id, title, feedURL string, episodes ...backend.Episode) backend.Podcast {
	return backend.Podcast{
		ID: id,
		Media: backend.PodcastMedia{
			ID:       id,
			Metadata: backend.PodcastMetadata{Title: title, FeedURL: feedURL},
			Episodes: episodes,
		},
	}
}

func newTestCache(t *testing.T) *FeedCacheManager {
	t.Helper()
	return newFeedCacheManager(filepath.Join(t.TempDir(), "feed_cache.json"))
}

func checkOpts(cache *FeedCacheManager) FeedCheckOptions {
	return FeedCheckOptions{Cache: cache, Concurrency: 2, Timeout: 5 * time.Second, MaxAttempts: 1}
}

// etagServer serves a feed with an ETag and honours If-None-Match, which is how
// the large majority of real podcast hosts behave.
func etagServer(t *testing.T, etag, body string) (*httptest.Server, *int) {
	t.Helper()
	bodyServed := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		bodyServed++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &bodyServed
}

func TestCheckFeedsForUpdatesUsesConditionalGet(t *testing.T) {
	t.Parallel()
	body := feedXML("Mon, 01 Sep 2026 10:00:00 -0000", feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000"))
	srv, bodyServed := etagServer(t, `"v1"`, body)

	pods := []backend.Podcast{testPodcast("p1", "Show", srv.URL,
		backend.Episode{GUID: "g1", Title: "Ep 1", AudioFile: &backend.PodcastAudioFile{Duration: 1}})}
	index := buildEpisodeIndexFromPodcasts(nil, pods)
	cache := newTestCache(t)

	first := CheckFeedsForUpdates(pods, index, checkOpts(cache))
	if len(first) != 1 || first[0].Status != FeedChanged {
		t.Fatalf("first sweep: expected FeedChanged, got %+v", first[0])
	}
	if len(first[0].New) != 0 {
		t.Errorf("episode already in the catalog should not count as new: %+v", first[0].New)
	}
	if first[0].NeedsServer() {
		t.Error("no new episodes means the server needs no refresh")
	}

	second := CheckFeedsForUpdates(pods, index, checkOpts(cache))
	if second[0].Status != FeedUnchanged {
		t.Fatalf("second sweep: expected FeedUnchanged, got %v (%s)", second[0].Status, second[0].Reason)
	}
	if second[0].NeedsServer() {
		t.Error("an unchanged feed must never trigger server work")
	}
	if *bodyServed != 1 {
		t.Errorf("expected the body to be transferred once, got %d transfers", *bodyServed)
	}
}

func TestCheckFeedsForUpdatesDetectsNewEpisode(t *testing.T) {
	t.Parallel()
	older := feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000")
	newer := feedItem("Ep 2", "g2", "Tue, 02 Sep 2026 10:00:00 -0000")

	version := `"v1"`
	body := feedXML("Mon, 01 Sep 2026 10:00:00 -0000", older)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", version)
		if r.Header.Get("If-None-Match") == version {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	pods := []backend.Podcast{testPodcast("p1", "Show", srv.URL,
		backend.Episode{GUID: "g1", Title: "Ep 1", AudioFile: &backend.PodcastAudioFile{Duration: 1}})}
	index := buildEpisodeIndexFromPodcasts(nil, pods)
	cache := newTestCache(t)

	CheckFeedsForUpdates(pods, index, checkOpts(cache))

	version = `"v2"`
	body = feedXML("Tue, 02 Sep 2026 10:00:00 -0000", newer, older)

	res := CheckFeedsForUpdates(pods, index, checkOpts(cache))
	if res[0].Status != FeedChanged {
		t.Fatalf("expected FeedChanged after publication, got %v", res[0].Status)
	}
	if len(res[0].New) != 1 || res[0].New[0].GUID != "g2" {
		t.Fatalf("expected exactly the new episode g2, got %+v", res[0].New)
	}
	if !res[0].NeedsServer() {
		t.Error("a feed with an episode the catalog lacks must need server work")
	}
}

// Roughly one feed in six serves no usable validator, so the content markers
// are the only thing standing between an unchanged feed and needless work.
func TestCheckFeedsForUpdatesFallsBackToContentMarkers(t *testing.T) {
	t.Parallel()
	served := 0
	body := feedXML("Mon, 01 Sep 2026 10:00:00 -0000", feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served++
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	pods := []backend.Podcast{testPodcast("p1", "Show", srv.URL)}
	index := buildEpisodeIndexFromPodcasts(nil, pods)
	cache := newTestCache(t)

	if res := CheckFeedsForUpdates(pods, index, checkOpts(cache)); res[0].Status != FeedChanged {
		t.Fatalf("first sight of a feed is always a change, got %v", res[0].Status)
	}
	res := CheckFeedsForUpdates(pods, index, checkOpts(cache))
	if res[0].Status != FeedUnchanged {
		t.Fatalf("identical markers should read as unchanged, got %v (%s)", res[0].Status, res[0].Reason)
	}
	if served != 2 {
		t.Errorf("without a validator the body must be fetched each time, got %d fetches", served)
	}
}

func TestCheckFeedsForUpdatesForceIgnoresCache(t *testing.T) {
	t.Parallel()
	body := feedXML("Mon, 01 Sep 2026 10:00:00 -0000", feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000"))
	srv, bodyServed := etagServer(t, `"v1"`, body)

	pods := []backend.Podcast{testPodcast("p1", "Show", srv.URL)}
	index := buildEpisodeIndexFromPodcasts(nil, pods)
	cache := newTestCache(t)

	CheckFeedsForUpdates(pods, index, checkOpts(cache))

	opts := checkOpts(cache)
	opts.Force = true
	if res := CheckFeedsForUpdates(pods, index, opts); res[0].Status != FeedChanged {
		t.Fatalf("--force must re-read the feed, got %v", res[0].Status)
	}
	if *bodyServed != 2 {
		t.Errorf("expected 2 body transfers under --force, got %d", *bodyServed)
	}
}

func TestCheckFeedsForUpdatesUnreadableFeedFallsBackToServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	pods := []backend.Podcast{
		testPodcast("p1", "Gone", srv.URL),
		testPodcast("p2", "No Feed", ""),
	}
	index := buildEpisodeIndexFromPodcasts(nil, pods)

	res := CheckFeedsForUpdates(pods, index, checkOpts(newTestCache(t)))
	for _, r := range res {
		if r.Status != FeedUnknown {
			t.Errorf("%s: expected FeedUnknown, got %v", r.Title, r.Status)
		}
		if r.Err == nil {
			t.Errorf("%s: expected an error to report", r.Title)
		}
		if !r.NeedsServer() {
			t.Errorf("%s: a feed we cannot read should be handed to the server", r.Title)
		}
	}
}

func TestCheckFeedsForUpdatesEmptyInput(t *testing.T) {
	t.Parallel()
	if res := CheckFeedsForUpdates(nil, EpisodeIndex{}, checkOpts(newTestCache(t))); len(res) != 0 {
		t.Fatalf("expected no results, got %d", len(res))
	}
}

func TestCheckFeedsForUpdatesCatchesUnindexedEpisodeEvenIfOriginUnchanged(t *testing.T) {
	t.Parallel()
	ep1 := feedItem("Ep 1", "g1", "Mon, 01 Sep 2026 10:00:00 -0000")
	ep2 := feedItem("Ep 2", "g2", "Tue, 02 Sep 2026 10:00:00 -0000")
	body := feedXML("Tue, 02 Sep 2026 10:00:00 -0000", ep2, ep1)
	srv, _ := etagServer(t, `"v2"`, body)

	pods := []backend.Podcast{testPodcast("p1", "Show", srv.URL,
		backend.Episode{GUID: "g1", Title: "Ep 1", AudioFile: &backend.PodcastAudioFile{Duration: 1}})}
	index := buildEpisodeIndexFromPodcasts(nil, pods)

	cache := newTestCache(t)
	cache.Put(srv.URL, &FeedCacheEntry{
		FeedURL:       srv.URL,
		ETag:          `"v2"`,
		LatestGUID:    "g2",
		EpisodeCount:  2,
		LastBuildDate: "Tue, 02 Sep 2026 10:00:00 -0000",
		LastChecked:   time.Now(),
	})

	res := CheckFeedsForUpdates(pods, index, checkOpts(cache))
	if len(res) != 1 || len(res[0].New) != 1 || res[0].New[0].GUID != "g2" {
		t.Fatalf("expected new episode g2, got %+v", res)
	}
	if !res[0].NeedsServer() {
		t.Error("expected NeedsServer() to be true when catalog lacks latest episode")
	}
}
