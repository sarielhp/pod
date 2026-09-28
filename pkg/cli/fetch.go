package cli

import (
	"fmt"
	"time"

	"github.com/sarielhp/clihelp"

	"pod/pkg/config"
	"pod/pkg/podcast"
)

func buildFetchCommand(opts *CLIOptions, action *string, countVal *int) clihelp.Command {
	return clihelp.Command{
		Name:        "fetch",
		Description: "Fetch latest podcast episodes and remove ad segments",
		UsageLine:   "pod fetch [podcast-id] [options]",
		Parameters: []clihelp.Param{
			{Name: "[podcast-id]", Description: "Specify target podcast by name, index, or ID"},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <name>", "", "Target podcast by ID, index, or name"),
			clihelp.Int(countVal, "-n, --count <number>", 1, "Latest N episodes per show to inspect"),
			clihelp.Bool(&opts.DryRun, "-d, --dry-run", false, "Preview actions without downloading or cleaning"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
			clihelp.Bool(&opts.IncludeHourly, "--hourly", false, "Include hourly news bulletins, skipped by default"),
			clihelp.Bool(&opts.NoClean, "--no-clean", false, "Skip ad-removal processing after downloading"),
			clihelp.Bool(&opts.NoClean, "--download-only", false, "Skip ad-removal processing after downloading"),
			clihelp.String(&opts.UseLLM, "--profile <id/name>", "", "Select LLM profile ID or name"),
			clihelp.String(&opts.Force, "-f, --force <stage>", "", "Force: 'whisper', 'llm', or 'all'"),
			clihelp.Int(&opts.FeedJobs, "-j, --jobs <number>", 0, "Feeds to fetch concurrently (default 24)"),
			clihelp.String(&opts.PodcastsDir, "--podcasts-dir <dir>", "", "Override podcasts directory"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod fetch",
				Description: "Fetch and clean the latest episode for each active subscription",
			},
			{
				Line:        "pod fetch TheDaily",
				Description: "Fetch and clean the latest episode of a specific show",
			},
			{
				Line:        "pod fetch -n 3",
				Description: "Fetch and clean up to the 3 newest episodes per show",
			},
			{
				Line:        "pod fetch --no-clean",
				Description: "Download and queue newest episodes without cleaning ads",
			},
			{
				Line:        "pod fetch --dry-run",
				Description: "Preview which new episodes would be downloaded",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "fetch"
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 && opts.Podcast == "" {
				opts.Podcast = ctx.Args[0]
			}
			if *countVal <= 0 {
				return fmt.Errorf("download count must be positive")
			}
			opts.Count = *countVal
			opts.CountGiven = true
			return nil
		},
	}
}

func runFetchCommand(cfg Config, cli CLIOptions) error {
	store, storeErr := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if storeErr != nil {
		return fmt.Errorf("load subscriptions: %w", storeErr)
	}
	subs := store.List()
	if len(subs) == 0 {
		fmt.Fprintf(progressFor(cli), "No subscriptions found in %s\n", store.FilePath())
		return nil
	}

	lib := library(cfg, cli, nil)
	target := cli.Podcast
	if target == "" && len(cli.Args) > 0 {
		target = cli.Args[0]
	}
	count := cli.Count
	if count <= 0 {
		count = 1
	}
	opts := podcast.FetchOptions{
		Target:        target,
		Count:         count,
		IncludeHourly: cli.IncludeHourly,
		Jobs:          cli.FeedJobs,
	}

	start := time.Now()
	plans := lib.PlanFetch(subs, opts, feedCheckProgress(cli))
	fmt.Fprint(progressFor(cli), "\r\x1b[K")
	reportSubDownloadPlans(plans, time.Since(start), cli)

	if cli.DryRun {
		printDryRunPlans(plans, cli)
		return nil
	}

	return executeFetchDownloads(lib, store, cfg, cli, plans, opts)
}

func executeFetchDownloads(lib *podcast.Library, store *podcast.SubscriptionStore, cfg Config, cli CLIOptions, plans []podcast.SubscriptionPlan, opts podcast.FetchOptions) error {
	toDownloadCount := 0
	for _, p := range plans {
		toDownloadCount += len(p.ToDownload)
	}
	if toDownloadCount == 0 {
		return nil
	}

	dlOpts := podcast.SubscriptionDownloadOptions{
		Target:      opts.Target,
		Jobs:        opts.Jobs,
		AlwaysQueue: true,
		Defaults: config.PolicyDefaults{
			DownloadPolicy: cfg.DefaultDownloadPolicy,
			DownloadK:      cfg.DefaultDownloadK,
			AdRemoval:      cfg.DefaultAdRemoval,
		},
	}

	res := lib.ExecuteSubscriptionDownloads(plans, store, dlOpts)
	for _, err := range res.Failures {
		fmt.Fprintf(errFor(cli), "Warning: failed downloading %v\n", err)
	}
	if !cli.Quiet && res.Downloaded > 0 {
		fmt.Fprintf(outFor(cli), "Downloaded %d episode(s) across %d podcast(s).\n", res.Downloaded, res.Podcasts)
	}
	dlErr := downloadFailuresError(res)

	if cli.NoClean || res.Downloaded == 0 {
		return dlErr
	}

	if !cli.Quiet {
		fmt.Fprintln(outFor(cli), "\nStarting ad removal on downloaded episode(s)...")
	}
	if err := handleQueueRun(lib, cfg, cli, opts.Target); err != nil {
		return err
	}
	return dlErr
}
