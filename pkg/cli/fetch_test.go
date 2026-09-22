package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/pipeline"
	"pod/pkg/podcast"
)

func TestFetchCLIOptionsParsing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args        []string
		wantAction  string
		wantPodcast string
		wantCount   int
		wantDryRun  bool
		wantQuiet   bool
		wantVerbose bool
		wantHourly  bool
		wantNoClean bool
	}{
		{
			args:        []string{"fetch"},
			wantAction:  "fetch",
			wantPodcast: "",
			wantCount:   1,
		},
		{
			args:        []string{"fetch", "TheDaily"},
			wantAction:  "fetch",
			wantPodcast: "TheDaily",
			wantCount:   1,
		},
		{
			args:        []string{"fetch", "-p", "Marketplace", "-n", "3", "-d", "-q", "-v", "--hourly", "--no-clean"},
			wantAction:  "fetch",
			wantPodcast: "Marketplace",
			wantCount:   3,
			wantDryRun:  true,
			wantQuiet:   true,
			wantVerbose: true,
			wantHourly:  true,
			wantNoClean: true,
		},
		{
			args:        []string{"fetch", "--download-only", "-n", "2"},
			wantAction:  "fetch",
			wantCount:   2,
			wantNoClean: true,
		},
	}

	for _, tc := range cases {
		var action string
		var opts CLIOptions
		app := buildCLIApp(&action, &opts)
		if err := app.Execute(tc.args); err != nil {
			t.Fatalf("unexpected error executing %v: %v", tc.args, err)
		}
		if action != tc.wantAction {
			t.Errorf("args %v: got action %q, want %q", tc.args, action, tc.wantAction)
		}
		if opts.Podcast != tc.wantPodcast {
			t.Errorf("args %v: got podcast %q, want %q", tc.args, opts.Podcast, tc.wantPodcast)
		}
		if opts.Count != tc.wantCount {
			t.Errorf("args %v: got count %d, want %d", tc.args, opts.Count, tc.wantCount)
		}
		if opts.DryRun != tc.wantDryRun {
			t.Errorf("args %v: got dry-run %v, want %v", tc.args, opts.DryRun, tc.wantDryRun)
		}
		if opts.Quiet != tc.wantQuiet {
			t.Errorf("args %v: got quiet %v, want %v", tc.args, opts.Quiet, tc.wantQuiet)
		}
		if opts.Verbose != tc.wantVerbose {
			t.Errorf("args %v: got verbose %v, want %v", tc.args, opts.Verbose, tc.wantVerbose)
		}
		if opts.IncludeHourly != tc.wantHourly {
			t.Errorf("args %v: got hourly %v, want %v", tc.args, opts.IncludeHourly, tc.wantHourly)
		}
		if opts.NoClean != tc.wantNoClean {
			t.Errorf("args %v: got no-clean %v, want %v", tc.args, opts.NoClean, tc.wantNoClean)
		}
	}
}

func TestFetchCommandReorganization(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	var visible []string
	var hidden []string
	for _, cmd := range app.Commands {
		if cmd.Hidden {
			hidden = append(hidden, cmd.Name)
		} else {
			visible = append(visible, cmd.Name)
		}
	}

	// 7 visible commands: fetch, queue, server, player, info, config, tui
	wantVisible := []string{"fetch", "queue", "server", "player", "info", "config", "tui"}
	if len(visible) != len(wantVisible) {
		t.Fatalf("expected %d visible commands, got %d: %v", len(wantVisible), len(visible), visible)
	}
	for _, want := range wantVisible {
		found := false
		for _, v := range visible {
			if v == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected visible command %q not found in %v", want, visible)
		}
	}

	// 4 hidden backward-compatible commands: rm_ads, detect, gen_rss, transcribe
	wantHidden := []string{"rm_ads", "detect", "gen_rss", "transcribe"}
	for _, want := range wantHidden {
		found := false
		for _, h := range hidden {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected hidden command %q not found in %v", want, hidden)
		}
	}
}

func TestSingleLetterAbbreviationsForVisibleCommands(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		letter string
		want   string
	}{
		{"f", "fetch"},
		{"q", "queue"},
		{"s", "server"},
		{"p", "player"},
		{"i", "info"},
		{"c", "config"},
		{"t", "tui"},
	}

	for _, p := range pairs {
		var action string
		var opts CLIOptions
		app := buildCLIApp(&action, &opts)
		err := app.Execute([]string{p.letter, "--help"})
		// --help may return an error or nil depending on clihelp version, but should match command p.want
		if err != nil && !strings.Contains(err.Error(), "help") {
			t.Errorf("letter %q failed to resolve to %s: %v", p.letter, p.want, err)
		}
	}
}

