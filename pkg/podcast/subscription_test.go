package podcast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubscriptionStoreRejectsInvalidFeedURLs(t *testing.T) {
	t.Parallel()
	store, err := NewSubscriptionStore(filepath.Join(t.TempDir(), "podcasts.json"))
	if err != nil {
		t.Fatal(err)
	}
	original := Subscription{ID: "show", Title: "Show", FeedURL: "https://example.com/feed.xml"}
	if err := store.Add(original); err != nil {
		t.Fatal(err)
	}
	for _, feedURL := range []string{"8056024", "example.com/feed.xml", "/feed.xml", "//example.com/feed.xml", "ftp://example.com/feed.xml", "https:///feed.xml", "https://:8080/feed.xml", "http://example.com:bad/feed.xml", "https://%xx/feed.xml", ""} {
		t.Run(feedURL, func(t *testing.T) {
			sub := original
			sub.FeedURL = feedURL
			if err := store.Add(sub); err == nil || !strings.Contains(err.Error(), "feed URL") {
				t.Fatalf("Add(%q) error = %v", feedURL, err)
			}
			items := store.List()
			if len(items) != 1 || items[0].FeedURL != original.FeedURL {
				t.Fatalf("invalid feed changed subscriptions: %+v", items)
			}
		})
	}
	for _, feedURL := range []string{"http://localhost:8080/feed.xml", " https://example.com/rss?format=xml "} {
		sub := original
		sub.FeedURL = feedURL
		if err := store.Add(sub); err != nil {
			t.Fatalf("Add(%q): %v", feedURL, err)
		}
		if got := store.List()[0].FeedURL; got != strings.TrimSpace(feedURL) {
			t.Fatalf("saved URL = %q", got)
		}
	}
}

func TestSubscriptionStoreCRUD(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "podcasts.json")

	store, err := NewSubscriptionStore(storeFile)
	if err != nil {
		t.Fatalf("NewSubscriptionStore failed: %v", err)
	}

	if len(store.List()) != 0 {
		t.Fatalf("expected 0 items initially, got %d", len(store.List()))
	}

	sub := Subscription{
		Title:   "Hardcore History",
		FeedURL: "https://feed.dancarlin.com/hh.xml",
	}
	if err := store.Add(sub); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	if len(store.List()) != 1 {
		t.Fatalf("expected 1 item, got %d", len(store.List()))
	}

	found := store.Get("hardcore history")
	if found == nil {
		t.Fatalf("expected to find subscription by title")
	}
	if found.FeedURL != sub.FeedURL {
		t.Errorf("expected feed URL %s, got %s", sub.FeedURL, found.FeedURL)
	}
	if found.ID == "" {
		t.Errorf("expected generated ID")
	}

	if err := store.Save(); err != nil {
		t.Fatalf("store.Save failed: %v", err)
	}

	// Reload from disk
	loadedStore, err := NewSubscriptionStore(storeFile)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(loadedStore.List()) != 1 {
		t.Fatalf("expected 1 item after reload, got %d", len(loadedStore.List()))
	}

	// Remove
	removed, err := loadedStore.Remove(found.ID)
	if err != nil || !removed {
		t.Fatalf("failed to remove: %v", err)
	}
	if len(loadedStore.List()) != 0 {
		t.Fatalf("expected 0 items after remove, got %d", len(loadedStore.List()))
	}
}

func TestSubscriptionStoreImportOPML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "podcasts.json")

	store, err := NewSubscriptionStore(storeFile)
	if err != nil {
		t.Fatalf("NewSubscriptionStore failed: %v", err)
	}

	opmlXML := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="1.0">
  <head><title>Feeds</title></head>
  <body>
    <outline text="RadioLab" title="RadioLab" type="rss" xmlUrl="https://feeds.example.com/radiolab.xml"/>
    <outline text="The Daily" title="The Daily" type="rss" xmlUrl="https://feeds.example.com/daily.xml"/>
  </body>
</opml>`

	count, err := store.ImportFromOPML([]byte(opmlXML))
	if err != nil {
		t.Fatalf("ImportFromOPML failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported feeds, got %d", count)
	}

	data, err := os.ReadFile(storeFile)
	if err != nil || len(data) == 0 {
		t.Fatalf("expected saved file at %s", storeFile)
	}
}
