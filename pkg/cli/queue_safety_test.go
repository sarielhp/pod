package cli

import (
	"io"
	"os"
	"path/filepath"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"reflect"
	"testing"
	"time"
)

func queueTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[path] = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestQueueReadOnlyAndEligibility(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "Show")
	if err := os.MkdirAll(filepath.Join(dir, ".work"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"episode.mp3", "episode.precut.mp3", ".work/temp.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "queue.json"), []byte(`["episode.mp3"]`), 0644); err != nil {
		t.Fatal(err)
	}
	before := queueTree(t, root)
	if err := handleQueueList(podcast.Open(podcast.Config{PodcastsDir: root}, nil, nil), "Show", CLIOptions{ProcOptions: ProcOptions{Quiet: true}}); err != nil {
		t.Fatal(err)
	}
	if err := handleQueueRun(podcast.Open(podcast.Config{PodcastsDir: root}, nil, nil), Config{PodcastsDir: root}, CLIOptions{ProcOptions: ProcOptions{DryRun: true}}, "episode.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := handleQueueToday(root, CLIOptions{ProcOptions: ProcOptions{DryRun: true}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, queueTree(t, root)) {
		t.Fatal("read-only command changed metadata")
	}
	if err := handleQueueAdd(io.Discard, podcast.Open(podcast.Config{PodcastsDir: root}, nil, nil), []string{"all"}); err != nil {
		t.Fatal(err)
	}
	entries, err := episode.ReadQueue(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("queue=%v error=%v", entries, err)
	}
	if err := handleQueueAdd(io.Discard, podcast.Open(podcast.Config{PodcastsDir: root}, nil, nil), []string{"episode.precut.mp3"}); err == nil {
		t.Fatal("direct precut target accepted")
	}
}

func TestQueueCommandsSurfaceCorruptQueue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, _ := createTestPodcastWithEpisodes(t, root, "Show", []string{"episode"})
	path := filepath.Join(dir, "queue.json")
	if err := os.WriteFile(path, []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"list", "add", "clear", "run"} {
		if err := runQueueCommand(Config{PodcastsDir: root}, CLIOptions{QueueSubcmd: action}); err == nil {
			t.Errorf("%s hid corrupt queue", action)
		}
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("corrupt queue overwritten")
	}
}
