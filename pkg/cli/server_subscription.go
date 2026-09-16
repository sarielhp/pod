package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

func buildServerAddSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "add",
		Description: "Add a podcast subscription by RSS feed URL",
		UsageLine:   "pod server add <feed-url> [title]",
		Parameters: []clihelp.Param{
			{Name: "<feed-url>", Description: "Upstream podcast RSS feed URL"},
			{Name: "[title]", Description: "Optional title for podcast"},
		},
		Args: clihelp.RangeArgs(1, 2),
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "add"
			opts.SyncSubcmd = "add"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildServerRemoveSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "remove",
		Description: "Remove a podcast subscription by ID or title",
		UsageLine:   "pod server remove <id-or-title>",
		Parameters: []clihelp.Param{
			{Name: "<id-or-title>", Description: "Podcast ID, folder, or title"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "remove"
			opts.SyncSubcmd = "remove"
			opts.Args = ctx.Args
			return nil
		},
	}
}

// buildRSSGenCommand generates the static site.
//
// It is a top-level command rather than a subcommand of `server`, because
// publishing the local site is a different kind of act from talking to
// upstream feeds — and a `feed` sitting beside `feeds` invited running one
// while meaning the other.
func buildRSSGenCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "rss_gen",
		Description: "Generate the RSS feed and web pages for local podcasts",
		UsageLine:   "pod rss_gen [id-or-title]",
		Parameters: []clihelp.Param{
			{Name: "[id-or-title]", Description: "Optional podcast ID or title to regenerate"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod rss_gen", Description: "Regenerate every show's feed.xml and index.html, and the catalog"},
			{Line: "pod rss_gen p0001", Description: "Regenerate one show"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "rss_gen"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildServerImportSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "import",
		Description: "Import subscriptions from OPML file or backend into local store",
		UsageLine:   "pod server import [file]",
		Parameters: []clihelp.Param{
			{Name: "[file]", Description: "Optional OPML file to import (defaults to importing from backend)"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "import"
			opts.SyncSubcmd = "import"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func handleServerAdd(cfg Config, cli CLIOptions) error {
	if len(cli.Args) == 0 {
		return fmt.Errorf("feed URL is required")
	}
	feedURL := strings.TrimSpace(cli.Args[0])
	title := ""
	if len(cli.Args) > 1 {
		title = strings.TrimSpace(cli.Args[1])
	}

	var eps []backend.FeedEpisode
	if title == "" {
		fmt.Fprintf(progressFor(cli), "Inspecting feed: %s\n", feedURL)
		fetchedEps, _, _, _, err := podcast.FetchFeedDirect(feedURL, "", "")
		if err == nil && len(fetchedEps) > 0 {
			eps = fetchedEps
		}
	}

	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}

	imgURL := ""
	if entry := library(cfg, cli, nil).FeedCache().Get(feedURL); entry != nil {
		imgURL = entry.ImageURL
	}
	sub := podcast.Subscription{Title: title, FeedURL: feedURL, ImageURL: imgURL}
	if err := store.Add(sub); err != nil {
		return fmt.Errorf("add subscription: %w", err)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("save subscriptions: %w", err)
	}

	added := store.Get(feedURL)
	if !cli.Quiet && added != nil {
		fmt.Fprintf(outFor(cli), "Added podcast subscription: [%s] %s\n", util.BoldCyan(added.ID), added.Title)
		podDir := filepath.Join(cfg.PodcastsDir, added.Folder)
		if cfg.ServerBaseURL != "" {
			_ = podcast.PublishPodcast(podDir, *added, cfg.ServerBaseURL, eps)
			fmt.Fprintf(outFor(cli), "Local RSS feed available at: %s/%s/feed.xml\n", strings.TrimRight(cfg.ServerBaseURL, "/"), added.Folder)
		}
	}
	return nil
}

func handleServerRemove(cfg Config, cli CLIOptions) error {
	if len(cli.Args) == 0 {
		return fmt.Errorf("subscription ID or title is required")
	}
	query := cli.Args[0]
	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}

	removed, err := store.Remove(query)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("no subscription matching %q found", query)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("save subscriptions: %w", err)
	}

	fmt.Fprintf(progressFor(cli), "Removed subscription: %s\n", query)
	return nil
}

func handleServerFeed(cfg Config, cli CLIOptions) error {
	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}
	subs := store.List()
	if len(subs) == 0 {
		return fmt.Errorf("no subscriptions found in %s", store.FilePath())
	}

	target := ""
	if len(cli.Args) > 0 {
		target = cli.Args[0]
	}

	matched := 0
	for _, sub := range subs {
		if !podcast.SubscriptionMatches(sub, target) {
			continue
		}
		matched++
		podDir := filepath.Join(cfg.PodcastsDir, sub.Folder)
		if err := podcast.PublishPodcast(podDir, sub, cfg.ServerBaseURL, nil); err != nil {
			fmt.Fprintf(errFor(cli), "Warning: failed to write feed for %s: %v\n", sub.Title, err)
			continue
		}
		if sub.ImageURL == "" {
			if entry := library(cfg, cli, nil).FeedCache().Get(sub.FeedURL); entry != nil && entry.ImageURL != "" {
				sub.ImageURL = entry.ImageURL
				_ = store.Add(sub)
				_ = store.Save()
			}
		}
		eps := podcast.CollectLocalEpisodes(podDir, nil)
		fmt.Fprintf(outFor(cli), "Updated feed: %s (%d episodes)\n", filepath.Join(podDir, "feed.xml"), len(eps))
	}
	if matched > 0 && cfg.PodcastsDir != "" {
		// The catalogue lists every show, so it goes stale whenever any one
		// of them is republished — not only on a run with no target.
		_ = podcast.PublishCatalog(cfg.PodcastsDir, subs)
	}
	// A target that matches nothing used to print nothing and exit zero,
	// which reads as success: the feed it was asked to rebuild stays stale
	// and the caller has no way to tell.
	if target != "" && matched == 0 {
		return fmt.Errorf("no subscription matches %q; `pod info` lists them", target)
	}
	if target == "" && cfg.PodcastsDir != "" {
		fmt.Fprintf(outFor(cli), "Updated catalog webpage: %s\n", filepath.Join(cfg.PodcastsDir, "index.html"))
	}
	return nil
}

func handleServerImport(cfg Config, cli CLIOptions) error {
	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}
	if len(cli.Args) > 0 {
		data, err := os.ReadFile(cli.Args[0])
		if err != nil {
			return fmt.Errorf("read OPML file: %w", err)
		}
		n, err := store.ImportFromOPML(data)
		if err != nil {
			return fmt.Errorf("import OPML: %w", err)
		}
		fmt.Fprintf(progressFor(cli), "Imported %d new subscription(s) from %s\n", n, cli.Args[0])
		return nil
	}

	reader, err := backend.ReaderFromAppConfig(&cfg, reporter(cli))
	if err != nil {
		return fmt.Errorf("backend not available for import: %w", err)
	}
	n, err := store.ImportFromBackend(reader)
	if err != nil {
		return fmt.Errorf("backend import failed: %w", err)
	}
	fmt.Fprintf(progressFor(cli), "Imported %d new subscription(s) from backend into %s\n", n, store.FilePath())
	return nil
}

