package cli

import (
	"fmt"

	"github.com/sarielhp/clihelp"

	"pod/pkg/backend"
	"pod/pkg/podcast"
)

func buildServerIdentifySubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "identify",
		Description: "Record which feed episode each downloaded file is, so nothing depends on file names",
		UsageLine:   "pod server identify [-p <podcast>] [--dry-run]",
		Args:        clihelp.MaximumNArgs(0),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Limit to one podcast"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Report what would be recorded without writing anything"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress the per-podcast lines"),
		},
		Run: func(_ *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "identify"
			return nil
		},
	}
}

func handleServerIdentify(cfg Config, cli CLIOptions) error {
	lib := library(cfg, cli, nil)
	if lib.Config().PodcastsDir == "" {
		return fmt.Errorf("podcasts_dir is not configured")
	}
	entries, err := pruneScope(lib, cli)
	if err != nil {
		return err
	}
	store, err := lib.Subscriptions()
	if err != nil {
		return err
	}
	fetch := func(url string) ([]backend.FeedEpisode, error) {
		eps, _, _, _, err := podcast.FetchFeedDirect(url, "", "")
		return eps, err
	}
	results := lib.IdentifyLibrary(entries, store.List(), fetch, cli.DryRun)
	printIdentifyResults(cli, results)
	return nil
}

func printIdentifyResults(cli CLIOptions, results []podcast.IdentifyResult) {
	var total, already, fromFeed, synthetic, audioless int
	w := progressFor(cli)
	for _, r := range results {
		total += r.Total
		audioless += r.Audioless
		already += r.Already
		fromFeed += r.FromFeed
		synthetic += r.Synthetic
		if r.FromFeed+r.Synthetic == 0 {
			continue
		}
		fmt.Fprintf(w, "%s: %d matched to the feed, %d local\n", listTitle(r.Title), r.FromFeed, r.Synthetic)
		if r.FeedErr != nil {
			fmt.Fprintf(errFor(cli), "  feed unavailable (%v); recorded from local information only\n", r.FeedErr)
		}
	}
	verb := "Recorded"
	if cli.DryRun {
		verb = "Would record"
	}
	fmt.Fprintf(outFor(cli), "\n%d episode(s), %d of them with their audio gone: %d already identified. %s %d matched to the feed and %d local identities.\n",
		total, audioless, already, verb, fromFeed, synthetic)
}
