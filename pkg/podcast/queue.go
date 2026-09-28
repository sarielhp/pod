package podcast

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/episode"
	"pod/pkg/util"
)

type DownloadQueueItem struct {
	ID           string               `json:"id"`
	PodcastTitle string               `json:"podcast_title"`
	PodcastDir   string               `json:"podcast_dir,omitempty"`
	PodcastID    string               `json:"podcast_id,omitempty"`
	EpisodeTitle string               `json:"episode_title"`
	GUID         string               `json:"guid,omitempty"`
	EnclosureURL string               `json:"enclosure_url,omitempty"`
	PubDate      string               `json:"pub_date,omitempty"`
	PublishedAt  int64                `json:"published_at,omitempty"`
	DurationSec  float64              `json:"duration_sec,omitempty"`
	Status       string               `json:"status"`
	OwnerPID     int                  `json:"owner_pid,omitempty"`
	Error        string               `json:"error,omitempty"`
	AddedAt      time.Time            `json:"added_at"`
	EpisodeObj   *backend.FeedEpisode `json:"episode_obj,omitempty"`
}

type DownloadQueuePersist struct {
	Items     []DownloadQueueItem `json:"items"`
	UpdatedAt time.Time           `json:"updated_at,omitempty"`
}

type DownloadQueue struct {
	mu            util.Mutex
	filePath      string
	reconcileOnce util.Once
	testHookMu    util.Mutex
	testHook      func(item DownloadQueueItem) error

	workerMu       util.Mutex
	workerRunning  bool
	workerPending  bool
	workerExitHook func()
}

func NewDownloadQueue(path string) *DownloadQueue {
	if path == "" {
		path = defaultDownloadQueuePath()
	}
	return &DownloadQueue{
		filePath: path,
	}
}

func defaultDownloadQueuePath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "download_queue.json")
		}
		configHome = filepath.Join(home, ".config")
	}
	podDir := filepath.Join(configHome, "pod")
	if _, err := os.Stat(filepath.Join(podDir, "download_queue.json")); err == nil {
		return filepath.Join(podDir, "download_queue.json")
	}
	legacyDir := filepath.Join(configHome, "abs")
	if _, err := os.Stat(filepath.Join(legacyDir, "download_queue.json")); err == nil {
		return filepath.Join(legacyDir, "download_queue.json")
	}
	_ = os.MkdirAll(podDir, 0755)
	return filepath.Join(podDir, "download_queue.json")
}

var globalDownloadQueueOnce util.Once
var globalDownloadQueue *DownloadQueue

// defaultDownloadQueue is the process-wide download queue for the default
// queue file. It is unexported: callers outside this package reach it through
// Library.Queue, so library state is owned rather than ambient.
func defaultDownloadQueue() *DownloadQueue {
	globalDownloadQueueOnce.Do(func() {
		globalDownloadQueue = NewDownloadQueue("")
	})
	return globalDownloadQueue
}

func (q *DownloadQueue) SetFilePath(path string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.filePath = path
}

func (q *DownloadQueue) SetTestHook(hook func(item DownloadQueueItem) error) {
	q.testHookMu.Lock()
	defer q.testHookMu.Unlock()
	q.testHook = hook
}

func (q *DownloadQueue) SetWorkerExitHookForTest(hook func()) {
	q.testHookMu.Lock()
	defer q.testHookMu.Unlock()
	q.workerExitHook = hook
}

func (q *DownloadQueue) getWorkerExitHook() func() {
	q.testHookMu.Lock()
	defer q.testHookMu.Unlock()
	return q.workerExitHook
}

func (q *DownloadQueue) getTestHook() func(item DownloadQueueItem) error {
	q.testHookMu.Lock()
	defer q.testHookMu.Unlock()
	return q.testHook
}

func (q *DownloadQueue) ReconcileStaleDownloadingItems(persist *DownloadQueuePersist) {
	q.reconcileOnce.Do(func() {
		dirty := false
		for i := range persist.Items {
			if persist.Items[i].Status == "downloading" {
				if persist.Items[i].OwnerPID <= 0 || !util.IsProcessAlive(persist.Items[i].OwnerPID) {
					persist.Items[i].Status = "queued"
					persist.Items[i].OwnerPID = 0
					dirty = true
				}
			}
		}
		if dirty {
			_ = q.Save(persist)
		}
	})
}

func (q *DownloadQueue) Load() *DownloadQueuePersist {
	data, err := os.ReadFile(q.filePath)
	if err != nil {
		return &DownloadQueuePersist{Items: []DownloadQueueItem{}}
	}
	var p DownloadQueuePersist
	if err := json.Unmarshal(data, &p); err != nil {
		return &DownloadQueuePersist{Items: []DownloadQueueItem{}}
	}
	if p.Items == nil {
		p.Items = []DownloadQueueItem{}
	}
	q.ReconcileStaleDownloadingItems(&p)
	return &p
}

