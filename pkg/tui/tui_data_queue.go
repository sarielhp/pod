package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/util"
)

var queueUpdateMu util.SyncMutex

func loadAllQueues(pods []tuiPodcast) map[string][]string {
	q := make(map[string][]string)
	for _, pod := range pods {
		data, err := os.ReadFile(filepath.Join(pod.dir, "queue.json"))
		if err != nil {
			q[pod.dir] = nil
			continue
		}
		var entries []string
		if err := json.Unmarshal(data, &entries); err != nil {
			q[pod.dir] = nil
			continue
		}

		adFreeMap := make(map[string]bool)
		for _, ep := range pod.episodes {
			if ep.hasAdsRemoved {
				adFreeMap[ep.filename] = true
			}
		}

		var filtered []string
		needsResave := false
		for _, e := range entries {
			if !strings.HasSuffix(strings.ToLower(e), ".mp3") {
				needsResave = true
				continue
			}
			if adFreeMap[e] {
				needsResave = true
				continue
			}
			filtered = append(filtered, e)
		}
		q[pod.dir] = filtered
		if needsResave {
			_ = saveQueue(pod.dir, filtered)
		}
	}
	return q
}

func updateQueue(dir string, mutate func([]string) []string) error {
	queueUpdateMu.Lock()
	defer queueUpdateMu.Unlock()

	path := filepath.Join(dir, "queue.json")
	lock, err := util.AcquireFileLockWithTimeout(path, 5*time.Second)
	if err != nil {
		return fmt.Errorf("lock queue: %w", err)
	}
	if lock == nil {
		return fmt.Errorf("queue %s is locked by another pod instance", path)
	}
	defer lock.Release()

	var entries []string
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			return err
		}
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

func saveQueue(dir string, entries []string) error {
	return updateQueue(dir, func([]string) []string {
		return entries
	})
}

type AdQueueItem struct {
	PodcastDir    string
	PodcastName   string
	Filename      string
	Title         string
	HasAdsRemoved bool
	PublishedAt   int64
	Duration      float64
}

func getAllAdQueueItems(pods []tuiPodcast, q map[string][]string) []AdQueueItem {
	var list []AdQueueItem
	for _, pod := range pods {
		filenames := q[pod.dir]
		for _, fn := range filenames {
			item := AdQueueItem{
				PodcastDir:  pod.dir,
				PodcastName: pod.name,
				Filename:    fn,
				Title:       fn,
			}
			hasAdsRemoved := false
			for _, ep := range pod.episodes {
				if ep.filename == fn {
					item.Title = ep.displayTitle()
					item.HasAdsRemoved = ep.hasAdsRemoved
					item.PublishedAt = ep.publishedAt
					item.Duration = ep.duration
					hasAdsRemoved = ep.hasAdsRemoved
					break
				}
			}
			if !hasAdsRemoved {
				list = append(list, item)
			}
		}
	}
	return list
}
