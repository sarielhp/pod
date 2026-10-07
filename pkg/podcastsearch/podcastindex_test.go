package podcastsearch

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestPodcastIndexSearchSignsAndParses(t *testing.T) {
	t.Parallel()
	const (
		apiKey    = "KEY123"
		apiSecret = "SECRET456"
		authDate  = int64(1700000000)
	)
	var gotPath, gotAuth, gotKey, gotDate, gotQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("X-Auth-Key")
		gotDate = r.Header.Get("X-Auth-Date")
		gotQ = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"status":"true","count":1,"feeds":[{"id":75075,"title":"The Daily","url":"http://feed.example/x","originalUrl":"http://orig.example/x","image":"http://img.example/i.png","author":"The New York Times","episodeCount":2100}]}`))
	}))
	defer srv.Close()

	client := NewPodcastIndex(apiKey, apiSecret)
	client.baseURL = srv.URL
	client.now = func() time.Time { return time.Unix(authDate, 0) }

	results, err := client.Search(context.Background(), "the daily", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/search/byterm" {
		t.Errorf("path = %q, want /search/byterm", gotPath)
	}
	if gotQ != "the daily" {
		t.Errorf("q = %q, want %q", gotQ, "the daily")
	}
	sum := sha1.Sum([]byte(apiKey + apiSecret + strconv.FormatInt(authDate, 10)))
	if want := hex.EncodeToString(sum[:]); gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if gotKey != apiKey || gotDate != strconv.FormatInt(authDate, 10) {
		t.Errorf("auth headers = key %q date %q", gotKey, gotDate)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	r := results[0]
	if r.ID != "75075" || r.Title != "The Daily" || r.Author != "The New York Times" {
		t.Errorf("unexpected result: %+v", r)
	}
	if r.FeedURL != "http://orig.example/x" {
		t.Errorf("FeedURL = %q; originalUrl should win over url", r.FeedURL)
	}
	if r.ImageURL != "http://img.example/i.png" || r.EpisodeCount != 2100 {
		t.Errorf("unexpected artwork/count: %+v", r)
	}
}

func TestPodcastIndexReportsItsOwnFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"false","description":"invalid API key"}`))
	}))
	defer srv.Close()

	client := NewPodcastIndex("k", "s")
	client.baseURL = srv.URL
	if _, err := client.Search(context.Background(), "x", 1); err == nil {
		t.Fatal("expected an error when status is false")
	}
}

func TestPodcastIndexByIDAndURL(t *testing.T) {
	t.Parallel()
	var lastPath, lastID, lastURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		lastID = r.URL.Query().Get("id")
		lastURL = r.URL.Query().Get("url")
		_, _ = w.Write([]byte(`{"status":"true","feed":{"id":42,"title":"Show","url":"http://f/x"}}`))
	}))
	defer srv.Close()

	client := NewPodcastIndex("k", "s")
	client.baseURL = srv.URL

	byID, err := client.ByID(context.Background(), "42")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if lastPath != "/podcasts/byfeedid" || lastID != "42" {
		t.Errorf("ByID hit %s?id=%s", lastPath, lastID)
	}
	if byID.ID != "42" || byID.Title != "Show" {
		t.Errorf("unexpected ByID result: %+v", byID)
	}

	if _, err := client.ByURL(context.Background(), "http://f/x"); err != nil {
		t.Fatalf("ByURL: %v", err)
	}
	if lastPath != "/podcasts/byfeedurl" || lastURL != "http://f/x" {
		t.Errorf("ByURL hit %s?url=%s", lastPath, lastURL)
	}

	if _, err := client.ByID(context.Background(), "not-a-number"); err == nil {
		t.Error("expected an error for a non-numeric id")
	}
}

func TestPodcastIndexRequiresBothCredentials(t *testing.T) {
	t.Parallel()
	if !(Credentials{APIKey: "k", APISecret: "s"}).Configured() {
		t.Error("a full pair should be configured")
	}
	if (Credentials{APIKey: "k"}).Configured() || (Credentials{APISecret: "s"}).Configured() {
		t.Error("a lone half is not a usable credential")
	}
}