func (q *DownloadQueue) Save(persist *DownloadQueuePersist) error {
	_ = os.MkdirAll(filepath.Dir(q.filePath), 0755)
	persist.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(persist, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(q.filePath, append(data, '\n'), 0644)
}

func matchEpisodeDeduplication(guid1, enc1, title1, guid2, enc2, title2 string) bool {
	if guid1 != "" && guid2 != "" && strings.EqualFold(strings.TrimSpace(guid1), strings.TrimSpace(guid2)) {
		return true
	}
	if enc1 != "" && enc2 != "" && strings.EqualFold(strings.TrimSpace(enc1), strings.TrimSpace(enc2)) {
		return true
	}
	t1 := episode.NormalizeEpisodeTitle(title1)
	t2 := episode.NormalizeEpisodeTitle(title2)
	if t1 != "" && t2 != "" && t1 == t2 {
		return true
	}
	return false
}

func (q *DownloadQueue) IsEpisodeInQueue(guid, encURL, title string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	persist := q.Load()
	for _, existing := range persist.Items {
		if existing.Status == "completed" {
			continue
		}
		if matchEpisodeDeduplication(guid, encURL, title, existing.GUID, existing.EnclosureURL, existing.EpisodeTitle) {
			return true
		}
	}
	return false
}

var queueLockTimeout = 5 * time.Second

func queueLockTimeoutForTest(d time.Duration) time.Duration {
	prev := queueLockTimeout
	queueLockTimeout = d
	return prev
}

// withQueueFile runs fn over the queue's on-disk state under both the
// in-process mutex and the cross-process file lock, then saves whatever fn
// left in persist when it asks to. Every read-modify-write goes through here:
// the TUI and a CLI command are separate processes over one file, and a
// mutex alone let one of them overwrite the other's update.
func (q *DownloadQueue) withQueueFile(fn func(persist *DownloadQueuePersist) (save bool, err error)) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := util.AcquireFileLockWithTimeout(q.filePath, queueLockTimeout)
	if err != nil {
		return err
	}
	if lock == nil {
		return fmt.Errorf("download queue %s is locked by another process", q.filePath)
	}
	defer lock.Release()

	persist := q.Load()
	save, err := fn(persist)
	if err != nil || !save {
		return err
	}
	return q.Save(persist)
}

func (q *DownloadQueue) Enqueue(item DownloadQueueItem) (bool, string) {
	ok, status := false, "save_error"
	err := q.withQueueFile(func(persist *DownloadQueuePersist) (bool, error) {
		ok, status = enqueueInto(persist, item)
		return ok, nil
	})
	if err != nil {
		return false, "save_error"
	}
	return ok, status
}

func enqueueInto(persist *DownloadQueuePersist, item DownloadQueueItem) (bool, string) {
	for _, existing := range persist.Items {
		if existing.Status == "completed" || existing.Status == "failed" {
			continue
		}
		if matchEpisodeDeduplication(item.GUID, item.EnclosureURL, item.EpisodeTitle, existing.GUID, existing.EnclosureURL, existing.EpisodeTitle) {
			return false, "already_queued"
		}
	}

	for i, existing := range persist.Items {
		if existing.Status == "failed" && matchEpisodeDeduplication(item.GUID, item.EnclosureURL, item.EpisodeTitle, existing.GUID, existing.EnclosureURL, existing.EpisodeTitle) {
			persist.Items[i].Status = "queued"
			persist.Items[i].Error = ""
			persist.Items[i].AddedAt = time.Now().UTC()
			return true, "queued"
		}
	}

	if item.ID == "" {
		item.ID = fmt.Sprintf("dl-%d", time.Now().UnixNano())
	}
	item.Status = "queued"
	item.AddedAt = time.Now().UTC()

	persist.Items = append(persist.Items, item)
	return true, "queued"
}

func (q *DownloadQueue) Remove(id string) bool {
	found := false
	err := q.withQueueFile(func(persist *DownloadQueuePersist) (bool, error) {
		updated := persist.Items[:0:0]
		for _, item := range persist.Items {
			if item.ID == id {
				found = true
				continue
			}
			updated = append(updated, item)
		}
		persist.Items = updated
		return found, nil
	})
	return found && err == nil
}

func (q *DownloadQueue) Clear() {
	_ = q.withQueueFile(func(persist *DownloadQueuePersist) (bool, error) {
		persist.Items = []DownloadQueueItem{}
		return true, nil
	})
}

