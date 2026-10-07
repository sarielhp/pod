package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/podcast"
)

func TestServerDownloadArguments(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"My Show", "123"} {
		for _, args := range [][]string{
			{"library", "download", target, "-k", "3"},
			{"library", "download", "-k", "3", target},
			{"library", "download", "-p", target, "-k", "3"},
		} {
			var action string
			var opts CLIOptions
			if err := buildCLIApp(&action, &opts).Execute(args); err != nil {
				t.Fatal(err)
			}
			if action != "library" || opts.ServerSubcmd != "download" || opts.Podcast != target || !opts.CountGiven || opts.Count != 3 {
				t.Fatalf("incorrect parsing for %v: action=%s opts=%+v", args, action, opts)
			}
		}
	}
	for _, count := range []string{"0", "-2"} {
		var action string
		var opts CLIOptions
		if err := buildCLIApp(&action, &opts).Execute([]string{"library", "download", "--count=" + count}); err == nil {
			t.Fatalf("accepted invalid count %s", count)
		}
	}
}

type serverDownloadTestBackend struct {
	backend.Backend
	item     backend.Podcast
	feed     []backend.FeedEpisode
	selected []backend.FeedEpisode
	resets   int
	waits    int
	queueErr error
	waitErr  error
}

func (b *serverDownloadTestBackend) Name() string { return "standalone" }
func (b *serverDownloadTestBackend) Podcasts() ([]backend.Podcast, error) {
	return []backend.Podcast{b.item}, nil
}
func (b *serverDownloadTestBackend) GetPodcast(string) (*backend.Podcast, error) {
	return &b.item, nil
}
func (b *serverDownloadTestBackend) PodcastFeedEpisodes(string) ([]backend.FeedEpisode, error) {
	return b.feed, nil
}
func (b *serverDownloadTestBackend) ActiveDownloads(string) ([]backend.ActiveDownload, error) {
	return nil, nil
}
func (b *serverDownloadTestBackend) ResetPodcastDateCheck(string, string) error {
	b.resets++
	return nil
}
func (b *serverDownloadTestBackend) DownloadEpisodes(_ string, episodes []backend.FeedEpisode) error {
	b.selected = episodes
	return b.queueErr
}
func (b *serverDownloadTestBackend) WaitForActiveDownloads([]backend.Podcast, time.Duration) error {
	b.waits++
	return b.waitErr
}

func newServerDownloadTestBackend(t *testing.T) *serverDownloadTestBackend {
	t.Helper()
	b := &serverDownloadTestBackend{}
	b.item.ID = "123"
	b.item.Path = t.TempDir()
	b.item.Media.Metadata.Title = "Download Test"
	b.item.Media.Metadata.FeedURL = "https://example.invalid/feed"
	for i, title := range []string{"Old", "Middle", "Latest"} {
		b.feed = append(b.feed, backend.FeedEpisode{Title: title, GUID: title, PublishedAt: int64(i + 1), EnclosureURL: "https://example.invalid/" + title})
	}
	return b
}

func TestServerDownloadDryRunAndNoWait(t *testing.T) {
	t.Parallel()
	for _, dryRun := range []bool{true, false} {
		b := newServerDownloadTestBackend(t)
		marker := filepath.Join(t.TempDir(), "processed")
		cfg := Config{PostProcessors: []string{"touch " + marker}}
		var action string
		var opts CLIOptions
		if err := buildCLIApp(&action, &opts).Execute([]string{"library", "download", "123", "-k", "1", "--no-wait", "--quiet"}); err != nil {
			t.Fatal(err)
		}
		opts.DryRun = dryRun
		if err := runServerDownloads(b, cfg, opts); err != nil {
			t.Fatal(err)
		}
		if b.waits != 0 {
			t.Fatalf("unexpected wait: %d", b.waits)
		}
		if dryRun && (b.resets != 0 || len(b.selected) != 0) {
			t.Fatal("dry run mutated backend")
		}
		if !dryRun && (len(b.selected) != 1 || b.selected[0].Title != "Latest") {
			t.Fatalf("wrong selected episodes: %+v", b.selected)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("postprocessing ran before downloads completed")
		}
	}
}

func TestServerDownloadFailureStopsPostprocessing(t *testing.T) {
	t.Parallel()
	for _, failQueue := range []bool{true, false} {
		b := newServerDownloadTestBackend(t)
		failure := errors.New("backend unavailable")
		if failQueue {
			b.queueErr = failure
		} else {
			b.waitErr = failure
		}
		marker := filepath.Join(t.TempDir(), "processed")
		cfg := Config{PostProcessors: []string{"touch " + marker}}
		var action string
		var opts CLIOptions
		if err := buildCLIApp(&action, &opts).Execute([]string{"library", "download", "-k", "1", "--quiet"}); err != nil {
			t.Fatal(err)
		}
		if err := runServerDownloads(b, cfg, opts); !errors.Is(err, failure) {
			t.Fatalf("expected backend error, got %v", err)
		}
		if failQueue && b.waits != 0 || !failQueue && b.waits != 1 {
			t.Fatalf("unexpected waits: %d", b.waits)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("postprocessing ran after failure")
		}
	}
}

func TestServerDownloadCountSelectsNewestOrOldest(t *testing.T) {
	t.Parallel()
	for _, oldest := range []bool{false, true} {
		b := newServerDownloadTestBackend(t)
		b.item.Media.Episodes = []backend.Episode{{Title: "Old", GUID: "Old", AudioFile: &backend.PodcastAudioFile{Duration: 1}}}
		var action string
		var opts CLIOptions
		args := []string{"library", "download", "123", "-k", "1", "--no-wait", "--quiet"}
		if oldest {
			args = append(args, "--oldest")
		}
		if err := buildCLIApp(&action, &opts).Execute(args); err != nil {
			t.Fatal(err)
		}
		if err := runServerDownloads(b, Config{}, opts); err != nil {
			t.Fatal(err)
		}
		want := "Latest"
		if oldest {
			want = "Middle"
		}
		if len(b.selected) != 1 || b.selected[0].Title != want {
			t.Fatalf("oldest=%v: selected %+v, want %s", oldest, b.selected, want)
		}
	}
}

func TestServerDownloadAmbiguousTarget(t *testing.T) {
	t.Parallel()
	podcasts := []backend.Podcast{
		{ID: "101", Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: "Daily News"}}},
		{ID: "102", Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: "Daily Tech"}}},
	}
	opts := CLIOptions{Podcast: "daily", ProcOptions: ProcOptions{Quiet: true}}
	_, err := filterServerTargets(podcasts, opts)
	if err == nil || !errors.Is(err, podcast.ErrAmbiguousPodcast) {
		t.Fatalf("expected ErrAmbiguousPodcast, got %v", err)
	}

	opts.Podcast = "news"
	targets, err := filterServerTargets(podcasts, opts)
	if err != nil || len(targets) != 1 || targets[0].ID != "101" {
		t.Fatalf("expected 1 unique target for 'news', got targets=%+v err=%v", targets, err)
	}
}
