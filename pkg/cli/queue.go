package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"pod/pkg/adremoval"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"
)

type queueEpisodeItem = podcast.QueueItem

func resolveQueueSubcmdArgs(subcmd string, args []string) (string, []string) {
	if subcmd != "" {
		return subcmd, args
	}
	if len(args) == 0 {
		return "list", args
	}
	switch strings.ToLower(args[0]) {
	case "priority", "list", "ls", "add", "today", "latest", "remove", "clear", "run":
		return strings.ToLower(args[0]), args[1:]
	default:
		return "list", args
	}
}

func runQueueCommand(cfg Config, cli CLIOptions) error {
	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "."
	}

	lib := library(cfg, cli, nil)
	subcmd, args := resolveQueueSubcmdArgs(cli.QueueSubcmd, cli.Args)

	switch subcmd {
	case "priority":
		return handleQueuePriority(outFor(cli), podcastsDir, args)
	case "list", "ls":
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return handleQueueList(lib, target, cli)
	case "add":
		if len(args) == 0 {
			args = []string{"all"}
		}
		if len(args) > 0 && strings.EqualFold(args[0], "latest") {
			limit, target, err := parseQueueLatestArgs(args[1:], cli.Count)
			if err != nil {
				return err
			}
			return runQueueLatest(cfg, podcastsDir, limit, target, cli)
		}
		return handleQueueAdd(outFor(cli), lib, args)
	case "today":
		if len(args) != 0 {
			return fmt.Errorf("queue today accepts no arguments")
		}
		return runQueueToday(cfg, podcastsDir, cli, time.Now())
	case "latest":
		limit, target, err := parseQueueLatestArgs(args, cli.Count)
		if err != nil {
			return err
		}
		return runQueueLatest(cfg, podcastsDir, limit, target, cli)
	case "remove":
		if len(args) == 0 {
			return fmt.Errorf("missing target ID(s) to remove from queue")
		}
		return handleQueueRemove(outFor(cli), lib, args)
	case "clear":
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return handleQueueClear(outFor(cli), lib, target)
	case "run":
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return handleQueueRun(lib, cfg, cli, target)
	default:
		return fmt.Errorf("unknown queue action %q (use list, add, today, latest, remove, clear, or run)", subcmd)
	}
}

