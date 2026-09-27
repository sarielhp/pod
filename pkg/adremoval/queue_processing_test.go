package adremoval

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/episode"
	"pod/pkg/types"
)

func TestQueuedCompletedEpisodeWithoutTranscriptIsNotSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := episode.SaveEpisodeStatus(episode.StatusPathFor(path), &types.EpisodeStatusFile{Status: types.StateDone}); err != nil {
		t.Fatal(err)
	}
	lock, process, stop := checkSkipOrLockAudioFile(path, path, 0, 1, 0, types.ProcOptions{Quiet: true})
	if lock != nil {
		defer lock.Release()
	}
	if !process || stop {
		t.Fatal("stale done status caused missing-transcript episode to be skipped")
	}
}
