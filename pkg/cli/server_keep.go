package cli

import (
	"fmt"
	"path/filepath"
	"strconv"

	"pod/pkg/backend"
	"pod/pkg/podcast"

	"github.com/sarielhp/clihelp"
)

func buildServerPruneSubcommand(opts *CLIOptions, action *string, keepVal *int) clihelp.Command {
	return clihelp.Command{
		Name:        "prune",
		Description: "Delete older MP3 files per podcast keep policy or retention limit (preserves transcripts)",
		UsageLine:   "pod server prune [number] [options]",
		Parameters:  []clihelp.Param{{Name: "[number]", Description: "Number of latest episodes to keep per podcast"}},
		Args:        clihelp.RangeArgs(0, 1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Specify podcast"),
			clihelp.Int(keepVal, "-k, --keep <number>", -1, "Keep policy count"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Dry run"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Detailed outputs"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "keep"
			opts.SyncSubcmd = "prune"
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 {
				if k, err := strconv.Atoi(ctx.Args[0]); err == nil {
					opts.KeepCount = &k
				} else if opts.Podcast == "" {
					opts.Podcast = ctx.Args[0]
				}
			} else if *keepVal > 0 {
				opts.KeepCount = keepVal
			}
			return nil
		},
	}
}

func handleServerKeep(config Config, cli CLIOptions) error {
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err != nil {
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	podcasts, err := resolveServerTargetPodcasts(b, cli)
	if err != nil {
		return err
	}
	keep := -1
	if cli.KeepCount != nil {
		keep = *cli.KeepCount
	}
	if keep > 0 {
		return prunePodcastsByCount(b, podcasts, keep, cli)
	}
	return prunePodcastsByKeepPolicy(config, cli, b, podcasts)
}

func prunePodcastsByCount(b backend.Backend, podcasts []backend.Podcast, keep int, cli CLIOptions) error {
	for _, item := range podcasts {
		title := item.Media.Metadata.Title
		if title == "" {
			title = "Untitled"
		}
		deleted, err := b.ApplyKeepPolicy(item.ID, title, keep, cli.DryRun)
		if err != nil && !cli.Quiet {
			fmt.Fprintf(outFor(cli), "! Error applying keep policy to %s: %v\n", title, err)
		} else if !cli.Quiet {
			fmt.Fprintf(outFor(cli), "✓ %s: pruned %d episode(s) (limit: %d)\n", title, deleted, keep)
		}
	}
	return nil
}

func prunePodcastsByKeepPolicy(cfg Config, cli CLIOptions, b backend.Backend, podcasts []backend.Podcast) error {
	lib := library(cfg, cli, b)
	if len(podcasts) == 0 && cfg.PodcastsDir != "" {
		target := serverTargetName(cli)
		if target == "" || target == "all" {
			for _, e := range podcast.ScanPodcastDirs(cfg.PodcastsDir) {
				podcasts = append(podcasts, backend.Podcast{
					ID:    e.ShortID,
					Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: e.Title}},
					Path:  e.Dir,
				})
			}
		} else if group, err := podcast.ResolvePodcastGroup(cfg.PodcastsDir, target); err == nil {
			for _, e := range group.Entries {
				podcasts = append(podcasts, backend.Podcast{
					ID:    e.ShortID,
					Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: e.Title}},
					Path:  e.Dir,
				})
			}
		}
	}
	for _, item := range podcasts {
		title := item.Media.Metadata.Title
		if title == "" {
			title = "Untitled"
		}
		dir := item.Path
		if dir == "" && cfg.PodcastsDir != "" {
			dir = filepath.Join(cfg.PodcastsDir, item.RelPath)
		}
		if dir == "" {
			continue
		}
		res, err := lib.PrunePodcastKeepPolicy(dir, title, cli.DryRun)
		if err != nil && !cli.Quiet {
			fmt.Fprintf(outFor(cli), "! Error pruning %s: %v\n", title, err)
			continue
		}
		if cli.Quiet {
			continue
		}
		if cli.DryRun {
			if res.DeletedEpisodes > 0 || cli.Verbose {
				fmt.Fprintf(outFor(cli), "[dry-run] %s: would prune %d episode(s) (%s; %d transcript(s) preserved; policy: %s)\n",
					title, res.DeletedEpisodes, formatDiskSize(res.FreedBytes), res.PreservedTranscripts, res.Policy)
			}
		} else {
			if res.DeletedEpisodes > 0 || cli.Verbose {
				fmt.Fprintf(outFor(cli), "✓ %s: pruned %d episode(s) (%s freed; %d transcript(s) preserved; policy: %s)\n",
					title, res.DeletedEpisodes, formatDiskSize(res.FreedBytes), res.PreservedTranscripts, res.Policy)
			}
		}
	}
	return nil
}
