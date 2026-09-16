package podcast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
