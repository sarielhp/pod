package podcast

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pod/pkg/util"
)

func TestTwoQueuesOverOneFileLoseNoEnqueues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download_queue.json")
	a, b := NewDownloadQueue(path), NewDownloadQueue(path)
	const perQueue = 40

	var wg sync.WaitGroup
	for i, q := range []*DownloadQueue{a, b} {
		wg.Add(1)
		go func(tag int, q *DownloadQueue) {
			defer wg.Done()
			for n := 0; n < perQueue; n++ {
				id := fmt.Sprintf("q%d-%d", tag, n)
				ok, status := q.Enqueue(DownloadQueueItem{ID: id, EpisodeTitle: id, GUID: id})
				if !ok {
					t.Errorf("enqueue %s: %s", id, status)
				}
			}
		}(i, q)
	}
	wg.Wait()

	got := len(NewDownloadQueue(path).Items())
	if got != 2*perQueue {
		t.Fatalf("%d of %d concurrent enqueues from two queue instances survived; the rest were overwritten", got, 2*perQueue)
	}
}

func TestClaimAndFinalizeFailWhenTheQueueIsLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download_queue.json")
	q := NewDownloadQueue(path)
	if ok, _ := q.Enqueue(DownloadQueueItem{ID: "x", EpisodeTitle: "x"}); !ok {
		t.Fatal("enqueue")
	}
	lock, err := util.AcquireFileLock(path)
	if err != nil || lock == nil {
		t.Fatalf("could not take the queue lock for the test: %v", err)
	}
	defer lock.Release()

	orig := queueLockTimeoutForTest(50 * time.Millisecond)
	defer queueLockTimeoutForTest(orig)

	if _, claimed, err := q.Claim(); err == nil || claimed {
		t.Fatalf("Claim on a locked queue returned claimed=%v err=%v; a busy lock must be an error, not an empty success", claimed, err)
	}
	if err := q.Finalize("x", nil); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("Finalize on a locked queue returned %v", err)
	}
}

func TestWorkerExitDoesNotClobberItsSuccessor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download_queue.json")
	q := NewDownloadQueue(path)

	var active, maxActive int32
	release := make(chan struct{})
	entered := make(chan struct{}, 8)
	q.SetTestHook(func(item DownloadQueueItem) error {
		n := atomic.AddInt32(&active, 1)
		for {
			m := atomic.LoadInt32(&maxActive)
			if n <= m || atomic.CompareAndSwapInt32(&maxActive, m, n) {
				break
			}
		}
		entered <- struct{}{}
		if item.ID != "first" {
			<-release
		}
		atomic.AddInt32(&active, -1)
		return nil
	})

	firstExited := make(chan struct{})
	resumeFirst := make(chan struct{})
	var exits int32
	q.SetWorkerExitHookForTest(func() {
		if atomic.AddInt32(&exits, 1) == 1 {
			close(firstExited)
			<-resumeFirst
		}
	})

	q.Enqueue(DownloadQueueItem{ID: "first", EpisodeTitle: "first"})
	q.TriggerWorker(nil)
	<-entered
	<-firstExited

	q.Enqueue(DownloadQueueItem{ID: "second", EpisodeTitle: "second"})
	q.TriggerWorker(nil)
	<-entered

	close(resumeFirst)
	time.Sleep(20 * time.Millisecond)

	q.Enqueue(DownloadQueueItem{ID: "third", EpisodeTitle: "third"})
	q.TriggerWorker(nil)
	time.Sleep(20 * time.Millisecond)
	close(release)
	q.WaitWorkerForTest()

	if atomic.LoadInt32(&maxActive) > 1 {
		t.Fatalf("%d download workers ran at once after the first worker's exit cleared the flag its successor owned", maxActive)
	}
	for _, it := range q.Items() {
		if it.Status != "completed" {
			t.Fatalf("item %s ended as %s", it.ID, it.Status)
		}
	}
}