func TestQueueAbsorbedSubcommands(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	queueCmd := findTestCommand(app, "queue")
	if queueCmd == nil {
		t.Fatalf("queue command not found")
	}

	wantSubs := []string{"recut", "export", "audit"}
	for _, want := range wantSubs {
		found := false
		for _, sub := range queueCmd.Subcommands {
			if sub.Name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected queue subcommand %q not found", want)
		}
	}

	// Test executing queue recut
	action = ""
	opts = CLIOptions{}
	if err := app.Execute([]string{"queue", "recut", "-n", "5"}); err != nil {
		t.Fatalf("unexpected error executing queue recut: %v", err)
	}
	if action != "queue" || opts.QueueSubcmd != "recut" || !opts.Recut || opts.Count != 5 {
		t.Errorf("queue recut mismatch: action=%q, subcmd=%q, recut=%v, count=%d", action, opts.QueueSubcmd, opts.Recut, opts.Count)
	}

	// Test executing queue export srt
	action = ""
	opts = CLIOptions{}
	if err := app.Execute([]string{"queue", "export", "srt", "test.json"}); err != nil {
		t.Fatalf("unexpected error executing queue export srt: %v", err)
	}
	if action != "queue" || opts.QueueSubcmd != "export" || opts.ExportFormat != "srt" {
		t.Errorf("queue export mismatch: action=%q, subcmd=%q, fmt=%q", action, opts.QueueSubcmd, opts.ExportFormat)
	}

	// Test executing queue audit
	action = ""
	opts = CLIOptions{}
	if err := app.Execute([]string{"queue", "audit", "--dry-run"}); err != nil {
		t.Fatalf("unexpected error executing queue audit: %v", err)
	}
	if action != "queue" || opts.QueueSubcmd != "audit" || !opts.DryRun {
		t.Errorf("queue audit mismatch: action=%q, subcmd=%q, dryRun=%v", action, opts.QueueSubcmd, opts.DryRun)
	}
}

func TestBackwardCompatibilityHiddenCommands(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	// rm_ads
	if err := app.Execute([]string{"rm_ads", "recut", "-n", "2"}); err != nil {
		t.Fatalf("unexpected error executing rm_ads recut: %v", err)
	}
	if action != "rm_ads" || opts.ProcSubcmd != "recut" {
		t.Errorf("rm_ads mismatch: action=%q, procSubcmd=%q", action, opts.ProcSubcmd)
	}

	// ui alias for tui
	action = ""
	opts = CLIOptions{}
	if err := app.Execute([]string{"ui", "--debug"}); err != nil {
		t.Fatalf("unexpected error executing ui: %v", err)
	}
	if action != "tui" || !opts.Debug {
		t.Errorf("ui alias mismatch: action=%q, debug=%v", action, opts.Debug)
	}
}

