package podcast

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/episode"
)

func queueFixture(t *testing.T) (root, podDir string) {
	t.Helper()
	root = t.TempDir()
	podDir = filepath.Join(root, "Show")
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ep1.mp3", "ep2.mp3"} {
		if err := os.WriteFile(filepath.Join(podDir, name), []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, podDir
}

func TestEnqueuePodcastIsIdempotent(t *testing.T) {
	t.Parallel()
	_, podDir := queueFixture(t)

	n, err := EnqueuePodcast(podDir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("first enqueue added %d, want 2", n)
	}

	n, err = EnqueuePodcast(podDir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("re-enqueueing the same episodes added %d, want 0", n)
	}

	entries, err := episode.ReadQueue(podDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("queue holds %d entries, want 2: %v", len(entries), entries)
	}
}

func TestClearPodcastQueueEmptiesIt(t *testing.T) {
	t.Parallel()
	_, podDir := queueFixture(t)
	if _, err := EnqueuePodcast(podDir); err != nil {
		t.Fatal(err)
	}
	if err := ClearPodcastQueue(podDir); err != nil {
		t.Fatal(err)
	}
	entries, err := episode.ReadQueue(podDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("queue still holds %v", entries)
	}
}

// The reported total counts podcasts that had a queue file, including ones
// whose queue was already empty. Changing that would quietly change what the
// number on screen means.
func TestClearAllQueuesCountsQueueFilesNotEntries(t *testing.T) {
	t.Parallel()
	root, podDir := queueFixture(t)
	emptyDir := filepath.Join(root, "Empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyDir, "ep.mp3"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnqueuePodcast(podDir); err != nil {
		t.Fatal(err)
	}
	if err := ClearPodcastQueue(emptyDir); err != nil {
		t.Fatal(err)
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	cleared, err := lib.ClearAllQueues()
	if err != nil {
		t.Fatal(err)
	}
	if cleared != 2 {
		t.Errorf("cleared %d, want 2 (both podcasts have a queue file)", cleared)
	}
}

func TestQueueItemsAcrossLibraryAndByTarget(t *testing.T) {
	t.Parallel()
	root, podDir := queueFixture(t)
	if _, err := EnqueuePodcast(podDir); err != nil {
		t.Fatal(err)
	}
	lib := Open(Config{PodcastsDir: root}, nil, nil)

	all, err := lib.QueueItems("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("library queue holds %d items, want 2", len(all))
	}
	for _, it := range all {
		if it.PodcastDir != podDir || it.AudioPath == "" {
			t.Errorf("item not located: %+v", it)
		}
	}

	byPodcast, err := lib.QueueItems("Show")
	if err != nil {
		t.Fatal(err)
	}
	if len(byPodcast) != 2 {
		t.Errorf("targeting the podcast gave %d items, want 2", len(byPodcast))
	}
}

func TestQueueItemsRejectsAnEpisodeThatIsNotQueued(t *testing.T) {
	t.Parallel()
	root, podDir := queueFixture(t)
	lib := Open(Config{PodcastsDir: root}, nil, nil)
	if _, err := lib.QueueItems(filepath.Join(podDir, "ep1.mp3")); err == nil {
		t.Error("expected an error for an episode that is not in the queue")
	}
}

func TestQueueFilenameIsRelativeInsideThePodcast(t *testing.T) {
	t.Parallel()
	if got := QueueFilename("/lib/Show", "/lib/Show/ep1.mp3"); got != "ep1.mp3" {
		t.Errorf("got %q, want %q", got, "ep1.mp3")
	}
	if got := QueueFilename("/lib/Show", "/lib/Show/sub/ep1.mp3"); got != filepath.Join("sub", "ep1.mp3") {
		t.Errorf("nested episode: got %q", got)
	}
	// Outside the podcast directory, only the base name is meaningful.
	if got := QueueFilename("/lib/Show", "/elsewhere/ep1.mp3"); got != "ep1.mp3" {
		t.Errorf("outside the podcast: got %q", got)
	}
}
