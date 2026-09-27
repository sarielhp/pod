package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"pod/pkg/episode"
	"pod/pkg/util"
	"reflect"
	"testing"
	"time"

	"pod/pkg/podcast"
)

func TestQueueTodaySelection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	zone, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, zone)
	dir, paths := createTestPodcastWithEpisodes(t, root, "Today", []string{"midnight", "late", "yesterday", "tomorrow", "unknown", "clean", "cached"})
	dates := []string{"2026-03-08T00:00:00-06:00", "2026-03-09T04:59:59Z", "2026-03-07T23:59:59-06:00", "2026-03-09T00:00:00-05:00", "", "2026-03-08T12:00:00-05:00", "invalid"}
	for i, path := range paths {
		st := episode.GetOrCreateEpisodeStatus(path)
		st.PublishedAt = dates[i]
		if err := episode.SaveEpisodeStatus(episode.StatusPathFor(path), st); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	markEpisodeClean(t, paths[5])
	index := &podcast.CachedPodcastIndex{Episodes: []podcast.CachedEpisodeSummary{
		{EpisodeFile: podcast.EpisodeFile{Filename: filepath.Base(paths[6]), PublishedAt: now.UnixMilli()}},
		{EpisodeFile: podcast.EpisodeFile{Filename: "unknown extended.mp3", PublishedAt: now.UnixMilli()}},
	}}
	for i, date := range dates[:6] {
		if published, err := time.Parse(time.RFC3339, date); err == nil {
			index.Episodes = append(index.Episodes, podcast.CachedEpisodeSummary{EpisodeFile: podcast.EpisodeFile{Path: paths[i], PublishedAt: published.UnixMilli()}})
		}
	}
	if err := podcast.SavePodcastCache(dir, index); err != nil {
		t.Fatal(err)
	}
	episode.AddToQueue(dir, "existing.mp3")
	if err := handleQueueToday(root, CLIOptions{
		ProcOptions: ProcOptions{
			DryRun: true,
		},
	}, now); err != nil {
		t.Fatal(err)
	}
	if got := readTodayTestQueue(t, dir); !reflect.DeepEqual(got, []string{"existing.mp3"}) {
		t.Fatalf("dry run changed queue: %v", got)
	}
	for i := 0; i < 2; i++ {
		if err := handleQueueToday(root, CLIOptions{
			ProcOptions: ProcOptions{
				Quiet: true,
			},
		}, now); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"existing.mp3", "cached.mp3", "late.mp3", "midnight.mp3"}
	if got := readTodayTestQueue(t, dir); !reflect.DeepEqual(got, want) {
		t.Fatalf("queue = %v, want %v", got, want)
	}
}

func TestQueueTodayRequiresSourcePublicationDate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	dir, paths := createTestPodcastWithEpisodes(t, root, "SourceDates", []string{"old", "today", "unknown"})
	for i, path := range paths {
		st := episode.GetOrCreateEpisodeStatus(path)
		st.PublishedAt = now.Format(time.RFC3339)
		if i == 1 {
			st.PublishedAt = now.AddDate(0, 0, -5).Format(time.RFC3339)
		}
		if err := episode.SaveEpisodeStatus(episode.StatusPathFor(path), st); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	cache := &podcast.CachedPodcastIndex{Episodes: []podcast.CachedEpisodeSummary{
		{EpisodeFile: podcast.EpisodeFile{Path: paths[0], PublishedAt: now.AddDate(0, 0, -5).UnixMilli()}},
		{EpisodeFile: podcast.EpisodeFile{Path: paths[1], PublishedAt: now.UnixMilli()}},
	}}
	if err := podcast.SavePodcastCache(dir, cache); err != nil {
		t.Fatal(err)
	}
	if err := handleQueueToday(root, CLIOptions{ProcOptions: ProcOptions{Quiet: true}}, now); err != nil {
		t.Fatal(err)
	}
	if got := readTodayTestQueue(t, dir); !reflect.DeepEqual(got, []string{"today.mp3"}) {
		t.Fatalf("queue = %v", got)
	}
	items, err := collectQueueDisplayItems(podcast.PodcastDirEntry{Dir: dir})
	if err != nil || len(items) != 1 {
		t.Fatalf("display disagrees with source publication: %+v, %v", items, err)
	}
	published, err := time.Parse(time.RFC3339, items[0].PublishedAt)
	if err != nil || !published.Equal(now) {
		t.Fatalf("display disagrees with source publication: %s", items[0].PublishedAt)
	}
}

func readTodayTestQueue(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestQueueTodayParsing(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"queue", "today", "--dry-run", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	if action != "queue" || opts.QueueSubcmd != "today" || !opts.DryRun || !opts.Quiet {
		t.Fatalf("unexpected parsed command: %s %+v", action, opts)
	}
	for _, args := range [][]string{{"queue", "today", "extra"}, {"download"}} {
		if err := app.Execute(args); err == nil {
			t.Fatalf("expected rejection for %v", args)
		}
	}
}

func TestQueueTodayNestedEpisodesResolveForRun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "show")
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	var episodes []podcast.CachedEpisodeSummary
	for _, name := range []string{"one", "two", "yesterday"} {
		path := filepath.Join(dir, name, "podcast.mp3")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
		date := now
		if name == "yesterday" {
			date = now.AddDate(0, 0, -1)
		}
		episodes = append(episodes, podcast.CachedEpisodeSummary{
			EpisodeFile: podcast.EpisodeFile{
				Path:        path,
				Filename:    "podcast.mp3",
				PublishedAt: date.UnixMilli(),
			},
		})
	}
	if err := podcast.SavePodcastCache(dir, &podcast.CachedPodcastIndex{Episodes: episodes}); err != nil {
		t.Fatal(err)
	}
	opts := CLIOptions{ProcOptions: ProcOptions{Quiet: true}}
	for i := 0; i < 2; i++ {
		if err := handleQueueToday(root, opts, now); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{filepath.Join("one", "podcast.mp3"), filepath.Join("two", "podcast.mp3")}
	if got := readTodayTestQueue(t, dir); !reflect.DeepEqual(got, want) {
		t.Fatalf("queue = %v, want %v", got, want)
	}
	items, err := podcast.Open(podcast.Config{PodcastsDir: root}, nil, nil).QueueItems("")
	if err != nil || len(items) != 2 {
		t.Fatalf("run items = %v, error = %v", items, err)
	}
	for i, item := range items {
		if item.AudioPath != filepath.Join(dir, want[i]) || !util.FileExists(item.AudioPath) {
			t.Fatalf("queue run resolved incorrect path: %s", item.AudioPath)
		}
	}
}
