package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"pod/pkg/adremoval"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"
)

func buildServerDownloadSubcommand(opts *CLIOptions, action *string, countVal, keepVal *int) clihelp.Command {
	return clihelp.Command{
		Name:        "download",
		Description: "Download undownloaded episodes for podcasts",
		UsageLine:   "pod server download [podcast-id] [options]",
		Parameters:  []clihelp.Param{{Name: "[podcast-id]", Description: "Specify podcast by name, index, or ID"}},
		Args:        clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Specify podcast by name, index, or ID"),
			clihelp.Int(countVal, "-k, --count <number>", -1, "Number of undownloaded episodes to download"),
			clihelp.Bool(&opts.DownloadAll, "--all", false, "Download all episodes from entire feed catalog"),
			clihelp.Bool(&opts.Fill, "-f, --fill", false, "Fill gaps in downloaded episodes"),
			clihelp.Int(keepVal, "-K, --keep <number>", -1, "Enforce keep count policies"),
			clihelp.BoolToggle(&opts.CheckNew, "--[no-]check-new", true, "Check new episodes published"),
			clihelp.Int(&opts.FeedJobs, "-j, --jobs <number>", 0, "Feeds to fetch concurrently (default 24)"),
			clihelp.Bool(&opts.Oldest, "--oldest", false, "Download oldest first"),
			clihelp.Bool(&opts.NoWait, "--no-wait", false, "Do not wait for download completion"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show output without executing"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed info"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "download"
			opts.SyncSubcmd = "download"
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 && opts.Podcast == "" {
				opts.Podcast = ctx.Args[0]
			}
			if *countVal != -1 {
				if *countVal <= 0 {
					return fmt.Errorf("download count must be positive")
				}
				opts.Count = *countVal
				opts.CountGiven = true
			} else {
				opts.Count = 1
			}
			if *keepVal > 0 {
				opts.KeepCount = keepVal
			}
			return nil
		},
	}
}

