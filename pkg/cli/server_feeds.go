package cli

import (
	"fmt"
	"io"
	"time"

	"pod/pkg/backend"
	"pod/pkg/podcast"
	"pod/pkg/util"

	"github.com/sarielhp/clihelp"
)

const (
	// feedCheckTimeout bounds a single feed fetch. A sweep waits on dozens of
	// unrelated origins, so one unresponsive publisher must not be able to
	// stall the run.
	feedCheckTimeout = 15 * time.Second
	// feedCheckAttempts is deliberately small: a feed that fails twice is
	// reported as unreadable and handed to the server rather than retried at
	// length here.
	feedCheckAttempts = 2
)

type feedCheckSummary struct {
	Results      []podcast.FeedCheckResult
	Unchanged    int
	Changed      int
	Unreadable   int
	Refreshed    int
	NewEpisodes  int
	Undownloaded int
	Elapsed      time.Duration
}

// resolveFeedTargets resolves the podcasts to check. Unlike the download and
// pruning paths it needs only metadata, so it takes the backend's cheap listing
// when one exists rather than transferring every episode of every podcast.
func resolveFeedTargets(b backend.Backend, cli CLIOptions) ([]backend.Podcast, error) {
	podcasts, err := backend.ListPodcastsFrom(b)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch podcasts from server: %w", err)
	}
	return filterServerTargets(podcasts, cli)
}

// checkServerFeeds fetches every target feed directly and conditionally, then
// asks the server to update only those that turned out to need it.
func checkServerFeeds(b backend.Backend, podcasts []backend.Podcast, cli CLIOptions) *feedCheckSummary {
	start := time.Now()
	index := podcast.BuildEpisodeIndex(b, podcasts)
	results := podcast.CheckFeedsForUpdates(podcasts, index, podcast.FeedCheckOptions{
		Concurrency: cli.FeedJobs,
		Force:       cli.Refresh,
		Timeout:     feedCheckTimeout,
		MaxAttempts: feedCheckAttempts,
	})

	summary := &feedCheckSummary{Results: results}
	for i := range results {
		r := &results[i]
		switch r.Status {
		case podcast.FeedUnchanged:
			summary.Unchanged++
		case podcast.FeedChanged:
			summary.Changed++
		default:
			summary.Unreadable++
		}
		summary.NewEpisodes += len(r.New)
		summary.Undownloaded += r.Undownloaded
	}

	summary.Refreshed = 0
	summary.Elapsed = time.Since(start)
	return summary
}

func reportFeedCheck(summary *feedCheckSummary, cli CLIOptions) {
	if cli.Quiet {
		return
	}
	for i := range summary.Results {
		r := &summary.Results[i]
		if cli.Verbose || r.Status != podcast.FeedUnchanged || len(r.New) > 0 {
			printFeedCheckLine(outFor(cli), r, cli.Verbose)
		}
	}

	total := len(summary.Results)
	fmt.Fprintf(outFor(cli), "\nChecked %d feed(s) in %.1fs: %d unchanged, %d changed, %d unreadable.\n",
		total, summary.Elapsed.Seconds(), summary.Unchanged, summary.Changed, summary.Unreadable)
	fmt.Fprintf(outFor(cli), "%d new episode(s) not yet in the server catalog (%d undownloaded episode(s) available).\n",
		summary.NewEpisodes, summary.Undownloaded)
	if summary.Refreshed > 0 {
		fmt.Fprintf(outFor(cli), "Woke the server for %d podcast(s); the rest needed no server work.\n", summary.Refreshed)
	} else {
		fmt.Fprintln(outFor(cli), "No server work was needed.")
	}
}

func printFeedCheckLine(w io.Writer, r *podcast.FeedCheckResult, verbose bool) {
	title := util.DisplayName(r.Title)
	switch {
	case r.Status == podcast.FeedUnknown:
		fmt.Fprintf(w, "! %s: could not read feed: %v\n", title, r.Err)
	case len(r.New) > 0:
		fmt.Fprintf(w, "+ %s: %d new episode(s) (%d in feed, %d undownloaded)\n",
			title, len(r.New), r.EpisodeCount, r.Undownloaded)
		if verbose {
			for _, ep := range r.New {
				fmt.Fprintf(w, "    + %s (%s)\n", util.DisplayName(ep.Title), ep.PubDate)
			}
		}
	default:
		fmt.Fprintf(w, "  %s: unchanged (%s, %d episodes, %d undownloaded)\n",
			title, r.Reason, r.EpisodeCount, r.Undownloaded)
	}
}

func buildServerFeedsSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "feeds",
		Description: "Check podcast feeds directly for newly published episodes",
		UsageLine:   "pod server feeds [options] [podcast-id]",
		Parameters: []clihelp.Param{
			{Name: "[podcast-id]", Description: "Optional podcast identifier"},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Specify podcast by name, index, or ID"),
			clihelp.Bool(&opts.Refresh, "-F, --force", false, "Ignore cached feed validators and re-read every feed"),
			clihelp.Int(&opts.FeedJobs, "-j, --jobs <number>", 0, "Feeds to fetch concurrently (default 16)"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug output"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod server feeds",
				Description: "Check all feeds for new episodes, waking the server only for those that changed",
			},
			{
				Line:        "pod server feeds -v",
				Description: "Scan all podcast feeds and report the verdict for each one",
			},
			{
				Line:        "pod server feeds -p 'Huberman Lab'",
				Description: "Check the feed for a specific podcast",
			},
			{
				Line:        "pod server feeds --force -j 24",
				Description: "Re-read every feed, ignoring cached validators, 24 at a time",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "feeds"
			opts.SyncSubcmd = "feeds"
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 && opts.Podcast == "" {
				opts.Podcast = ctx.Args[0]
			}
			return nil
		},
	}
}

func handleServerFeeds(config Config, cli CLIOptions) error {
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err != nil {
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	podcasts, err := resolveFeedTargets(b, cli)
	if err != nil {
		return err
	}
	fmt.Fprintf(progressFor(cli), "Checking %d podcast feed(s) directly for new episodes...\n", len(podcasts))
	reportFeedCheck(checkServerFeeds(b, podcasts, cli), cli)
	return nil
}
