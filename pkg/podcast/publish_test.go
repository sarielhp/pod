package podcast

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/types"
)

func TestPublishPodcastEmitsContentHashTokenIdempotently(t *testing.T) {
	t.Parallel()

	podDir := t.TempDir()
	audio := filepath.Join(podDir, "2026-01-01_abcdef0123.mp3")
	if err := os.WriteFile(audio, []byte("ad-free episode audio"), 0644); err != nil {
		t.Fatal(err)
	}
	st := types.EpisodeStatusFile{
		MediaFile: filepath.Base(audio),
		Status:    types.StateDone,
		Cleaned:   types.EpisodeAudioMeta{Filename: filepath.Base(audio), DurationSec: 60},
	}
	data, _ := json.Marshal(st)
	_ = os.WriteFile(audio+".json", data, 0644)

	sub := Subscription{ID: "s", Title: "Show", Folder: filepath.Base(podDir)}
	if err := PublishPodcast(podDir, sub, "http://srv:8080/podcasts", nil); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(podDir, "feed.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "?v=") {
		t.Fatalf("feed enclosure missing ?v= token:\n%s", first)
	}

	// Republishing with unchanged audio must not churn the feed.
	if err := PublishPodcast(podDir, sub, "http://srv:8080/podcasts", nil); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(podDir, "feed.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("republish changed an unchanged feed:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestGenerateCatalogWebpage(t *testing.T) {
	t.Parallel()
	podcastsDir := t.TempDir()
	subs := []Subscription{
		{
			ID:      "show1",
			Title:   "Alpha Show",
			FeedURL: "https://example.com/alpha.xml",
			Folder:  "Alpha Show",
		},
		{
			ID:      "show2",
			Title:   "Beta Show",
			FeedURL: "https://example.com/beta.xml",
			Folder:  "Beta Show",
		},
	}

	htmlData, err := generateCatalogWebpageHTML(podcastsDir, subs)
	if err != nil {
		t.Fatalf("generateCatalogWebpageHTML failed: %v", err)
	}

	content := string(htmlData)
	if !strings.Contains(content, "Alpha Show") || !strings.Contains(content, "Beta Show") {
		t.Errorf("expected catalog HTML to contain both show titles")
	}

	if err := PublishCatalog(podcastsDir, subs); err != nil {
		t.Fatalf("PublishCatalog failed: %v", err)
	}

	idxPath := filepath.Join(podcastsDir, "index.html")
	if _, err := os.Stat(idxPath); err != nil {
		t.Fatalf("expected catalog index.html to exist at %s: %v", idxPath, err)
	}
}