func (q *DownloadQueue) Items() []DownloadQueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	persist := q.Load()
	return persist.Items
}

func (q *DownloadQueue) Claim() (DownloadQueueItem, bool, error) {
	var claimed DownloadQueueItem
	found := false
	err := q.withQueueFile(func(persist *DownloadQueuePersist) (bool, error) {
		for i := range persist.Items {
			if persist.Items[i].Status != "queued" {
				continue
			}
			persist.Items[i].Status = "downloading"
			persist.Items[i].OwnerPID = os.Getpid()
			claimed, found = persist.Items[i], true
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return DownloadQueueItem{}, false, err
	}
	return claimed, found, nil
}

func (q *DownloadQueue) Finalize(itemID string, dlErr error) error {
	return q.withQueueFile(func(persist *DownloadQueuePersist) (bool, error) {
		return finalizeItem(persist, itemID, dlErr), nil
	})
}

func finalizeItem(persist *DownloadQueuePersist, itemID string, dlErr error) bool {
	for i := range persist.Items {
		if persist.Items[i].ID == itemID {
			if dlErr != nil {
				persist.Items[i].Status = "failed"
				persist.Items[i].Error = dlErr.Error()
			} else {
				persist.Items[i].Status = "completed"
			}
			persist.Items[i].OwnerPID = 0
			return true
		}
	}
	return false
}

func isNilBackend(b backend.Backend) bool {
	if b == nil {
		return true
	}
	v := reflect.ValueOf(b)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func (q *DownloadQueue) ProcessNext(client backend.Backend) (bool, error) {
	item, claimed, err := q.Claim()
	if err != nil || !claimed {
		return false, err
	}

	podcastID := item.PodcastID
	if podcastID == "" && !isNilBackend(client) {
		if pods, err := client.Podcasts(); err == nil {
			for _, p := range pods {
				if strings.EqualFold(strings.TrimSpace(p.Media.Metadata.Title), strings.TrimSpace(item.PodcastTitle)) {
					podcastID = p.ID
					break
				}
			}
		}
	}

	var dlErr error
	if hook := q.getTestHook(); hook != nil {
		dlErr = hook(item)
	} else if isNilBackend(client) {
		dlErr = fmt.Errorf("download client is unavailable")
	} else if podcastID == "" {
		dlErr = fmt.Errorf("podcast ID not found for '%s'", item.PodcastTitle)
	} else {
		feedEp := backend.FeedEpisode{
			Title:           item.EpisodeTitle,
			GUID:            item.GUID,
			EnclosureURL:    item.EnclosureURL,
			PubDate:         item.PubDate,
			PublishedAt:     item.PublishedAt,
			DurationSeconds: item.DurationSec,
		}
		if item.EnclosureURL != "" {
			feedEp.Enclosure = &backend.FeedEnclosure{URL: item.EnclosureURL}
		}
		if item.EpisodeObj != nil {
			feedEp = *item.EpisodeObj
		}
		dlErr = client.DownloadEpisodes(podcastID, []backend.FeedEpisode{feedEp})
	}

	fErr := q.Finalize(item.ID, dlErr)
	if fErr != nil {
		if dlErr != nil {
			return true, fmt.Errorf("download error: %v, finalize error: %w", dlErr, fErr)
		}
		return true, fmt.Errorf("finalize error: %w", fErr)
	}
	return true, dlErr
}

func (q *DownloadQueue) TriggerWorker(client backend.Backend) {
	q.workerMu.Lock()
	if q.workerRunning {
		q.workerPending = true
		q.workerMu.Unlock()
		return
	}
	q.workerRunning = true
	q.workerMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in download worker: %v\n%s", r, debug.Stack())
				q.workerMu.Lock()
				q.workerRunning = false
				q.workerPending = false
				q.workerMu.Unlock()
			}
		}()
		q.runWorkerLoop(client)
		if hook := q.getWorkerExitHook(); hook != nil {
			hook()
		}
	}()
}

func (q *DownloadQueue) runWorkerLoop(client backend.Backend) {
	for {
		processed, err := q.ProcessNext(client)
		if err != nil {
			log.Printf("download queue: processing error: %v", err)
		}
		if processed {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		q.workerMu.Lock()
		if q.workerPending {
			q.workerPending = false
			q.workerMu.Unlock()
			continue
		}
		q.workerRunning = false
		q.workerMu.Unlock()
		return
	}
}

func (q *DownloadQueue) WaitWorkerForTest() {
	for {
		q.workerMu.Lock()
		running := q.workerRunning
		q.workerMu.Unlock()
		if !running {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}
