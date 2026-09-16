package backend

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBackendRegistry(t *testing.T) {
	t.Parallel()
	b3, err := New("podfetch", Config{Host: "http://localhost:8000"})
	if err != nil {
		t.Fatalf("failed to create backend 'podfetch': %v", err)
	}
	if b3.Name() != "podfetch" {
		t.Errorf("expected name podfetch, got %s", b3.Name())
	}

	b4, err := New("pod_fetch", Config{Host: "http://localhost:8000"})
	if err != nil {
		t.Fatalf("failed to create backend by alias 'pod_fetch': %v", err)
	}
	if b4.Name() != "podfetch" {
		t.Errorf("expected name podfetch, got %s", b4.Name())
	}

	_, err = New("non_existent", Config{})
	if err == nil {
		t.Errorf("expected error for non existent backend")
	}
}

func TestFeedEpisodeUnmarshalJSON(t *testing.T) {
	t.Parallel()
	jsonData := `{
		"title": "Test Title",
		"pubDate": "Mon, 31 Aug 2026 12:00:00 GMT",
		"publishedAt": "1725105600000",
		"season": {"number": 2},
		"episode": 5,
		"enclosureUrl": "https://example.com/audio.mp3"
	}`

	var ep FeedEpisode
	if err := json.Unmarshal([]byte(jsonData), &ep); err != nil {
		t.Fatalf("failed to unmarshal FeedEpisode: %v", err)
	}

	if ep.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %s", ep.Title)
	}
	if ep.PublishedAt != 1725105600000 {
		t.Errorf("expected publishedAt 1725105600000, got %d", ep.PublishedAt)
	}
	if ep.Season != "2" {
		t.Errorf("expected season '2', got %s", ep.Season)
	}
	if ep.Episode != "5" {
		t.Errorf("expected episode '5', got %s", ep.Episode)
	}
	if ep.Enclosure == nil || ep.Enclosure.URL != "https://example.com/audio.mp3" {
		t.Errorf("expected enclosure URL 'https://example.com/audio.mp3', got %+v", ep.Enclosure)
	}
}

func TestAnalyzePodcastFrequency(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()
	dayMs := int64(24 * 3600 * 1000)

	dailyEps := []FeedEpisode{
		{PublishedAt: now},
		{PublishedAt: now - dayMs},
		{PublishedAt: now - 2*dayMs},
		{PublishedAt: now - 3*dayMs},
		{PublishedAt: now - 4*dayMs},
		{PublishedAt: now - 5*dayMs},
		{PublishedAt: now - 6*dayMs},
		{PublishedAt: now - 7*dayMs},
		{PublishedAt: now - 8*dayMs},
		{PublishedAt: now - 9*dayMs},
		{PublishedAt: now - 10*dayMs},
	}

	info := AnalyzePodcastFrequency(dailyEps)
	if info.Type != string(CadenceDaily) {
		t.Errorf("expected daily cadence, got %s", info.Type)
	}

	fewEps := []FeedEpisode{
		{PublishedAt: now},
		{PublishedAt: now - dayMs},
	}
	infoFew := AnalyzePodcastFrequency(fewEps)
	if infoFew.Type != string(CadenceIntermittent) {
		t.Errorf("expected intermittent for few episodes, got %s", infoFew.Type)
	}
}
