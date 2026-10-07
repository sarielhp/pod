package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/podcast"
	"pod/pkg/podcastsearch"
)

type fakeSearcher struct {
	name    string
	results []podcastsearch.Result
	err     error
}

func (f fakeSearcher) Search(context.Context, string, int) ([]podcastsearch.Result, error) {
	return f.results, f.err
}

func (f fakeSearcher) ByID(context.Context, string) (podcastsearch.Result, error) {
	return podcastsearch.Result{}, f.err
}

func (f fakeSearcher) Name() string {
	if f.name != "" {
		return f.name
	}
	return "Fake Directory"
}

func TestSearchCommandWiring(t *testing.T) {
	t.Parallel()

	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"search", "joe", "rogan"}); err != nil {
		t.Fatalf("search: %v", err)
	}
	if action != "search" || opts.SearchSubcmd != "" {
		t.Fatalf("action=%q subcmd=%q", action, opts.SearchSubcmd)
	}
	if strings.Join(opts.Args, " ") != "joe rogan" {
		t.Fatalf("query args = %v", opts.Args)
	}

	action, opts = "", CLIOptions{}
	app = buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"search", "add", "75075"}); err != nil {
		t.Fatalf("search add: %v", err)
	}
	if action != "search" || opts.SearchSubcmd != "add" {
		t.Fatalf("action=%q subcmd=%q", action, opts.SearchSubcmd)
	}
	if len(opts.Args) != 1 || opts.Args[0] != "75075" {
		t.Fatalf("add args = %v", opts.Args)
	}
}

func TestRunSearchQueryRendersResults(t *testing.T) {
	restore := searchSearcher
	t.Cleanup(func() { searchSearcher = restore })
	searchSearcher = func(Config) podcastsearch.Searcher {
		return fakeSearcher{results: []podcastsearch.Result{{
			ID: "75075", Title: "The Daily", Author: "The New York Times",
			FeedURL: "http://feed.example/daily", EpisodeCount: 2100,
		}}}
	}

	var buf bytes.Buffer
	cli := CLIOptions{Args: []string{"the", "daily"}, Out: &buf}
	if err := runSearchQuery(Config{}, cli); err != nil {
		t.Fatalf("runSearchQuery: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"The Daily", "75075", "pod search add <ID>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunSearchQueryJSON(t *testing.T) {
	restore := searchSearcher
	t.Cleanup(func() { searchSearcher = restore })
	searchSearcher = func(Config) podcastsearch.Searcher {
		return fakeSearcher{name: "Podcast Index", results: []podcastsearch.Result{{ID: "1", Title: "Show", FeedURL: "http://f/x"}}}
	}

	var buf bytes.Buffer
	cli := CLIOptions{Args: []string{"show"}, Out: &buf, JSON: true}
	if err := runSearchQuery(Config{}, cli); err != nil {
		t.Fatalf("runSearchQuery: %v", err)
	}
	var got searchJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if got.Source != "Podcast Index" || got.Query != "show" || len(got.Results) != 1 {
		t.Fatalf("unexpected JSON: %+v", got)
	}
}

func TestRunSearchQueryRequiresAQuery(t *testing.T) {
	if err := runSearchQuery(Config{}, CLIOptions{}); err == nil {
		t.Fatal("expected an error for an empty query")
	}
}

func TestRunSearchAddSubscribes(t *testing.T) {
	restore := searchResolver
	t.Cleanup(func() { searchResolver = restore })
	searchResolver = func(Config, string) (podcastsearch.Result, error) {
		return podcastsearch.Result{ID: "75075", Title: "The Daily", FeedURL: "http://feed.example/daily", ImageURL: "http://img/x.png"}, nil
	}

	dir := t.TempDir()
	cfg := Config{PodcastsDir: dir, SubscriptionsFile: filepath.Join(dir, "podcasts.json")}
	cli := CLIOptions{Args: []string{"75075"}, Out: io.Discard}

	if err := runSearchAdd(cfg, cli); err != nil {
		t.Fatalf("runSearchAdd: %v", err)
	}
	store, err := podcast.NewSubscriptionStore(cfg.SubscriptionsFile)
	if err != nil {
		t.Fatal(err)
	}
	subs := store.List()
	if len(subs) != 1 {
		t.Fatalf("got %d subscriptions, want 1", len(subs))
	}
	if subs[0].Title != "The Daily" || subs[0].FeedURL != "http://feed.example/daily" || subs[0].ImageURL != "http://img/x.png" {
		t.Fatalf("unexpected subscription: %+v", subs[0])
	}

	var buf bytes.Buffer
	cli.Out = &buf
	if err := runSearchAdd(cfg, cli); err != nil {
		t.Fatalf("second runSearchAdd: %v", err)
	}
	if !strings.Contains(buf.String(), "Already subscribed") {
		t.Fatalf("re-subscribing should say so, got %q", buf.String())
	}
	if subs := store.List(); len(subs) != 1 {
		t.Fatalf("re-subscribing created a duplicate: %d", len(subs))
	}
}

func TestRunSearchAddProbesFeedTitle(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Probed Show</title><item><title>Ep</title><enclosure url="http://example.com/1.mp3" type="audio/mpeg"/></item></channel></rss>`)
	}))
	defer feed.Close()

	restore := searchResolver
	t.Cleanup(func() { searchResolver = restore })
	searchResolver = func(Config, string) (podcastsearch.Result, error) {
		return podcastsearch.Result{FeedURL: feed.URL + "/feed.xml"}, nil
	}

	dir := t.TempDir()
	cfg := Config{PodcastsDir: dir, SubscriptionsFile: filepath.Join(dir, "podcasts.json")}
	if err := runSearchAdd(cfg, CLIOptions{Args: []string{feed.URL + "/feed.xml"}, Out: io.Discard}); err != nil {
		t.Fatalf("runSearchAdd: %v", err)
	}
	store, err := podcast.NewSubscriptionStore(cfg.SubscriptionsFile)
	if err != nil {
		t.Fatal(err)
	}
	subs := store.List()
	if len(subs) != 1 || subs[0].Title != "Probed Show" {
		t.Fatalf("expected the feed title to be probed, got %+v", subs)
	}
}
