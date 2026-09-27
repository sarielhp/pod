package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/backend"
	"pod/pkg/podcast"
	"pod/pkg/types"
)

func TestFindTargetEpisodeFromBackend_OnlyDownloadsLatest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir := filepath.Join(tmp, "My Show")
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	b := newMockTestBackend()
	b.podcastMap["item-1"] = &backend.Podcast{
		ID: "item-1",
		Media: backend.PodcastMedia{
			Metadata: backend.PodcastMetadata{Title: "My Show", FeedURL: "https://example.com/feed.xml"},
		},
	}
	b.feedEpisodes = []backend.FeedEpisode{
		{Title: "Ep 3", PublishedAt: 1725364800000, GUID: "g-3", EnclosureURL: "https://example.com/ep3.mp3"},
		{Title: "Ep 2", PublishedAt: 1725278400000, GUID: "g-2", EnclosureURL: "https://example.com/ep2.mp3"},
		{Title: "Ep 1", PublishedAt: 1725192000000, GUID: "g-1", EnclosureURL: "https://example.com/ep1.mp3"},
	}
	b.downloadFn = func(_ string, _ []backend.FeedEpisode) error {
		return fmt.Errorf("simulated download error")
	}

	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "My Show",
		ShortID:    "ms",
		FolderName: "My Show",
		UUID:       "item-1",
	}

	cfg := types.Config{
		PodcastsDir: tmp,
	}
	targetPath, handled := findTargetEpisodeFromBackend(b, resolved, cfg, true)

	if targetPath != "" {
		t.Errorf("expected targetPath empty on download failure, got %s", targetPath)
	}
	if !handled {
		t.Errorf("expected handled=true, got false")
	}
	if b.downloadCalls != 1 {
		t.Errorf("expected exactly 1 download attempt (for latest episode Ep 3), got %d attempts", b.downloadCalls)
	}
}

func TestFindLocalPathForFeedEpisode_ShowPrefixAndSubdir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir := filepath.Join(tmp, "The Show")

	subDir := filepath.Join(podDir, "Episode 1")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	ep1Mp3 := filepath.Join(subDir, "podcast.mp3")
	if err := os.WriteFile(ep1Mp3, []byte("audio1"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	ep2Mp3 := filepath.Join(podDir, "The Show - Episode 2.mp3")
	if err := os.WriteFile(ep2Mp3, []byte("audio2"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	fe1 := backend.FeedEpisode{Title: "Episode 1"}
	path1, ok1 := findLocalPathForFeedEpisode(podDir, fe1, nil)
	if !ok1 || path1 != ep1Mp3 {
		t.Fatalf("expected to resolve subfolder episode %s, got (%s, %v)", ep1Mp3, path1, ok1)
	}

	fe2 := backend.FeedEpisode{Title: "Episode 2"}
	path2, ok2 := findLocalPathForFeedEpisode(podDir, fe2, nil)
	if !ok2 || path2 != ep2Mp3 {
		t.Fatalf("expected to resolve show-prefixed episode %s, got (%s, %v)", ep2Mp3, path2, ok2)
	}
}

func TestResolveMatchingEpisodeAudioFile_ContainerPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir := filepath.Join(tmp, "The News")
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	mp3Path := filepath.Join(podDir, "ep.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	ep := backend.Episode{
		AudioFile: &backend.PodcastAudioFile{
			Metadata: &backend.AudioFileMetadata{
				Path: "/podcasts/The News/ep.mp3",
			},
		},
	}

	got, ok := resolveMatchingEpisodeAudioFile(podDir, ep)
	if !ok || got != mp3Path {
		t.Fatalf("expected (%s, true), got (%s, %v)", mp3Path, got, ok)
	}
}
