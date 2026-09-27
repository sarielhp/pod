package podcast

import (
	"os"
	"path/filepath"
	"pod/pkg/config"
	"pod/pkg/episode"
)

func EpisodePriority(podDir, audioPath string) int {
	if podDir == "" && audioPath != "" {
		podDir = priorityPodcastDir(audioPath)
	}
	cfg := config.LoadPodcastConfig(podDir, config.DefaultPodcastConfig(nil))
	priority := max(0, min(10, cfg.Priority))
	if audioPath != "" {
		if st, err := episode.LoadEpisodeStatus(episode.StatusPathFor(audioPath)); err == nil && st != nil {
			priority = max(priority, max(0, min(10, st.Priority)))
		}
	}
	return priority
}

func priorityPodcastDir(path string) string {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return DetectPodcastDirForAudio(path)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, config.PodcastConfigFileName)); err == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return DetectPodcastDirForAudio(path)
		}
		dir = parent
	}
}
