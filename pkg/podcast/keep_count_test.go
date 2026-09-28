package podcast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/util"
)

func countPruneBackend(t *testing.T) (*StandaloneBackend, string) {
	t.Helper()
	podcastsDir := t.TempDir()
	store, err := NewSubscriptionStore(filepath.Join(t.TempDir(), "podcasts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Subscription{ID: "show", Title: "Show", FeedURL: "https://example.com/f.xml", Folder: "Show"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(podcastsDir, "Show")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-10 * time.Hour)
	for i, name := range []string{"old.mp3", "mid.mp3", "new.mp3"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
		ts := base.Add(time.Duration(i) * time.Hour)
		_ = os.Chtimes(p, ts, ts)
	}
	return &StandaloneBackend{cfg: backend.Config{PodcastsDir: podcastsDir}, store: store, downloader: NewDownloader()}, dir
}

func TestApplyKeepPolicyCountReportsAFailedDelete(t *testing.T) {
	b, dir := countPruneBackend(t)
	lock, err := util.AcquireFileLock(filepath.Join(dir, "old.mp3"))
	if err != nil || lock == nil {
		t.Fatalf("test lock: %v", err)
	}
	defer lock.Release()

	deleted, err := b.ApplyKeepPolicy("show", "Show", 1, false)
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1 (mid.mp3 only; old.mp3 is locked by another instance)", deleted)
	}
	if err == nil || !strings.Contains(err.Error(), "old.mp3") {
		t.Fatalf("the locked episode's failure was not reported: %v", err)
	}
	if !util.FileExists(filepath.Join(dir, "old.mp3")) {
		t.Fatal("a locked episode was deleted from under its worker")
	}
	if util.FileExists(filepath.Join(dir, "mid.mp3")) || !util.FileExists(filepath.Join(dir, "new.mp3")) {
		t.Fatal("wrong episodes were pruned")
	}
}

func TestApplyKeepPolicyCountSkipsAnEpisodeInRemoteFlight(t *testing.T) {
	b, dir := countPruneBackend(t)
	status := `{"version":1,"status":"transcribing_remotely"}`
	if err := os.WriteFile(filepath.Join(dir, "old.mp3.json"), []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
	deleted, err := b.ApplyKeepPolicy("show", "Show", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || !util.FileExists(filepath.Join(dir, "old.mp3")) {
		t.Fatalf("deleted = %d; an episode being transcribed remotely must not be pruned", deleted)
	}
}
