package cli

import (
	"fmt"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strconv"

	"github.com/sarielhp/clihelp"
)

func buildServerListSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "list",
		Description: "List all available podcasts in the library with episode counts",
		UsageLine:   "pod library list [options]",
		Args:        clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed output (Feed URLs and IDs)"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "list"
			opts.SyncSubcmd = "list"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildServerGetInfoSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "get-info",
		Description: "Cache metadata for latest K episodes",
		UsageLine:   "pod library get-info [<k>] [options]",
		Parameters:  []clihelp.Param{{Name: "[<k>]", Description: "Episodes count per podcast (default 100)"}},
		Args:        clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Specify podcast by name, index, or ID"),
			clihelp.Bool(&opts.Refresh, "-r, --refresh", false, "Force re-fetching metadata from feed"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed per-episode metadata"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "get-info"
			opts.SyncSubcmd = "get-info"
			opts.Count = 100
			opts.CountGiven = false
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 {
				if k, err := strconv.Atoi(ctx.Args[0]); err == nil && k > 0 {
					opts.Count = k
					opts.CountGiven = true
				}
			}
			return nil
		},
	}
}

func handleServerList(cfg Config, cli CLIOptions) error {
	if backend.IsStandalone(&cfg) {
		store, storeErr := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
		if storeErr == nil {
			return renderSubscriptionList(outFor(cli), store.List(), cfg.PodcastsDir, cli.Verbose)
		}
		return fmt.Errorf("load subscriptions: %w", storeErr)
	}

	b, err := backend.FromAppConfig(&cfg, reporter(cli))
	if err != nil {
		store, storeErr := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
		if storeErr == nil && len(store.List()) > 0 {
			return renderSubscriptionList(outFor(cli), store.List(), cfg.PodcastsDir, cli.Verbose)
		}
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	podcasts, err := b.Podcasts()
	if err != nil {
		return fmt.Errorf("failed to fetch podcasts: %w", err)
	}
	if len(podcasts) == 0 {
		fmt.Fprintln(progressFor(cli), "No podcasts found on server.")
		return nil
	}
	for idx, p := range podcasts {
		shortID := podcast.GeneratePodcastShortID(p.Media.Metadata.Title)
		fmt.Fprintf(outFor(cli), "%3d. %s [%s] (%d episodes)\n", idx+1, util.Bold(util.DisplayName(p.Media.Metadata.Title)), util.BoldCyan(shortID), len(p.Media.Episodes))
		if cli.Verbose {
			fmt.Fprintf(outFor(cli), "     ID:      %s\n", p.ID)
			if p.Media.Metadata.FeedURL != "" {
				fmt.Fprintf(outFor(cli), "     Feed:    %s\n", p.Media.Metadata.FeedURL)
			}
			if p.RelPath != "" {
				fmt.Fprintf(outFor(cli), "     Folder:  %s\n", p.RelPath)
			}
		}
	}
	return nil
}

func handleServerGetInfo(config Config, cli CLIOptions) error {
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err != nil {
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	podcasts, err := b.Podcasts()
	if err != nil {
		return fmt.Errorf("failed to fetch podcasts: %w", err)
	}
	totalEpisodes := 0
	for _, p := range podcasts {
		totalEpisodes += len(p.Media.Episodes)
	}
	fmt.Fprintf(outFor(cli), "Server:          %s\n", b.Name())
	fmt.Fprintf(outFor(cli), "Total Podcasts:  %d\n", len(podcasts))
	fmt.Fprintf(outFor(cli), "Total Episodes:  %d\n", totalEpisodes)
	return nil
}