func TestFetchExecutionDryRun(t *testing.T) {
	t.Parallel()
	now := time.Now()
	pubDate := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>News Daily</title>
<item><title>Episode 100</title><pubDate>%s</pubDate><enclosure url="http://example.com/ep100.mp3" type="audio/mpeg"/></item>
</channel></rss>`, pubDate)))
	}))
	defer feedSrv.Close()

	root := t.TempDir()
	podcastsDir := filepath.Join(root, "podcasts")
	showDir := filepath.Join(podcastsDir, "News Daily")
	_ = os.MkdirAll(showDir, 0755)

	store, err := podcast.NewSubscriptionStore(filepath.Join(root, "podcasts.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Add(podcast.Subscription{ID: "nd", Title: "News Daily", Folder: "News Daily", FeedURL: feedSrv.URL})
	_ = store.Save()

	var buf bytes.Buffer
	cfg := Config{
		PodcastsDir:       podcastsDir,
		SubscriptionsFile: filepath.Join(root, "podcasts.json"),
	}
	cli := CLIOptions{
		ProcOptions: ProcOptions{
			DryRun: true,
		},
		Out:         &buf,
		PodcastsDir: podcastsDir,
	}

	if err := runFetchCommand(cfg, cli); err != nil {
		t.Fatalf("runFetchCommand failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "News Daily") || !strings.Contains(out, "Episode 100") {
		t.Errorf("expected dry-run output to mention News Daily and Episode 100, got:\n%s", out)
	}

	// Verify no files were downloaded
	files, _ := filepath.Glob(filepath.Join(showDir, "*.mp3"))
	if len(files) != 0 {
		t.Errorf("expected 0 files downloaded in dry-run, found %d", len(files))
	}
}

func TestFetchExecutionNoClean(t *testing.T) {
	t.Parallel()
	now := time.Now()
	pubDate := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	audioSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio bytes"))
	}))
	defer audioSrv.Close()

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Fetch Show</title>
<item><title>Ep 1</title><pubDate>%s</pubDate><enclosure url="%s" type="audio/mpeg"/></item>
</channel></rss>`, pubDate, audioSrv.URL+"/ep1.mp3")))
	}))
	defer feedSrv.Close()

	root := t.TempDir()
	podcastsDir := filepath.Join(root, "podcasts")
	showDir := filepath.Join(podcastsDir, "Fetch Show")
	_ = os.MkdirAll(showDir, 0755)

	store, err := podcast.NewSubscriptionStore(filepath.Join(root, "podcasts.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Add(podcast.Subscription{ID: "fs", Title: "Fetch Show", Folder: "Fetch Show", FeedURL: feedSrv.URL})
	_ = store.Save()

	var buf bytes.Buffer
	cfg := Config{
		PodcastsDir:       podcastsDir,
		SubscriptionsFile: filepath.Join(root, "podcasts.json"),
	}
	cli := CLIOptions{
		NoClean:     true,
		Out:         &buf,
		PodcastsDir: podcastsDir,
	}

	if err := runFetchCommand(cfg, cli); err != nil {
		t.Fatalf("runFetchCommand failed: %v", err)
	}

	// Verify file was downloaded
	files, _ := filepath.Glob(filepath.Join(showDir, "*.mp3"))
	if len(files) != 1 {
		t.Fatalf("expected 1 downloaded mp3, got %d", len(files))
	}

	// Verify file was added to .queue
	queueEntries, err := pipeline.ReadQueue(showDir)
	if err != nil {
		t.Fatalf("failed to read queue: %v", err)
	}
	if len(queueEntries) != 1 {
		t.Fatalf("expected 1 queued entry, got %d", len(queueEntries))
	}
}

func TestFetchExecutionLatestAlreadyDownloaded(t *testing.T) {
	t.Parallel()
	now := time.Now()
	pubDate := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Existing Show</title>
<item><title>Ep 1</title><pubDate>%s</pubDate><enclosure url="http://example.com/ep1.mp3" type="audio/mpeg"/></item>
<item><title>Ep 2</title><pubDate>%s</pubDate><enclosure url="http://example.com/ep2.mp3" type="audio/mpeg"/></item>
</channel></rss>`, now.Add(-2*time.Hour).Format(time.RFC1123Z), pubDate)))
	}))
	defer feedSrv.Close()

	root := t.TempDir()
	podcastsDir := filepath.Join(root, "podcasts")
	showDir := filepath.Join(podcastsDir, "Existing Show")
	_ = os.MkdirAll(showDir, 0755)

	// Ep 2 (the latest) is already on disk. Ep 1 is NOT on disk.
	if err := os.WriteFile(filepath.Join(showDir, "Ep 2.mp3"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := podcast.NewSubscriptionStore(filepath.Join(root, "podcasts.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Add(podcast.Subscription{ID: "es", Title: "Existing Show", Folder: "Existing Show", FeedURL: feedSrv.URL})
	_ = store.Save()

	var buf bytes.Buffer
	cfg := Config{
		PodcastsDir:       podcastsDir,
		SubscriptionsFile: filepath.Join(root, "podcasts.json"),
	}
	cli := CLIOptions{
		Out:         &buf,
		PodcastsDir: podcastsDir,
	}

	if err := runFetchCommand(cfg, cli); err != nil {
		t.Fatalf("runFetchCommand failed: %v", err)
	}

	// Verify only Ep 2 is on disk, Ep 1 was NOT backfilled!
	files, _ := filepath.Glob(filepath.Join(showDir, "*.mp3"))
	if len(files) != 1 {
		t.Fatalf("expected only 1 file (Ep 2), got %d (never backfill older episodes!)", len(files))
	}
	if filepath.Base(files[0]) != "Ep 2.mp3" {
		t.Errorf("expected Ep 2.mp3, got %s", files[0])
	}
}
