package cli

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

func TestPodcastPriorityPersistsAndReordersQueue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a, ap := createTestPodcastWithEpisodes(t, root, "Alpha", []string{"episode"})
	b, bp := createTestPodcastWithEpisodes(t, root, "Beta", []string{"episode"})
	if err := handleQueueAdd(io.Discard, podcast.Open(podcast.Config{PodcastsDir: root}, nil, nil), []string{"all"}); err != nil {
		t.Fatal(err)
	}
	if err := handleQueuePriority(io.Discard, root, []string{"Beta", "8"}); err != nil {
		t.Fatal(err)
	}
	items, err := podcast.Open(podcast.Config{PodcastsDir: root}, nil, nil).QueueItems("")
	if err != nil {
		t.Fatal(err)
	}
	podcast.SortQueueItems(items)
	if items[0].AudioPath != bp[0] || items[0].Priority != 8 || items[1].Priority != 0 {
		t.Fatalf("order=%+v", items)
	}
	cfg := config.LoadPodcastConfig(b, config.DefaultPodcastConfig(nil))
	if cfg.Priority != 8 {
		t.Fatal("priority was not persisted")
	}
	st, _ := episode.LoadEpisodeStatus(episode.StatusPathFor(bp[0]))
	if st.Priority != 0 {
		t.Fatal("inherited priority became an episode override")
	}
	if err := handleQueuePriority(io.Discard, root, []string{"Beta", "0"}); err != nil {
		t.Fatal(err)
	}
	if podcast.EpisodePriority(b, bp[0]) != 0 {
		t.Fatal("lowering podcast priority ineffective")
	}
	before := queueTree(t, root)
	for _, value := range []string{"-1", "11", "invalid"} {
		if err := handleQueuePriority(io.Discard, root, []string{"Alpha", value}); err == nil {
			t.Errorf("accepted %s", value)
		}
	}
	id := podcast.EpisodeShortIDReadOnly(a, podcast.GeneratePodcastShortID("Alpha"), ap[0])
	if err := handleQueuePriority(io.Discard, root, []string{id, "8"}); err == nil {
		t.Fatal("accepted permanent episode priority")
	}
	if !reflect.DeepEqual(before, queueTree(t, root)) {
		t.Fatal("invalid priority command changed files")
	}
}

func TestRmAdsEpisodeQueuesUrgentlyAndRetainsOnFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, paths := createTestPodcastWithEpisodes(t, root, "Show", []string{"other", "requested"})
	if _, err := episode.AddToQueueChecked(dir, filepath.Base(paths[0])); err != nil {
		t.Fatal(err)
	}
	id := podcast.EpisodeShortIDReadOnly(dir, podcast.GeneratePodcastShortID("Show"), paths[1])
	cfg := Config{PodcastsDir: root}
	cli := CLIOptions{Args: []string{id}, ProcOptions: ProcOptions{DryRun: true, Quiet: true}}
	before := queueTree(t, root)
	if err := runRmAdsCommand(cfg, cli); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, queueTree(t, root)) {
		t.Fatal("dry run changed files")
	}
	lock, err := util.AcquireFileLock(paths[1])
	if err != nil || lock == nil {
		t.Fatalf("lock: %v", err)
	}
	defer lock.Release()
	cli.DryRun = false
	if err := runRmAdsCommand(cfg, cli); err == nil {
		t.Fatal("locked requested episode reported success")
	}
	entries, err := episode.ReadQueue(dir)
	if err != nil || !reflect.DeepEqual(entries, []string{"requested.mp3", "other.mp3"}) {
		t.Fatalf("queue=%v error=%v", entries, err)
	}
	if podcast.EpisodePriority(dir, paths[1]) != 10 {
		t.Fatal("request missing priority 10")
	}
	other, _ := episode.LoadEpisodeStatus(episode.StatusPathFor(paths[0]))
	if other.Priority != 0 || other.Status != StateDownloaded {
		t.Fatal("processed unrelated episode")
	}
	if _, err := episode.RemoveQueuedAudio(dir, paths[1]); err != nil {
		t.Fatal(err)
	}
	if podcast.EpisodePriority(dir, paths[1]) != 0 {
		t.Fatal("temporary boost survived removal")
	}
}

func TestQueuePriorityStringMatching(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	createTestPodcastWithEpisodes(t, root, "History Show", []string{"episode1"})
	createTestPodcastWithEpisodes(t, root, "Science Show", []string{"episode2"})

	if err := handleQueuePriority(io.Discard, root, []string{"hist", "7"}); err != nil {
		t.Fatalf("expected unique substring match: %v", err)
	}

	err := handleQueuePriority(io.Discard, root, []string{"show", "5"})
	if err == nil || !errors.Is(err, podcast.ErrAmbiguousPodcast) {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}
