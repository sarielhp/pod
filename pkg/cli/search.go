package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/podcastsearch"
	"pod/pkg/util"
)

const defaultSearchLimit = podcastsearch.DefaultMax

// searchSearcher and searchResolver are seams. A test replaces them with a
// searcher pointed at a local server, so the command's rendering and its
// subscription path can be exercised without reaching the network.
var (
	searchSearcher = func(cfg Config) podcastsearch.Searcher {
		return podcastsearch.New(podcastIndexCredentials(cfg))
	}
	searchResolver = func(cfg Config, idOrURL string) (podcastsearch.Result, error) {
		return podcastsearch.Resolve(context.Background(), podcastIndexCredentials(cfg), idOrURL)
	}
)

func buildSearchCommand(opts *CLIOptions, action *string, limitVal *int) clihelp.Command {
	return clihelp.Command{
		Name:        "search",
		Description: "Search for podcasts by name and subscribe to one",
		UsageLine:   "pod search <query> [options]",
		Parameters: []clihelp.Param{
			{Name: "<query>", Description: "Title or keywords to search for"},
		},
		Subcommands: []clihelp.Command{buildSearchAddSubcommand(opts, action)},
		Options: []clihelp.Option{
			clihelp.Int(limitVal, "-n, --limit <number>", defaultSearchLimit, "Maximum number of results"),
			clihelp.Bool(&opts.JSON, "--json", false, "Emit results as JSON"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
		},
		Examples: []clihelp.Example{
			{Line: "pod search huberman", Description: "Search for a show by name"},
			{Line: "pod search \"the daily\" -n 5", Description: "Limit the search to five results"},
			{Line: "pod search add 75075", Description: "Subscribe to the result with that ID"},
			{Line: "pod search add https://example.com/feed.xml", Description: "Subscribe to a feed URL directly"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "search"
			opts.SearchSubcmd = ""
			opts.Args = ctx.Args
			if *limitVal <= 0 {
				*limitVal = defaultSearchLimit
			}
			opts.Count = *limitVal
			return nil
		},
	}
}

func buildSearchAddSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "add",
		Description: "Subscribe to a podcast by its search result ID or feed URL",
		UsageLine:   "pod search add <id-or-feed-url>",
		Parameters: []clihelp.Param{
			{Name: "<id-or-feed-url>", Description: "ID shown by `pod search`, or an RSS feed URL"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "search"
			opts.SearchSubcmd = "add"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func runSearchCommand(cfg Config, cli CLIOptions) error {
	if cli.SearchSubcmd == "add" {
		return runSearchAdd(cfg, cli)
	}
	return runSearchQuery(cfg, cli)
}

func podcastIndexCredentials(cfg Config) podcastsearch.Credentials {
	key, secret := config.ResolvePodcastIndexCredentials(&cfg)
	return podcastsearch.Credentials{APIKey: key, APISecret: secret}
}

func runSearchQuery(cfg Config, cli CLIOptions) error {
	query := strings.TrimSpace(strings.Join(cli.Args, " "))
	if query == "" {
		return fmt.Errorf("a search query is required: pod search <query>")
	}
	limit := cli.Count
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	searcher := searchSearcher(cfg)
	results, err := searcher.Search(context.Background(), query, limit)
	if err != nil {
		return err
	}
	if cli.JSON {
		return writeSearchJSON(outFor(cli), searcher.Name(), query, results)
	}
	renderSearchResults(outFor(cli), searcher.Name(), query, results)
	if !podcastIndexCredentials(cfg).Configured() {
		fmt.Fprintln(progressFor(cli), "\nUsing Apple Podcasts search. Set PODCASTINDEX_API_KEY and PODCASTINDEX_API_SECRET (or a key file) for Podcast Index results.")
	}
	return nil
}

func runSearchAdd(cfg Config, cli CLIOptions) error {
	if len(cli.Args) == 0 {
		return fmt.Errorf("an ID or feed URL is required: pod search add <id-or-feed-url>")
	}
	arg := strings.TrimSpace(cli.Args[0])
	res, err := searchResolver(cfg, arg)
	if err != nil {
		return err
	}
	feedURL := strings.TrimSpace(res.FeedURL)
	if feedURL == "" {
		return fmt.Errorf("no RSS feed URL found for %q", arg)
	}

	title, imageURL := strings.TrimSpace(res.Title), strings.TrimSpace(res.ImageURL)
	if title == "" {
		if probedTitle, probedImage, probeErr := podcast.ProbeFeed(feedURL); probeErr == nil {
			title = probedTitle
			if imageURL == "" {
				imageURL = probedImage
			}
		}
	}

	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}
	if existing := subscriptionForFeedURL(store.List(), feedURL); existing != nil {
		if !cli.Quiet {
			fmt.Fprintf(outFor(cli), "Already subscribed: [%s] %s\n", util.BoldCyan(existing.ID), existing.Title)
		}
		return nil
	}

	sub := podcast.Subscription{Title: title, FeedURL: feedURL, ImageURL: imageURL}
	if err := store.Add(sub); err != nil {
		return fmt.Errorf("add subscription: %w", err)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("save subscriptions: %w", err)
	}

	added := subscriptionForFeedURL(store.List(), feedURL)
	if added == nil || cli.Quiet {
		return nil
	}
	fmt.Fprintf(outFor(cli), "Subscribed: [%s] %s\n", util.BoldCyan(added.ID), added.Title)
	fmt.Fprintf(outFor(cli), "Feed: %s\n", added.FeedURL)
	return nil
}

func subscriptionForFeedURL(subs []podcast.Subscription, feedURL string) *podcast.Subscription {
	for i := range subs {
		if subs[i].FeedURL == feedURL {
			return &subs[i]
		}
	}
	return nil
}

type searchJSON struct {
	Source  string                 `json:"source"`
	Query   string                 `json:"query"`
	Results []podcastsearch.Result `json:"results"`
}

func writeSearchJSON(w io.Writer, source, query string, results []podcastsearch.Result) error {
	if results == nil {
		results = []podcastsearch.Result{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(searchJSON{Source: source, Query: query, Results: results})
}

func renderSearchResults(w io.Writer, source, query string, results []podcastsearch.Result) {
	fmt.Fprintf(w, "Searching %s for %q...\n\n", source, query)
	if len(results) == 0 {
		fmt.Fprintln(w, "No podcasts found.")
		return
	}
	fmt.Fprintf(w, "%3s  %-11s  %-32s  %-22s  %5s\n", "#", "ID", "TITLE", "AUTHOR", "EPS")
	fmt.Fprintln(w, strings.Repeat("-", 80))
	for i, r := range results {
		fmt.Fprintf(w, "%3d  %s  %-32s  %-22s  %5d\n",
			i+1,
			util.BoldCyan(util.PadRight(r.ID, 11)),
			util.PadRight(util.TruncateDisplayName(r.Title, 30), 32),
			util.PadRight(util.TruncateDisplayName(r.Author, 20), 22),
			r.EpisodeCount,
		)
		if r.FeedURL != "" {
			fmt.Fprintf(w, "     %s\n", util.Truncate(r.FeedURL, 100))
		}
	}
	fmt.Fprintln(w, "\nSubscribe with: pod search add <ID>")
}
