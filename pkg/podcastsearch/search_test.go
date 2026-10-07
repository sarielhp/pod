package podcastsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestITunesSearchMapsAndFilters(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("media") != "podcast" || r.URL.Query().Get("entity") != "podcast" {
			t.Errorf("search must ask for podcasts: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"resultCount":3,"results":[
			{"collectionId":123,"collectionName":"A Show","artistName":"Someone","feedUrl":"http://feed/a","artworkUrl600":"http://img/a","trackCount":10,"collectionViewUrl":"http://apple/a"},
			{"collectionId":456,"collectionName":"No Feed","artistName":"Nobody"},
			{"collectionId":0,"collectionName":"Zero Id","feedUrl":"http://feed/z"}
		]}`))
	}))
	defer srv.Close()

	client := NewITunes()
	client.searchURL = srv.URL

	results, err := client.Search(context.Background(), "a show", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (entries without a feed or id are dropped)", len(results))
	}
	r := results[0]
	if r.ID != "123" || r.Title != "A Show" || r.Author != "Someone" || r.FeedURL != "http://feed/a" {
		t.Errorf("unexpected result: %+v", r)
	}
	if r.ImageURL != "http://img/a" || r.EpisodeCount != 10 || r.Source != "Apple Podcasts" {
		t.Errorf("unexpected artwork/count/source: %+v", r)
	}
}

func TestITunesByID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "123" {
			t.Errorf("lookup id = %q, want 123", r.URL.Query().Get("id"))
		}
		_, _ = w.Write([]byte(`{"results":[{"collectionId":123,"collectionName":"A Show","feedUrl":"http://feed/a"}]}`))
	}))
	defer srv.Close()

	client := NewITunes()
	client.lookupURL = srv.URL
	r, err := client.ByID(context.Background(), "123")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if r.FeedURL != "http://feed/a" {
		t.Errorf("unexpected result: %+v", r)
	}
	if _, err := client.ByID(context.Background(), "abc"); err == nil {
		t.Error("expected an error for a non-numeric id")
	}
}

func TestNewSelectsDirectoryFromCredentials(t *testing.T) {
	t.Parallel()
	if _, ok := New(Credentials{APIKey: "k", APISecret: "s"}).(*podcastIndex); !ok {
		t.Error("credentials should select Podcast Index")
	}
	if _, ok := New(Credentials{}).(*itunes); !ok {
		t.Error("no credentials should select the keyless directory")
	}
	if _, ok := New(Credentials{APIKey: "k"}).(*itunes); !ok {
		t.Error("a lone half is not enough to select Podcast Index")
	}
}

func TestResolveFeedURLNeedsNoDirectory(t *testing.T) {
	t.Parallel()
	r, err := Resolve(context.Background(), Credentials{}, "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.FeedURL != "https://example.com/feed.xml" {
		t.Errorf("a feed URL should be accepted as-is, got %+v", r)
	}
}

func TestClampMax(t *testing.T) {
	t.Parallel()
	cases := map[int]int{0: DefaultMax, -3: DefaultMax, 5: 5, 100: 100, 500: 100}
	for in, want := range cases {
		if got := ClampMax(in); got != want {
			t.Errorf("ClampMax(%d) = %d, want %d", in, got, want)
		}
	}
}
