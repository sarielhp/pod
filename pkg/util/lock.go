package util

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// FileLockWrapper holds an exclusive advisory lock on "<target>.lock". The lock
// file exists only while the lock is held: Release deletes it, so finished work
// leaves nothing behind. (The library once carried 1,752 empty lock files, one for
// every lock ever taken.)
type FileLockWrapper struct {
	f        *os.File
	lockPath string
}

// maxLockAttempts bounds the retries when the lock file is replaced under us.
const maxLockAttempts = 8

// tryLock makes one attempt at the lock on lockPath. It returns (nil, nil) when
// another process holds it.
//
// Deleting the lock file on release makes one case delicate: a process can open
// the file just before the holder deletes it, and then win the flock on a file
// that no longer exists at the path, while a third process creates a fresh file
// there and locks that too. Both would think they held the lock. So after
// winning the flock the file is compared with what is at the path, and the
// attempt starts over if they differ.
func tryLock(lockPath string) (*os.File, error) {
	for attempt := 0; attempt < maxLockAttempts; attempt++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to acquire lock on %s: %w", lockPath, err)
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to acquire lock on %s: %w", lockPath, err)
		}
		if holdsPath(f, lockPath) {
			return f, nil
		}
		unlockAndClose(f)
	}
	return nil, fmt.Errorf("failed to acquire lock on %s: the lock file keeps being replaced", lockPath)
}

// holdsPath reports whether f is still the file at path.
func holdsPath(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	current, err := os.Stat(path)
	return err == nil && os.SameFile(held, current)
}

func unlockAndClose(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

// AcquireFileLock takes the lock without waiting. It returns (nil, nil) when
// another process holds it.
func AcquireFileLock(targetPath string) (*FileLockWrapper, error) {
	lockPath := targetPath + ".lock"
	f, err := tryLock(lockPath)
	if err != nil || f == nil {
		return nil, err
	}
	return &FileLockWrapper{f: f, lockPath: lockPath}, nil
}

// AcquireFileLockWithTimeout keeps trying for up to timeout. It returns (nil, nil)
// if the lock stayed with another process.
func AcquireFileLockWithTimeout(targetPath string, timeout time.Duration) (*FileLockWrapper, error) {
	deadline := time.Now().Add(timeout)
	for {
		w, err := AcquireFileLock(targetPath)
		if err != nil || w != nil {
			return w, err
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Release deletes the lock file and gives up the lock. The file is deleted first,
// while the lock is still held, so that no one can lock the file being removed.
func (w *FileLockWrapper) Release() {
	if w == nil || w.f == nil {
		return
	}
	f := w.f
	w.f = nil
	_ = os.Remove(w.lockPath)
	unlockAndClose(f)
}

func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
