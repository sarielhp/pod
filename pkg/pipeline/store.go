package pipeline

import (
	"context"

	"pod/pkg/types"
)

// EpisodeStateStore defines process-safe retrieval and mutation of episode status metadata.
type EpisodeStateStore interface {
	Get(ctx context.Context, audioPath string) (*types.EpisodeStatusFile, error)
	Update(ctx context.Context, audioPath string, mutate func(*types.EpisodeStatusFile)) error
	IsCompleted(ctx context.Context, audioPath string) bool
	IsClean(ctx context.Context, audioPath string) bool
}

// FileEpisodeStateStore implements EpisodeStateStore using JSON status files with file and mutex locking.
type FileEpisodeStateStore struct{}

func newFileEpisodeStateStore() *FileEpisodeStateStore {
	return &FileEpisodeStateStore{}
}

func (s *FileEpisodeStateStore) Get(ctx context.Context, audioPath string) (*types.EpisodeStatusFile, error) {
	statPath := StatusPathFor(audioPath)
	st, err := LoadEpisodeStatus(statPath)
	if err != nil {
		return GetOrCreateEpisodeStatus(audioPath), nil
	}
	return st, nil
}

func (s *FileEpisodeStateStore) Update(ctx context.Context, audioPath string, mutate func(*types.EpisodeStatusFile)) error {
	return UpdateEpisodeStatus(audioPath, mutate)
}

func (s *FileEpisodeStateStore) IsCompleted(ctx context.Context, audioPath string) bool {
	return IsEpisodeCompleted(audioPath)
}

func (s *FileEpisodeStateStore) IsClean(ctx context.Context, audioPath string) bool {
	return IsEpisodeClean(audioPath)
}

var DefaultStateStore EpisodeStateStore = newFileEpisodeStateStore()