func renderSubscriptionList(w io.Writer, subs []podcast.Subscription, podcastsDir string, verbose bool) error {
	fmt.Fprintf(w, "%-8s %-32s %-6s %-20s %s\n", "ID", "TITLE", "EPS", "FOLDER", "FEED URL")
	fmt.Fprintln(w, strings.Repeat("-", 95))
	for _, s := range subs {
		title := util.TruncateDisplayName(s.Title, 30)
		folder := util.TruncateDisplayName(s.Folder, 18)
		epCount := 0
		if podcastsDir != "" {
			podDir := filepath.Join(podcastsDir, s.Folder)
			epCount = len(util.FindMP3Files(podDir))
		}
		feedURL := s.FeedURL
		if !verbose && len([]rune(feedURL)) > 35 {
			feedURL = util.Truncate(feedURL, 35)
		}
		fmt.Fprintf(w, "%-8s %s %-6d %s %s\n", s.ID, util.PadRight(title, 32), epCount, util.PadRight(folder, 20), feedURL)
	}
	fmt.Fprintf(w, "\nTotal: %d subscription(s)\n", len(subs))
	return nil
}

func subscriptionDownloadOptions(cfg Config, cli CLIOptions) podcast.SubscriptionDownloadOptions {
	target := cli.Podcast
	if target == "" && len(cli.Args) > 0 {
		target = cli.Args[0]
	}
	return podcast.SubscriptionDownloadOptions{
		Target:      target,
		Jobs:        cli.FeedJobs,
		Count:       cli.Count,
		CountGiven:  cli.CountGiven,
		DownloadAll: cli.DownloadAll,
		Defaults: config.PolicyDefaults{
			DownloadPolicy: cfg.DefaultDownloadPolicy,
			DownloadK:      cfg.DefaultDownloadK,
			AdRemoval:      cfg.DefaultAdRemoval,
		},
	}
}

