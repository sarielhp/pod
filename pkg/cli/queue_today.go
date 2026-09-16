package cli

import (
	"fmt"
	"path/filepath"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"
)

func buildQueueTodaySubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "today",
		Description: "Queue downloaded, uncleaned episodes whose source publication date is today (local calendar date; unknown dates skipped)",
		UsageLine:   "pod queue today [options]",
		Args:        clihelp.MaximumNArgs(0),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show eligible episodes without changing the queue"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "today"
			opts.Args = ctx.Args
			return nil
		},
	}
}

// cachedPublicationTimes reads the publication time of every cached episode,
// keyed by its absolute path.
func cachedPublicationTimes(dir string) map[string]time.Time {
	out := make(map[string]time.Time)
	index, _ := podcast.LoadPodcastCache(dir)
	if index == nil {
		return out
	}
	for _, ep := range index.Episodes {
		if ep.PublishedAt <= 0 {
			continue
		}
		out[filepath.Clean(cachedEpisodePath(dir, ep))] = time.UnixMilli(ep.PublishedAt)
	}
	return out
}

// cachedEpisodePath resolves a cached episode's location, which older caches
// recorded as a bare filename and newer ones as a path that may be relative.
func cachedEpisodePath(dir string, ep podcast.CachedEpisodeSummary) string {
	if ep.Path == "" {
		return filepath.Join(dir, ep.Filename)
	}
	if !filepath.IsAbs(ep.Path) {
		return filepath.Join(dir, ep.Path)
	}
	return ep.Path
}

func todayQueueCandidates(dir string, now time.Time, source map[string]time.Time) []string {
	cached := cachedPublicationTimes(dir)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)
	var candidates []string
	for _, path := range util.FindMP3Files(dir) {
		if !pipeline.IsQueueAudioPath(path) {
			continue
		}
		published := cached[filepath.Clean(path)]
		if source != nil {
			absolute, err := filepath.Abs(path)
			if err != nil {
				continue
			}
			published = source[absolute]
		} else if date, ok := podcast.SourcePublicationTime(path); ok {
			published = date
		}
		if !published.IsZero() && !published.Before(start) && published.Before(end) && !pipeline.IsEpisodeClean(path) {
			candidates = append(candidates, podcast.QueueFilename(dir, path))
		}
	}
	return candidates
}

func handleQueueToday(root string, cli CLIOptions, now time.Time) error {
	return handleQueueTodaySource(root, cli, now, nil)
}

func handleQueueTodaySource(root string, cli CLIOptions, now time.Time, source map[string]time.Time) error {
	total := 0
	for _, pod := range scanQueuePodcasts(root) {
		candidates := todayQueueCandidates(pod.Dir, now, source)
		if cli.DryRun {
			for _, filename := range candidates {
				fmt.Fprintf(outFor(cli), "[dry-run] Would queue for AdR: %s\n", filepath.Join(pod.Dir, filename))
			}
			continue
		}
		if len(candidates) == 0 {
			continue
		}
		err := pipeline.UpdateQueue(pod.Dir, func(entries []string) []string {
			existing := make(map[string]bool)
			for _, entry := range entries {
				existing[strings.ToLower(entry)] = true
			}
			for _, filename := range candidates {
				key := strings.ToLower(filename)
				if !existing[key] {
					entries = append(entries, filename)
					existing[key] = true
					total++
				}
			}
			return entries
		})
		if err != nil {
			return fmt.Errorf("queue today's episodes for %s: %w", pod.Title, err)
		}
	}
	if !cli.Quiet && !cli.DryRun {
		fmt.Fprintf(outFor(cli), "Added %d uncleaned episode(s) published today (%s, local time) to the AdR queue.\n", total, now.Format("2006-01-02"))
	}
	return nil
}
