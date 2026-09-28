package podcast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertInsidePodcastsDir(t *testing.T, podcastsDir, dir string) {
	t.Helper()
	rel, err := filepath.Rel(podcastsDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		t.Fatalf("podcast dir %s resolved outside %s", dir, podcastsDir)
	}
}

func TestSubscriptionFolderCannotEscapePodcastsDir(t *testing.T) {
	t.Parallel()
	podcastsDir := filepath.Join(t.TempDir(), "podcasts")
	outside := filepath.Join(filepath.Dir(podcastsDir), "outside")
	for _, d := range []string{podcastsDir, outside} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	storePath := filepath.Join(t.TempDir(), "podcasts.json")
	raw := `{"version":1,"subscriptions":[{"id":"evil","title":"Evil Show","feed_url":"https://example.com/evil.xml","folder":"../outside"}]}`
	if err := os.WriteFile(storePath, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := NewSubscriptionStore(storePath)
	if err != nil {
		t.Fatalf("NewSubscriptionStore: %v", err)
	}
	loaded := store.List()
	if len(loaded) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(loaded))
	}
	if strings.Contains(loaded[0].Folder, "..") {
		t.Fatalf("a folder loaded from podcasts.json still escapes: %q", loaded[0].Folder)
	}
	assertInsidePodcastsDir(t, podcastsDir, resolvePodcastDirForSub(loaded[0], podcastsDir))

	escaping := Subscription{ID: "evil", Title: "Evil Show", FeedURL: "https://example.com/evil.xml", Folder: "../outside"}
	assertInsidePodcastsDir(t, podcastsDir, resolvePodcastDirForSub(escaping, podcastsDir))

	if err := store.Add(Subscription{Title: "Other Show", FeedURL: "https://example.com/other.xml", Folder: "/../../etc"}); err != nil {
		t.Fatal(err)
	}
	other := store.Get("other show")
	if other == nil || other.Folder == "" || strings.Contains(other.Folder, "..") || filepath.IsAbs(other.Folder) {
		t.Fatalf("Add kept an escaping folder: %+v", other)
	}

	if err := store.Add(Subscription{Title: "Fine", FeedURL: "https://example.com/fine.xml", Folder: "Nested/Show"}); err != nil {
		t.Fatal(err)
	}
	if fine := store.Get("fine"); fine == nil || fine.Folder != "Nested/Show" {
		t.Fatalf("a contained folder must survive untouched: %+v", fine)
	}
}