func handleServerDownload(cfg Config, cli CLIOptions) error {
	if backend.IsStandalone(&cfg) {
		store, storeErr := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
		if storeErr == nil {
			return runSubscriptionDirectDownloads(store, cfg, cli)
		}
		return fmt.Errorf("load subscriptions: %w", storeErr)
	}

	b, err := backend.FromAppConfig(&cfg, reporter(cli))
	if err != nil {
		store, storeErr := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
		if storeErr == nil && len(store.List()) > 0 {
			return runSubscriptionDirectDownloads(store, cfg, cli)
		}
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	return runServerDownloads(b, cfg, cli)
}

func runServerDownloads(b backend.Backend, config Config, cli CLIOptions) error {
	podcasts, err := resolveDownloadTargets(b, cli)
	if err != nil {
		return err
	}
	plans := planServerDownloads(b, config, cli, podcasts)
	return executeServerDownloads(b, config, cli, podcasts, plans)
}

// resolveDownloadTargets lists the podcasts to consider. It takes the backend's
// cheap metadata listing: what the server already holds is read separately from
// its catalog in one go, so there is no reason to pay a request per podcast for
// episode lists here.
func resolveDownloadTargets(b backend.Backend, cli CLIOptions) ([]backend.Podcast, error) {
	if !cli.Quiet {
		fmt.Fprint(outFor(cli), "Reading podcast list from server...")
		os.Stdout.Sync()
	}
	start := time.Now()
	podcasts, err := backend.ListPodcastsFrom(b)
	if err != nil {
		fmt.Fprintln(progressFor(cli))
		return nil, fmt.Errorf("failed to fetch podcasts from server: %w", err)
	}
	targets, err := filterServerTargets(podcasts, cli)
	if err != nil {
		fmt.Fprintln(progressFor(cli))
		return nil, err
	}
	fmt.Fprintf(progressFor(cli), " %d podcast(s) (%.1fs).\n", len(targets), time.Since(start).Seconds())
	return targets, nil
}

// planServerDownloads asks every target feed what it offers, concurrently, and
// reports progress while it does: the feeds are the slow part of a run, and
// without this the command sits silent for most of its life.
func planServerDownloads(b backend.Backend, config Config, cli CLIOptions, podcasts []backend.Podcast) []podcast.DownloadPlan {
	start := time.Now()
	index := podcast.BuildEpisodeIndex(b, podcasts)

	progress := func(_, _ int) {}
	if !cli.Quiet {
		progress = func(done, total int) {
			fmt.Fprintf(outFor(cli), "\rChecking feeds for new episodes (%d/%d)...\x1b[K", done, total)
			os.Stdout.Sync()
		}
	}
	plans := podcast.PlanDownloads(b, podcasts, index, downloadOptions(config, cli), progress)
	fmt.Fprint(progressFor(cli), "\r\x1b[K")
	reportDownloadPlans(plans, time.Since(start), cli)
	return plans
}

// reportDownloadPlans says what the feed check found before anything is queued,
// so a run that has nothing to do says so once instead of once per podcast.
func reportDownloadPlans(plans []podcast.DownloadPlan, elapsed time.Duration, cli CLIOptions) {
	if cli.Quiet {
		return
	}
	selected, episodes, failed, unknown := 0, 0, 0, 0
	for i := range plans {
		if plans[i].Err != nil {
			failed++
		}
		if len(plans[i].Episodes) > 0 {
			selected++
			episodes += len(plans[i].Episodes)
		}
		unknown += len(plans[i].Unknown)
	}
	fmt.Fprintf(outFor(cli), "Checked %d feed(s) in %.1fs: %d episode(s) to download across %d podcast(s)",
		len(plans), elapsed.Seconds(), episodes, selected)
	if failed > 0 {
		fmt.Fprintf(outFor(cli), ", %d unreadable", failed)
	}
	fmt.Fprintln(outFor(cli), ".")

	for i := range plans {
		pTitle := util.DisplayName(plans[i].Title())
		switch {
		case plans[i].Err != nil:
			fmt.Fprintf(outFor(cli), "  ! %s: %v\n", pTitle, plans[i].Err)
		case len(plans[i].Unknown) > 0:
			fmt.Fprintf(outFor(cli), "  ? %s: %d episode(s) the server has not indexed yet\n",
				pTitle, len(plans[i].Unknown))
		case cli.Verbose && len(plans[i].Episodes) == 0:
			fmt.Fprintf(outFor(cli), "  - %s: up to date\n", pTitle)
		}
	}
	if unknown > 0 {
		fmt.Fprintf(outFor(cli), "%d episode(s) cannot be requested until the server indexes them; run 'pod server feeds'.\n", unknown)
	}
}

func downloadOptions(config Config, cli CLIOptions) podcast.DownloadOptions {
	return podcast.DownloadOptions{
		Count:                 cli.Count,
		Oldest:                cli.Oldest,
		DryRun:                cli.DryRun,
		NoWait:                true,
		Fill:                  cli.Fill,
		CountGiven:            cli.CountGiven,
		CheckNew:              cli.CheckNew,
		DownloadAll:           cli.DownloadAll,
		Keep:                  cli.KeepCount,
		Progress:              reporter(cli),
		Jobs:                  cli.FeedJobs,
		PodcastsDir:           config.PodcastsDir,
		DefaultDownloadPolicy: config.DefaultDownloadPolicy,
		DefaultDownloadK:      config.DefaultDownloadK,
	}
}

func executeServerDownloads(b backend.Backend, config Config, cli CLIOptions, podcasts []backend.Podcast, plans []podcast.DownloadPlan) error {
	opts := downloadOptions(config, cli)
	totalDownloaded, fromPodcasts := 0, 0
	for i := range plans {
		if plans[i].Err != nil || len(plans[i].Episodes) == 0 {
			continue
		}
		count, err := podcast.ExecuteEpisodeDownloads(b, plans[i].Item, plans[i].Episodes, plans[i].Reasons, opts)
		if err != nil {
			return fmt.Errorf("download %s: %w", plans[i].Title(), err)
		}
		if !cli.DryRun {
			totalDownloaded += count
			fromPodcasts++
		}
	}
	return finalizeServerDownloads(b, config, cli, podcasts, totalDownloaded, fromPodcasts)
}

func finalizeServerDownloads(b backend.Backend, config Config, cli CLIOptions, podcasts []backend.Podcast, totalDownloaded, fromPodcasts int) error {
	if !cli.Quiet && !cli.DryRun {
		fmt.Fprintf(outFor(cli), "Queued %d episode download(s) across %d podcast(s).\n", totalDownloaded, fromPodcasts)
	}
	if totalDownloaded > 0 && !cli.NoWait && !cli.DryRun {
		fmt.Fprintf(progressFor(cli), "Waiting for server to complete %d queued download(s)...\n", totalDownloaded)
		if err := b.WaitForActiveDownloads(podcasts, 5*time.Minute); err != nil {
			return fmt.Errorf("waiting for downloads: %w", err)
		}
	}
	if totalDownloaded > 0 && !cli.DryRun && !cli.NoWait {
		if len(config.PostProcessors) > 0 {
			if err := runPostProcessors(outFor(cli), errFor(cli), config.PostProcessors, cli.Quiet); err != nil {
				return err
			}
		} else {
			if targets, ok := resolveTargetAudioArgs(cli, config); ok {
				if _, err := adremoval.ProcessFiles(targets, cli.ProcOptions, config, reporter(cli)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// runPostProcessors runs each configured post-processor in turn.
//
// A failing processor is reported and does not stop the others, but the
// combined failure is returned. It used to be discarded outright, so a
// post-processor that never worked was indistinguishable from one that did:
// the command printed "Running post-processor: ..." and exited 0 either way.
func runPostProcessors(out, errOut io.Writer, processors []string, quiet bool) error {
	if !quiet {
		fmt.Fprintf(out, "\n=== Executing %d Post-Processor(s) ===\n", len(processors))
	}
	var failures []error
	for _, proc := range processors {
		parts := strings.Fields(proc)
		if len(parts) == 0 {
			continue
		}
		if !quiet {
			fmt.Fprintf(out, "Running post-processor: %s...\n", proc)
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Stdout = out
		cmd.Stderr = errOut
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(errOut, "Post-processor %q failed: %v\n", proc, err)
			failures = append(failures, fmt.Errorf("%s: %w", proc, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d post-processor(s) failed: %w",
			len(failures), len(processors), errors.Join(failures...))
	}
	return nil
}
