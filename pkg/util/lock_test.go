package util

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLockFileExistsOnlyWhileHeld(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "ep.mp3")
	lock, err := AcquireFileLock(target)
	if err != nil || lock == nil {
		t.Fatalf("first acquire: %v %v", lock, err)
	}
	if !FileExists(target + ".lock") {
		t.Error("the lock file should exist while the lock is held")
	}
	second, err := AcquireFileLock(target)
	if err != nil || second != nil {
		t.Errorf("a held lock must not be given out again: %v %v", second, err)
	}
	lock.Release()
	if FileExists(target + ".lock") {
		t.Error("releasing must delete the lock file")
	}
	lock.Release() // releasing twice is harmless
	again, err := AcquireFileLock(target)
	if err != nil || again == nil {
		t.Fatalf("the lock should be free after release: %v %v", again, err)
	}
	again.Release()
}

func TestAcquireWithTimeoutWaitsForARelease(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "status.json")
	holder, _ := AcquireFileLock(target)
	if holder == nil {
		t.Fatal("setup")
	}
	if w, err := AcquireFileLockWithTimeout(target, 50*time.Millisecond); w != nil || err != nil {
		t.Errorf("must give up while the lock stays held: %v %v", w, err)
	}
	go func() {
		time.Sleep(60 * time.Millisecond)
		holder.Release()
	}()
	w, err := AcquireFileLockWithTimeout(target, 3*time.Second)
	if err != nil || w == nil {
		t.Fatalf("should get the lock once it is released: %v %v", w, err)
	}
	w.Release()
}

// TestLockExcludesUnderContention has many goroutines each add to a counter kept
// in a file, protected only by the lock. Deleting the lock file on release must
// not let two of them in at once, or an update is lost.
func TestLockExcludesUnderContention(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "counter")
	counter := filepath.Join(dir, "counter.value")
	if err := os.WriteFile(counter, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	const workers, rounds = 12, 40
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				lock, err := AcquireFileLockWithTimeout(target, 30*time.Second)
				if err != nil || lock == nil {
					t.Errorf("no lock: %v %v", lock, err)
					return
				}
				data, _ := os.ReadFile(counter)
				n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
				_ = os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0o644)
				lock.Release()
			}
		}()
	}
	wg.Wait()
	data, _ := os.ReadFile(counter)
	if got := strings.TrimSpace(string(data)); got != strconv.Itoa(workers*rounds) {
		t.Errorf("lost updates: the counter is %s, want %d", got, workers*rounds)
	}
	if FileExists(target + ".lock") {
		t.Error("no lock file may be left behind once everyone is done")
	}
}

func TestAReplacedLockFileIsNotMistakenForTheHeldOne(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x.lock")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !holdsPath(f, path) {
		t.Fatal("a file is the file at its own path")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if holdsPath(f, path) {
		t.Error("a deleted file is not the file at the path")
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if holdsPath(f, path) {
		t.Error("a new file at the path is not the one we opened")
	}
}

func TestCleanupStaleLocksRemovesOnlyWhatNoOneHolds(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	show := filepath.Join(root, "Show")
	if err := os.MkdirAll(show, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := []string{filepath.Join(show, "a.mp3.lock"), filepath.Join(show, "a.mp3.json.lock"), filepath.Join(root, "top.lock")}
	for _, p := range stale {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	notALock := filepath.Join(show, "a.transcript.json")
	withData := filepath.Join(show, "data.lock")
	for p, body := range map[string]string{notALock: "{}", withData: "keep me"} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	held, err := AcquireFileLock(filepath.Join(show, "busy.mp3"))
	if err != nil || held == nil {
		t.Fatal("setup")
	}
	defer held.Release()

	if n := CleanupStaleLocks(root); n != 3 {
		t.Errorf("want the 3 stale lock files removed, got %d", n)
	}
	for _, p := range stale {
		if FileExists(p) {
			t.Errorf("%s should be gone", p)
		}
	}
	for _, p := range []string{notALock, withData, filepath.Join(show, "busy.mp3.lock")} {
		if !FileExists(p) {
			t.Errorf("%s must be left alone", p)
		}
	}
	if again, _ := AcquireFileLock(filepath.Join(show, "busy.mp3")); again != nil {
		t.Error("the sweep must not have broken a held lock")
	}
}