func handleQueueList(lib *podcast.Library, target string, cli CLIOptions) error {
	var entries []podcast.PodcastDirEntry
	if target != "" {
		res, err := lib.ResolveQueueTarget(target)
		if err != nil {
			return err
		}
		if res.IsPodcast() {
			entries = []podcast.PodcastDirEntry{{
				Dir:        res.Podcast.Dir,
				FolderName: res.Podcast.FolderName,
				Title:      res.Podcast.Title,
				ShortID:    res.Podcast.ShortID,
			}}
		} else if res.IsEpisode() {
			entries = []podcast.PodcastDirEntry{{
				Dir:        res.Episode.PodcastDir,
				FolderName: filepath.Base(res.Episode.PodcastDir),
				Title:      res.Episode.PodcastTitle,
				ShortID:    res.Episode.PodcastShortID,
			}}
		}
	} else {
		entries = lib.QueuePodcasts()
	}

	var allItems []queueEpisodeItem
	for _, p := range entries {
		items, err := collectQueueDisplayItems(p)
		if err != nil {
			return err
		}
		allItems = append(allItems, items...)
	}

	podcast.SortQueueItems(allItems)
	if cli.JSON {
		data, err := json.MarshalIndent(allItems, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}

	if cli.Quiet {
		for _, it := range allItems {
			fmt.Fprintln(outFor(cli), strings.Join(queueDisplayCells(it), " | "))
		}
		return nil
	}

	printQueueTable(outFor(cli), allItems)
	return nil
}

func printQueueTable(w io.Writer, items []queueEpisodeItem) {
	if len(items) == 0 {
		fmt.Fprintln(w, "AdR queue is currently empty.")
		return
	}

	fmt.Fprintf(w, "\nAdR Queue (%d queued):\n", len(items))
	cols := queueTableColumns(48)

	fmt.Fprintln(w, renderTableTop(cols))
	fmt.Fprintln(w, renderTableHeader(cols))
	fmt.Fprintln(w, renderTableDivider(cols))

	for _, it := range items {
		cells := queueDisplayCells(it)
		cells[1] = util.BoldCyan(cells[1])
		fmt.Fprintln(w, renderTableRow(cells, cols))
	}

	fmt.Fprintln(w, renderTableBottom(cols))
	fmt.Fprintln(w)
}

func handleQueueAdd(w io.Writer, lib *podcast.Library, targets []string) error {
	for _, query := range targets {
		if strings.EqualFold(query, "all") || query == "*" || query == "--all" {
			entries := lib.QueuePodcasts()
			totalAdded := 0
			for _, p := range entries {
				count, err := podcast.EnqueuePodcast(p.Dir)
				if err != nil {
					return err
				}
				if count > 0 {
					fmt.Fprintf(w, "Added %d uncleaned episode(s) of %s [%s] to queue\n",
						count, util.Bold(util.DisplayName(p.Title)), util.BoldCyan(p.ShortID))
					totalAdded += count
				}
			}
			fmt.Fprintf(w, "Added a total of %d uncleaned episode(s) across %d podcast(s) to queue.\n", totalAdded, len(entries))
			continue
		}

		res, err := lib.ResolveQueueTarget(query)
		if err != nil {
			return fmt.Errorf("failed to resolve %q: %w", query, err)
		}

		if res.IsEpisode() {
			ep := res.Episode
			added, err := pipeline.AddToQueueChecked(ep.PodcastDir, podcast.QueueFilename(ep.PodcastDir, ep.Path))
			if err != nil {
				return err
			}
			if added {
				fmt.Fprintf(w, "Added to queue: [%s] %s\n", util.BoldCyan(ep.ShortID), util.DisplayName(ep.Title))
			} else {
				fmt.Fprintf(w, "Already in queue: [%s] %s\n", util.BoldCyan(ep.ShortID), util.DisplayName(ep.Title))
			}
		} else if res.IsPodcast() {
			pod := res.Podcast
			count, err := podcast.EnqueuePodcast(pod.Dir)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "Added %d uncleaned episode(s) of %s [%s] to queue\n",
				count, util.Bold(util.DisplayName(pod.Title)), util.BoldCyan(pod.ShortID))
		}
	}
	return nil
}

func handleQueueRemove(w io.Writer, lib *podcast.Library, targets []string) error {
	for _, query := range targets {
		res, err := lib.ResolveQueueTarget(query)
		if err != nil {
			return fmt.Errorf("failed to resolve %q: %w", query, err)
		}

		if res.IsEpisode() {
			ep := res.Episode
			removed, err := pipeline.RemoveQueuedAudio(ep.PodcastDir, ep.Path)
			if err != nil {
				return err
			}
			if removed {
				fmt.Fprintf(w, "Removed from queue: [%s] %s\n", util.BoldCyan(ep.ShortID), util.DisplayName(ep.Title))
			} else {
				fmt.Fprintf(w, "Not found in queue: [%s] %s\n", util.BoldCyan(ep.ShortID), util.DisplayName(ep.Title))
			}
		} else if res.IsPodcast() {
			pod := res.Podcast
			if err := podcast.ClearPodcastQueue(pod.Dir); err != nil {
				return err
			}
			fmt.Fprintf(w, "Cleared queue for %s [%s]\n", util.Bold(util.DisplayName(pod.Title)), util.BoldCyan(pod.ShortID))
		}
	}
	return nil
}

