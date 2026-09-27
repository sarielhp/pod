package cli

import (
	"os"
	"path/filepath"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"strings"
	"testing"
	"time"
)

// createTestPodcastWithEpisodes builds a podcast directory with episodes and
// status files, as several command tests need one.
func createTestPodcastWithEpisodes(t *testing.T, root, podName string, titles []string) (string, []string) {
	podDir := filepath.Join(root, podName)
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultPodcastConfig(nil)
	cfg.ID = podcast.GeneratePodcastShortID(podName)
	if err := config.SavePodcastConfig(podDir, cfg); err != nil {
		t.Fatal(err)
	}

	var paths []string
	for i, title := range titles {
		filename := podcast.SanitizeTitle(title) + ".mp3"
		p := filepath.Join(podDir, filename)
		if err := os.WriteFile(p, []byte("fake mp3 data "+title), 0644); err != nil {
			t.Fatal(err)
		}
		st := episode.GetOrCreateEpisodeStatus(p)
		st.PublicationSource = "source"
		st.PublishedAt = time.Now().Add(-time.Duration(len(titles)-i) * 24 * time.Hour).Format(time.RFC3339)
		if err := episode.SaveEpisodeStatus(episode.StatusPathFor(p), st); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return podDir, paths
}

func markEpisodeClean(t *testing.T, mp3Path string) {
	if err := os.WriteFile(strings.TrimSuffix(mp3Path, filepath.Ext(mp3Path))+".transcript.json", []byte(`{"text":"This episode contains a complete discussion with enough meaningful transcript text."}`), 0644); err != nil {
		t.Fatal(err)
	}
	st := episode.GetOrCreateEpisodeStatus(mp3Path)
	st.Status = StateDone
	st.Original.DurationSec = 60.0
	st.Cleaned.DurationSec = 50.0
	if err := episode.SaveEpisodeStatus(episode.StatusPathFor(mp3Path), st); err != nil {
		t.Fatal(err)
	}
}
