package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/util"
)

// QueueFileName is the per-podcast ad-removal queue, stored in the podcast's
// own directory.
const QueueFileName = "queue.json"

// queueUpdateMu serialises queue writes within this process; the file lock
// below serialises them against other processes. The CLI and the TUI each used
// to keep their own copy of this function and their own mutex, so neither
// excluded the other.
var queueUpdateMu util.SyncMutex

// UpdateQueue rewrites a podcast directory's queue.json under a file lock.
func UpdateQueue(dir string, mutate func([]string) []string) error {
	queueUpdateMu.Lock()
	defer queueUpdateMu.Unlock()

	path := filepath.Join(dir, QueueFileName)
	lock, err := util.AcquireFileLockWithTimeout(path, 5*time.Second)
	if err != nil || lock == nil {
		return fmt.Errorf("queue is locked: %w", err)
	}
	defer lock.Release()

	entries, err := ReadQueue(dir)
	if err != nil {
		return err
	}
	entries = mutate(entries)
	if entries == nil {
		entries = []string{}
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(path, append(data, '\n'), 0644)
}

// AddToQueue appends an episode to a podcast's queue, reporting whether it was
// not already there.
func AddToQueue(podDir, filename string) bool {
	added, _ := AddToQueueChecked(podDir, filename)
	return added
}

func AddToQueueChecked(podDir, filename string) (bool, error) {
	added := false
	err := UpdateQueue(podDir, func(entries []string) []string {
		for _, e := range entries {
			if strings.EqualFold(e, filename) {
				return entries
			}
		}
		added = true
		return append(entries, filename)
	})
	return added && err == nil, err
}

// RemoveFromQueue drops an episode from a podcast's queue, reporting whether it
// was present.
func RemoveFromQueue(podDir, filename string) bool {
	found, _ := removeFromQueueChecked(podDir, filename)
	return found
}

func removeFromQueueChecked(podDir, filename string) (bool, error) {
	found := false
	err := UpdateQueue(podDir, func(entries []string) []string {
		var filtered []string
		for _, e := range entries {
			if strings.EqualFold(e, filename) {
				found = true
			} else {
				filtered = append(filtered, e)
			}
		}
		if found {
			return filtered
		}
		return entries
	})
	return found && err == nil, err
}

func ReadQueue(dir string) ([]string, error) {
	path := filepath.Join(dir, QueueFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read queue %s: %w", path, err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse queue %s: %w", path, err)
	}
	return entries, nil
}

// QueuedEpisodes reports the episode filenames queued in a podcast directory.
func QueuedEpisodes(podDir string) []string {
	data, err := os.ReadFile(filepath.Join(podDir, QueueFileName))
	if err != nil {
		return nil
	}
	var filenames []string
	if err := json.Unmarshal(data, &filenames); err != nil {
		return nil
	}
	return filenames
}