func handleQueueClear(w io.Writer, lib *podcast.Library, target string) error {
	if target != "" {
		res, err := lib.ResolveQueueTarget(target)
		if err != nil {
			return err
		}
		if res.IsPodcast() {
			if err := podcast.ClearPodcastQueue(res.Podcast.Dir); err != nil {
				return err
			}
			fmt.Fprintf(w, "Queue cleared for %s [%s]\n", util.Bold(util.DisplayName(res.Podcast.Title)), util.BoldCyan(res.Podcast.ShortID))
			return nil
		}
		if res.IsEpisode() {
			if _, err := pipeline.RemoveQueuedAudio(res.Episode.PodcastDir, res.Episode.Path); err != nil {
				return err
			}
			fmt.Fprintf(w, "Removed [%s] from queue\n", util.BoldCyan(res.Episode.ShortID))
			return nil
		}
	}

	entries := lib.QueuePodcasts()
	clearedCount := 0
	for _, p := range entries {
		qFile := filepath.Join(p.Dir, "queue.json")
		if _, err := os.Stat(qFile); err == nil {
			if err := podcast.ClearPodcastQueue(p.Dir); err != nil {
				return err
			}
			clearedCount++
		}
	}
	fmt.Fprintf(w, "Queue cleared across %d podcast(s).\n", clearedCount)
	return nil
}

func handleQueueRun(lib *podcast.Library, cfg Config, cli CLIOptions, target string) error {
	items, err := lib.QueueItems(target)
	if err != nil {
		return err
	}
	podcast.SortQueueItems(items)
	if len(items) == 0 {
		fmt.Fprintln(progressFor(cli), "AdR queue is currently empty.")
		return nil
	}

	fmt.Fprintf(progressFor(cli), "Found %d episode(s) in AdR queue.\n", len(items))

	if cli.DryRun {
		for _, it := range items {
			fmt.Fprintf(outFor(cli), "[dry-run] Would process ad removal for: [%s] %s (%s)\n", it.EpisodeID, util.DisplayName(it.Title), it.Filename)
		}
		return nil
	}

	cli.Normalize()
	return executeQueueRun(items, cli, cfg)
}

func executeQueueRun(items []queueEpisodeItem, cli CLIOptions, cfg Config) error {
	total := len(items)
	processedCount := 0
	var failedEpisodes []string

	for i := range items {
		podcast.SortQueueItems(items[i:])
		it := items[i]
		fmt.Fprintf(progressFor(cli), "\n[%d/%d] Processing queued episode: %s [%s]\n", i+1, total, util.DisplayName(it.Title), util.BoldCyan(it.EpisodeID))

		if !util.FileExists(it.AudioPath) {
			fmt.Fprintf(progressFor(cli), "Audio file not found on disk: %s (retained in queue)\n", it.Filename)
			failedEpisodes = append(failedEpisodes, it.Filename)
			continue
		}

		if !cli.ForceTranscribe && !cli.ForceLLM && !cli.Recut && pipeline.IsEpisodeClean(it.AudioPath) {
			fmt.Fprintf(progressFor(cli), "Episode already has ads removed: %s (removing from queue)\n", it.Filename)
			if _, err := pipeline.RemoveQueuedAudio(it.PodcastDir, it.AudioPath); err != nil {
				return err
			}
			continue
		}

		err := adremoval.ProcessQueuedTarget(it.PodcastDir, it.AudioPath, "rm_ads", cli.ProcOptions, cfg)
		if err != nil {
			if !cli.Quiet {
				fmt.Fprintf(errFor(cli), "Error processing %s: %v\n", it.Filename, err)
			}
			failedEpisodes = append(failedEpisodes, it.Filename)
			continue
		}
		processedCount++
	}

	if !cli.Quiet {
		fmt.Fprintf(outFor(cli), "\nFinished queue run: %d/%d episode(s) processed successfully.\n", processedCount, total)
		if len(failedEpisodes) > 0 {
			fmt.Fprintf(outFor(cli), "Failed episode(s): %s\n", strings.Join(failedEpisodes, ", "))
		}
	}

	if len(failedEpisodes) > 0 {
		return fmt.Errorf("%d episode(s) failed during queue run", len(failedEpisodes))
	}
	return nil
}

func buildQueueCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "queue",
		Description: "Manage the ad removal (AdR) processing queue",
		UsageLine:   "pod queue [command]",
		Subcommands: []clihelp.Command{
			buildQueueListSubcommand(opts, action),
			buildQueueLsSubcommand(opts, action),
			buildQueueLatestSubcommand(opts, action),
			buildQueueAddSubcommand(opts, action),
			buildQueueTodaySubcommand(opts, action),
			buildQueuePrioritySubcommand(opts, action),
			buildQueueRemoveSubcommand(opts, action),
			buildQueueClearSubcommand(opts, action),
			buildQueueRunSubcommand(opts, action),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod queue list",
				Description: "List all episodes currently in the ad removal queue",
			},
			{
				Line:        "pod queue latest",
				Description: "Queue the 10 latest published uncleaned episodes",
			},
			{
				Line:        "pod queue latest 10",
				Description: "Queue the 10 latest published uncleaned episodes",
			},
			{
				Line:        "pod queue add",
				Description: "Queue all episodes needing ad removal across all podcasts",
			},
			{
				Line:        "pod queue run",
				Description: "Process ad removal on all queued episodes",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			if len(ctx.Args) > 0 {
				switch strings.ToLower(ctx.Args[0]) {
				case "latest":
					opts.QueueSubcmd = "latest"
					opts.Args = ctx.Args[1:]
					return nil
				}
			}
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildQueueListSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "list",
		Description: "List queued episodes across library or podcast",
		UsageLine:   "pod queue list [podcast-id] [options]",
		Args:        clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Print episode details without table borders"),
			clihelp.Bool(&opts.JSON, "--json", false, "Output results in JSON format"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "list"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildQueueLsSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	cmd := buildQueueListSubcommand(opts, action)
	cmd.Name = "ls"
	cmd.UsageLine = "pod queue ls [podcast-id] [options]"
	return cmd
}

func buildQueueAddSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "add",
		Description: "Add uncleaned episodes to the ad removal queue (defaults to all)",
		UsageLine:   "pod queue add [id... | all | latest [N]]",
		Parameters: []clihelp.Param{
			{Name: "[id... | all | latest [N]]", Description: "Episode ID(s), podcast ID(s), 'all', or 'latest [N]' to queue uncleaned episodes"},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show eligible episodes without changing the queue"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod queue add",
				Description: "Queue all episodes needing ad removal across all podcasts",
			},
			{
				Line:        "pod queue add latest 10",
				Description: "Queue the 10 latest published uncleaned episodes",
			},
			{
				Line:        "pod queue add all",
				Description: "Queue all episodes needing ad removal across all podcasts",
			},
			{
				Line:        "pod queue add <podcast-id>",
				Description: "Queue all uncleaned episodes for a specific podcast",
			},
			{
				Line:        "pod queue add <episode-id>",
				Description: "Queue a specific episode for ad removal",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "add"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildQueueRemoveSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "remove",
		Description: "Remove one or more episodes from the queue",
		UsageLine:   "pod queue remove <id...>",
		Args:        clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "remove"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildQueueClearSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "clear",
		Description: "Clear queue for a specific podcast or all podcasts",
		UsageLine:   "pod queue clear [podcast-id]",
		Args:        clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "clear"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildQueueRunSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "run",
		Description: "Process ad removal for queued episodes",
		UsageLine:   "pod queue run [podcast-id] [options]",
		Args:        clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Quiet mode"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Verbose output"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Simulate processing without making changes"),
			clihelp.String(&opts.Force, "-f, --force <type>", "", "Force re-processing (all, whisper, llm)"),
			clihelp.String(&opts.UseLLM, "--use-llm <name|id>", "", "Select specific LLM profile"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod queue run",
				Description: "Process ad removal on all queued episodes",
			},
			{
				Line:        "pod queue run <podcast-id>",
				Description: "Process ad removal for queued episodes of a specific podcast",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "run"
			opts.Args = ctx.Args
			return nil
		},
	}
}
