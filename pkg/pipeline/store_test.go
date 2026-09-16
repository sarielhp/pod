package pipeline

import (
	"context"
	"path/filepath"
	"testing"

	"pod/pkg/types"
)

func TestFileEpisodeStateStoreUpdateAndGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "episode.mp3")

	store := newFileEpisodeStateStore()
	err := store.Update(context.Background(), audioPath, func(st *types.EpisodeStatusFile) {
		st.Status = types.StateNeedsAdR
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	st, err := store.Get(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if st.Status != types.StateNeedsAdR {
		t.Errorf("got status %v, want %v", st.Status, types.StateNeedsAdR)
	}
}
