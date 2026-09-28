package podcast

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/backend"
)

func newEpisodeDeleteBackend(t *testing.T) (*StandaloneBackend, string) {
	t.Helper()
	podcastsDir := t.TempDir()
	store, err := NewSubscriptionStore(filepath.Join(t.TempDir(), "podcasts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Subscription{ID: "show1", Title: "Great Podcast", FeedURL: "https://example.com/great.xml", Folder: "Great Podcast"}); err != nil {
		t.Fatal(err)
	}
	showDir := filepath.Join(podcastsDir, "Great Podcast")
	if err := os.MkdirAll(showDir, 0755); err != nil {
		t.Fatal(err)
	}
	epPath := filepath.Join(showDir, "Episode 1.mp3")
	if err := os.WriteFile(epPath, []byte("fake mp3 data"), 0644); err != nil {
		t.Fatal(err)
	}
	be := &StandaloneBackend{cfg: backend.Config{PodcastsDir: podcastsDir}, store: store, downloader: NewDownloader()}
	return be, epPath
}

func TestDeletePodcastEpisodeRemovesTheAudioFile(t *testing.T) {
	t.Parallel()
	be, epPath := newEpisodeDeleteBackend(t)
	p, err := be.GetPodcast("show1")
	if err != nil || p == nil || len(p.Media.Episodes) != 1 {
		t.Fatalf("GetPodcast: %v, %+v", err, p)
	}
	ep := p.Media.Episodes[0]
	if ep.AudioFile == nil || ep.AudioFile.Metadata == nil || ep.AudioFile.Metadata.Path != epPath {
		t.Fatalf("episode does not point at its file: %+v", ep.AudioFile)
	}

	if err := be.DeletePodcastEpisode("show1", ep.ID); err != nil {
		t.Fatalf("DeletePodcastEpisode: %v", err)
	}
	if _, err := os.Stat(epPath); !os.IsNotExist(err) {
		t.Fatalf("audio file still present after delete: %v", err)
	}
	after, err := be.GetPodcast("show1")
	if err != nil || after == nil {
		t.Fatalf("GetPodcast after delete: %v", err)
	}
	if len(after.Media.Episodes) != 0 {
		t.Fatalf("episode still listed after delete: %+v", after.Media.Episodes)
	}
}

func TestDeletePodcastEpisodeRejectsUnknownEpisodeAndPodcast(t *testing.T) {
	t.Parallel()
	be, epPath := newEpisodeDeleteBackend(t)
	if err := be.DeletePodcastEpisode("show1", "no-such-episode"); err == nil {
		t.Fatal("an unknown episode must be an error")
	}
	if err := be.DeletePodcastEpisode("no-such-show", "x"); err == nil {
		t.Fatal("an unknown podcast must be an error")
	}
	if _, err := os.Stat(epPath); err != nil {
		t.Fatalf("a rejected delete must leave the audio alone: %v", err)
	}
}
