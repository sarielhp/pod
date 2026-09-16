package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPodFetchLoginAndTestConnection(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer my-api-key" || r.Header.Get("x-api-key") == "my-api-key" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": 1, "name": "Test Show"},
			})
			return
		}
		u, p, ok := r.BasicAuth()
		if ok && u == "admin" && p == "secret" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": 1, "name": "Test Show"},
			})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	beAPIKey := newPodFetch(Config{
		Host:   srv.URL,
		APIKey: "my-api-key",
	})
	tok, err := beAPIKey.Login()
	if err != nil || tok != "my-api-key" {
		t.Fatalf("Login with API key failed: %v", err)
	}
	ok, err := beAPIKey.TestConnection(nil)
	if !ok || err != nil {
		t.Fatalf("TestConnection with API key failed: %v", err)
	}

	beBasic := newPodFetch(Config{
		Host: srv.URL,
		User: "admin",
		Pass: "secret",
	})
	tokBasic, err := beBasic.Login()
	if err != nil || tokBasic != "admin" {
		t.Fatalf("Login with Basic auth failed: %v", err)
	}
	okBasic, err := beBasic.TestConnection(nil)
	if !okBasic || err != nil {
		t.Fatalf("TestConnection with Basic auth failed: %v", err)
	}

	beFail := newPodFetch(Config{
		Host: srv.URL,
		User: "admin",
		Pass: "wrong",
	})
	okFail, _ := beFail.TestConnection(nil)
	if okFail {
		t.Errorf("expected TestConnection to fail with invalid credentials")
	}
}

func setupMockPodFetchServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/podcasts" && r.Method == "GET":
			_ = json.NewEncoder(w).Encode([]podFetchItemDTO{
				{ID: 1, Name: "Tech Show", Directory: "tech_show", RSSFeed: "https://example.com/tech.xml", ImageURL: "https://example.com/tech.jpg", Summary: "Tech summary", Author: "Host 1"},
			})
		case r.URL.Path == "/api/v1/podcasts/1" && r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 1, "name": "Tech Show", "directory": "tech_show", "rssfeed": "https://example.com/tech.xml", "image_url": "https://example.com/tech.jpg", "summary": "Tech summary", "author": "Host 1",
				"episodes": []podFetchEpisodeDTO{
					{ID: 101, PodcastID: 1, EpisodeID: "ep-101", Name: "Episode 101", URL: "https://example.com/ep101.mp3", DateOfRecording: "2026-08-30", TotalTime: 3600, LocalURL: "ep101.mp3", Description: "Ep 101 description", Status: "D"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestPodFetchLibrariesAndPodcasts(t *testing.T) {
	t.Parallel()
	srv := setupMockPodFetchServer()
	defer srv.Close()

	be := newPodFetch(Config{Host: srv.URL, APIKey: "key-123"})

	libs, err := be.PodcastLibraries()
	if err != nil || len(libs) == 0 || libs[0].MediaType != "podcast" {
		t.Fatalf("PodcastLibraries failed: %v, libs: %+v", err, libs)
	}

	podcasts, err := be.Podcasts()
	if err != nil || len(podcasts) != 1 || podcasts[0].ID != "1" || podcasts[0].Media.Metadata.Title != "Tech Show" {
		t.Fatalf("Podcasts failed: %v, list: %+v", err, podcasts)
	}
	if len(podcasts[0].Media.Episodes) != 1 || podcasts[0].Media.Episodes[0].Title != "Episode 101" {
		t.Errorf("unexpected episodes in podcast: %+v", podcasts[0].Media.Episodes)
	}

	pod, err := be.GetPodcast("1")
	if err != nil || pod == nil || pod.Media.Metadata.Title != "Tech Show" {
		t.Fatalf("GetPodcast failed: %v", err)
	}
}
