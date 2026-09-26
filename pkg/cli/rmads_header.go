package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/types"
	"pod/pkg/util"
)

// PrintEpisodeHeader leads with the podcast and the episode, each on its own
// line, so the two names are readable before any processing detail.
func PrintEpisodeHeader(inputFile string, idx, totalFiles, processedCount int, opts types.ProcOptions, out io.Writer) {
	resolved := inputFile
	if abs, err := filepath.Abs(inputFile); err == nil {
		resolved = abs
	}
	podcastDir := episode.DetectPodcastDirForAudio(resolved)
	podcastName := filepath.Base(podcastDir)
	episodeName := podcast.EpisodeTitleFromPath(resolved)

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n", util.BoldCyan("Podcast:"), util.Bold(util.DisplayName(podcastName)))
	fmt.Fprintf(out, "%s %s\n", util.BoldCyan("Episode:"), util.Bold(util.DisplayName(episodeName)))
	switch {
	case opts.Count > 0:
		fmt.Fprintf(out, "Processing (%d/%d limit): %s\n", processedCount, opts.Count, podcastDir)
	case totalFiles > 1:
		fmt.Fprintf(out, "Processing (%d/%d): %s\n", idx+1, totalFiles, podcastDir)
	default:
		fmt.Fprintf(out, "Processing: %s\n", podcastDir)
	}
}
