package cli

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

func buildQueueLatestSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "latest",
		Description: "Queue latest published episodes that do not have their ads removed yet",
		UsageLine:   "pod queue latest [N] [podcast-id] [options]",
		Parameters: []clihelp.Param{
			{Name: "[N]", Description: "Number of episodes to queue (default: 10)"},
			{Name: "[podcast-id]", Description: "Optional podcast ID or query to restrict to"},
		},
		Args: clihelp.MaximumNArgs(2),
		Options: []clihelp.Option{
			clihelp.Int(&opts.Count, "-n, --limit <number>", 10, "Number of latest episodes to queue"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show eligible episodes without changing the queue"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod queue latest",
				Description: "Queue the 10 latest published uncleaned episodes",
			},
			{
				Line:        "pod queue latest 10",
				Description: "Queue the 10 latest published uncleaned episodes",
			},
			{
				Line:        "pod queue latest 5 <podcast-id>",
				Description: "Queue the 5 latest published uncleaned episodes for a specific podcast",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "latest"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func parseQueueLatestArgs(args []string, defaultLimits ...int) (int, string, error) {
	limit := 10
	if len(defaultLimits) > 0 && defaultLimits[0] > 0 {
		limit = defaultLimits[0]
	}
	target := ""
	for _, arg := range args {
		if n, err := strconv.Atoi(arg); err == nil {
			if n <= 0 {
				return 0, "", fmt.Errorf("invalid episode count %q (must be a positive integer)", arg)
			}
			limit = n
		} else {
			if target != "" {
				return 0, "", fmt.Errorf("unexpected argument %q", arg)
			}
			target = arg
		}
	}
	return limit, target, nil
}

type latestQueueCandidate struct {
	path           string
	podDir         string
	podTitle       string
	podShortID     string
	episodeShortID string
	title          string
	pubTime        time.Time
}

func collectLatestUncleanedEpisodes(podcastsDir, target string) ([]latestQueueCandidate, error) {
	var entries []podcast.PodcastDirEntry
	if target != "" {
		res, err := resolveQueueTarget(podcastsDir, target)
		if err != nil {
			return nil, err
		}
		if res.IsPodcast() {
			entries = []podcast.PodcastDirEntry{{
				Dir:        res.Podcast.Dir,
				FolderName: res.Podcast.FolderName,
				Title:      res.Podcast.Title,
				ShortID:    res.Podcast.ShortID,
			}}
		} else {
			return nil, fmt.Errorf("target %q is an episode, not a podcast", target)
		}
	} else {
		entries = scanQueuePodcasts(podcastsDir)
	}

	var candidates []latestQueueCandidate
	for _, p := range entries {
		for _, path := range util.FindMP3Files(p.Dir) {
			if !episode.IsQueueAudioPath(path) || episode.IsEpisodeClean(path) {
				continue
			}
			st := episode.GetOrCreateEpisodeStatus(path)
			var fi os.FileInfo
			if stat, err := os.Stat(path); err == nil {
				fi = stat
			}
			pubTime := resolveEpisodePublicationTime(path, st, fi)
			epShortID := podcast.EpisodeShortIDReadOnly(p.Dir, p.ShortID, path)
			title := podcast.EpisodeTitleFromPath(path)
			candidates = append(candidates, latestQueueCandidate{
				path:           path,
				podDir:         p.Dir,
				podTitle:       p.Title,
				podShortID:     p.ShortID,
				episodeShortID: epShortID,
				title:          title,
				pubTime:        pubTime,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].pubTime.Equal(candidates[j].pubTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].pubTime.After(candidates[j].pubTime)
	})

	return candidates, nil
}

func runQueueLatest(cfg Config, podcastsDir string, limit int, target string, cli CLIOptions) error {
	candidates, err := collectLatestUncleanedEpisodes(podcastsDir, target)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		fmt.Fprintln(progressFor(cli), "No uncleaned episodes found to queue.")
		return nil
	}

	if limit > len(candidates) {
		limit = len(candidates)
	}
	selected := candidates[:limit]

	if cli.DryRun {
		for _, it := range selected {
			qFile := podcast.QueueFilename(it.podDir, it.path)
			qEntries, _ := episode.ReadQueue(it.podDir)
			isQueued := false
			for _, q := range qEntries {
				if strings.EqualFold(q, qFile) {
					isQueued = true
					break
				}
			}
			if isQueued {
				fmt.Fprintf(outFor(cli), "[dry-run] Already in queue: [%s] %s (%s)\n",
					it.episodeShortID, util.DisplayName(it.title), util.DisplayName(it.podTitle))
			} else {
				fmt.Fprintf(outFor(cli), "[dry-run] Would queue for AdR: [%s] %s (%s)\n",
					it.episodeShortID, util.DisplayName(it.title), util.DisplayName(it.podTitle))
			}
		}
		return nil
	}

	addedCount, alreadyCount := 0, 0
	for _, it := range selected {
		qFile := podcast.QueueFilename(it.podDir, it.path)
		added, err := episode.AddToQueueChecked(it.podDir, qFile)
		if err != nil {
			return fmt.Errorf("queue episode %s: %w", it.title, err)
		}
		if added {
			addedCount++
			if !cli.Quiet {
				fmt.Fprintf(outFor(cli), "Added to queue: [%s] %s (%s)\n",
					util.BoldCyan(it.episodeShortID), util.DisplayName(it.title), util.Bold(util.DisplayName(it.podTitle)))
			}
		} else {
			alreadyCount++
			if !cli.Quiet {
				fmt.Fprintf(outFor(cli), "Already in queue: [%s] %s (%s)\n",
					util.BoldCyan(it.episodeShortID), util.DisplayName(it.title), util.Bold(util.DisplayName(it.podTitle)))
			}
		}
	}

	if !cli.Quiet {
		if alreadyCount > 0 {
			fmt.Fprintf(outFor(cli), "Added %d uncleaned episode(s) to the AdR queue (%d already queued).\n", addedCount, alreadyCount)
		} else {
			fmt.Fprintf(outFor(cli), "Added %d uncleaned episode(s) to the AdR queue.\n", addedCount)
		}
	}
	return nil
}