func runSubscriptionDirectDownloads(store *podcast.SubscriptionStore, cfg Config, cli CLIOptions) error {
	subs := store.List()
	if len(subs) == 0 {
		return fmt.Errorf("no subscriptions found in %s", store.FilePath())
	}

	lib := library(cfg, cli, nil)
	opts := subscriptionDownloadOptions(cfg, cli)
	targets := podcast.SubscriptionTargets(subs, opts.Target)
	if len(targets) == 0 {
		fmt.Fprintln(progressFor(cli), "No matching podcast subscriptions found.")
		return nil
	}

	start := time.Now()
	plans := lib.PlanSubscriptionDownloads(targets, opts, feedCheckProgress(cli, len(targets)))
	fmt.Fprint(progressFor(cli), "\r\x1b[K")
	reportSubDownloadPlans(plans, time.Since(start), cli)

	if cli.DryRun {
		printDryRunPlans(plans, cli)
		return nil
	}

	res := lib.ExecuteSubscriptionDownloads(plans, store, opts)
	for _, err := range res.Failures {
		fmt.Fprintf(errFor(cli), "Warning: failed downloading %v\n", err)
	}
	if !cli.Quiet && res.Downloaded > 0 {
		fmt.Fprintf(outFor(cli), "Downloaded %d episode(s) across %d podcast(s).\n", res.Downloaded, res.Podcasts)
	}
	return nil
}

// feedCheckProgress returns the in-place counter shown while feeds are read,
// or nil when the caller asked for quiet.
func feedCheckProgress(cli CLIOptions, total int) func(done, total int) {
	if cli.Quiet {
		return nil
	}
	return func(done, total int) {
		fmt.Fprintf(outFor(cli), "\rChecking feeds for new episodes (%d/%d)...\x1b[K", done, total)
		os.Stdout.Sync()
	}
}

func reportSubDownloadPlans(plans []podcast.SubscriptionPlan, elapsed time.Duration, cli CLIOptions) {
	if cli.Quiet {
		return
	}
	selected, episodes, failed := 0, 0, 0
	for i := range plans {
		if plans[i].Err != nil {
			failed++
		}
		if len(plans[i].ToDownload) > 0 {
			selected++
			episodes += len(plans[i].ToDownload)
		}
	}
	fmt.Fprintf(outFor(cli), "Checked %d feed(s) in %.1fs: %d episode(s) to download across %d podcast(s)",
		len(plans), elapsed.Seconds(), episodes, selected)
	if failed > 0 {
		fmt.Fprintf(outFor(cli), ", %d unreadable", failed)
	}
	fmt.Fprintln(outFor(cli), ".")

	for i := range plans {
		pTitle := util.DisplayName(plans[i].Sub.Title)
		switch {
		case plans[i].Err != nil:
			fmt.Fprintf(outFor(cli), "  ! %s: %v\n", pTitle, plans[i].Err)
		case cli.Verbose && len(plans[i].ToDownload) == 0:
			fmt.Fprintf(outFor(cli), "  - %s: up to date\n", pTitle)
		}
	}
}

func printDryRunPlans(plans []podcast.SubscriptionPlan, cli CLIOptions) {
	if cli.Quiet {
		return
	}
	for _, plan := range plans {
		if len(plan.ToDownload) == 0 {
			continue
		}
		fmt.Fprintf(outFor(cli), "\n=== Podcast: %s ===\n", util.BoldCyan(plan.Sub.Title))
		fmt.Fprintf(outFor(cli), "Found %d episode(s) to download:\n", len(plan.ToDownload))
		for idx, ep := range plan.ToDownload {
			fmt.Fprintf(outFor(cli), "  %d. %s\n", idx+1, ep.Title)
		}
	}
}
