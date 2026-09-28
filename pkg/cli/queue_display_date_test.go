package cli

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/podcast"
)

func TestQueueDisplayUsesTheStatusFileDateWhenTheCacheHasNone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "2026-09-28_ICYMI.mp3")
	if err := os.WriteFile(mp3, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	status := `{"version":1,"status":"downloaded","published_at":"2026-09-28T02:25:33Z","publication_source":"feed"}`
	if err := os.WriteFile(mp3+".json", []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "queue.json"), []byte(`["2026-09-28_ICYMI.mp3"]`), 0644); err != nil {
		t.Fatal(err)
	}
	items, err := collectQueueDisplayItems(podcast.PodcastDirEntry{Dir: dir, ShortID: "fdwrf"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d rows", len(items))
	}
	if items[0].PublishedAt != "2026-09-28T02:25:33Z" {
		t.Fatalf("a freshly downloaded episode with a stamped published_at shows P-date %q", items[0].PublishedAt)
	}
}
