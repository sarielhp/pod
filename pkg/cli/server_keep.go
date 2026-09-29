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
		Description: "Delete older audio by keep policy, or keep only the newest N episodes of each podcast (transcripts are preserved)",
		UsageLine:   "pod server prune [number] [options]",
		Parameters:  []clihelp.Param{{Name: "[number]", Description: "Number of latest episodes to keep per podcast"}},
		Args:        clihelp.RangeArgs(0, 1),
		Examples: []clihelp.Example{
			{Line: "pod server prune 5 --skip-favorites --dry-run", Description: "Preview keeping the newest 5 episodes of every non-favorite podcast"},
			{Line: "pod server prune 3 -p 'Fresh Air'", Description: "Keep the newest 3 episodes of one podcast"},
		},
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Specify podcast"),
			clihelp.Int(keepVal, "-k, --keep <number>", -1, "Keep policy count"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Dry run"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Detailed outputs"),
			clihelp.Bool(&opts.SkipFavorites, "--skip-favorites", false, "With a count, leave favorite podcasts alone"),
			clihelp.Bool(&opts.ForceDelete, "-f, --force", false, "With a count, delete without asking for confirmation"),
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
	if cli.KeepCount != nil && *cli.KeepCount > 0 {
		return pruneLibraryByCount(config, cli, *cli.KeepCount)
	}
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err != nil {
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	podcasts, err := resolveServerTargetPodcasts(b, cli)
	if err != nil {
		return err
	}
	return prunePodcastsByKeepPolicy(config, cli, b, podcasts)
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
	reported := 0
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
		if res.DeletedEpisodes == 0 && !cli.Verbose {
			continue
		}
		reported++
		if cli.DryRun {
			fmt.Fprintf(outFor(cli), "[dry-run] %s: would prune %d episode(s) (%s; %d transcript(s) preserved; policy: %s)\n",
				title, res.DeletedEpisodes, formatDiskSize(res.FreedBytes), res.PreservedTranscripts, res.Policy)
			continue
		}
		fmt.Fprintf(outFor(cli), "✓ %s: pruned %d episode(s) (%s freed; %d transcript(s) preserved; policy: %s)\n",
			title, res.DeletedEpisodes, formatDiskSize(res.FreedBytes), res.PreservedTranscripts, res.Policy)
	}
	if reported == 0 && !cli.Quiet {
		fmt.Fprintf(outFor(cli), "Checked %d podcast(s); nothing to prune under their keep policies.\n", len(podcasts))
	}
	return nil
}
