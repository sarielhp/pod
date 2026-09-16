package cli

import (
	"fmt"
	"strconv"

	"pod/pkg/adremoval"
	"pod/pkg/config"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

// genRSSCount reads a count argument. `pod gen_rss 10` asks for the ten newest
// episodes; anything else is a podcast to regenerate.
func genRSSCount(args []string) (int, bool) {
	if len(args) != 1 {
		return 0, false
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// handleGenRSSLatest fetches, cleans and publishes the newest episodes.
//
// It is one command because the three steps are one intention: make the most
// recently published episodes available, ad-free, on the site. Doing it by
// hand meant `server download`, then `rm_ads` per episode, then `gen_rss`, and
// forgetting the last one left the site describing audio that had changed.
func handleGenRSSLatest(cfg Config, cli CLIOptions, count int) error {
	lib := library(cfg, cli, nil)
	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}
	subs := store.List()
	if len(subs) == 0 {
		return fmt.Errorf("no subscriptions found in %s", store.FilePath())
	}

	out := outFor(cli)
	sel := lib.PlanLatestEpisodes(subs, podcast.LatestEpisodePlanOptions{
		Count:         count,
		IncludeHourly: cli.IncludeHourly,
	})
	targets := sel.Paths()
	if len(targets) == 0 {
		fmt.Fprintln(out, "No recently published episodes found. Run `pod server feeds` first.")
		return nil
	}

	pending := 0
	for _, p := range sel.Plans {
		pending += len(p.ToDownload)
	}
	fmt.Fprintf(out, "Latest %d episode(s): %d to download, %d already here.\n",
		len(targets), pending, len(sel.Existing))

	if pending > 0 {
		res := lib.ExecuteSubscriptionDownloads(sel.Plans, store, subscriptionDownloadOptions(cfg, cli))
		fmt.Fprintf(out, "Downloaded %d episode(s) across %d podcast(s).\n", res.Downloaded, res.Podcasts)
		for _, e := range res.Failures {
			fmt.Fprintf(errFor(cli), "  download failed: %v\n", e)
		}
	}

	return cleanAndPublish(cfg, cli, targets)
}

// cleanAndPublish removes advertisements from each episode that still needs it
// and regenerates the site.
func cleanAndPublish(cfg Config, cli CLIOptions, targets []string) error {
	out := outFor(cli)

	var work []string
	for _, path := range targets {
		if !util.FileExists(path) {
			continue
		}
		if pipeline.IsEpisodeClean(path) {
			continue
		}
		work = append(work, path)
	}

	if len(work) == 0 {
		fmt.Fprintln(out, "Every episode is already ad-free.")
	} else {
		fmt.Fprintf(out, "Removing advertisements from %d episode(s)...\n", len(work))
		adremoval.ProcessFiles(work, cli.ProcOptions, cfg, "rm_ads")
	}

	// Republish unconditionally. Ad removal republishes the shows it touched,
	// but the catalogue and any show whose episodes were only downloaded still
	// need writing, and the whole point of the command is that the site is
	// correct when it returns.
	published := cli
	published.Args = nil
	return handleServerFeed(cfg, published)
}
