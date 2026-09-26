package cli

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/pipeline"
	"pod/pkg/types"
	"pod/pkg/util"
)

func TestQueueRetainsLockedEpisode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	pipeline.AddToQueue(dir, "episode.mp3")
	lock, err := util.AcquireFileLock(path)
	if err != nil || lock == nil {
		t.Fatalf("lock: %v", err)
	}
	defer lock.Release()
	opts := types.ProcOptions{Quiet: true, WhisperEngine: "local"}
	if err := ProcessQueuedTarget(dir, path, opts, types.Config{}); err == nil {
		t.Fatal("skipped processing reported success")
	}
	if got := pipeline.QueuedEpisodes(dir); len(got) != 1 || got[0] != "episode.mp3" {
		t.Fatalf("queue = %v, want episode retained", got)
	}
}
