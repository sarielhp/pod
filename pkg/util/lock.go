package util

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/gofrs/flock"
)

type FileLockWrapper struct {
	fl       *flock.Flock
	lockPath string
}

func AcquireFileLock(targetPath string) (*FileLockWrapper, error) {
	lockPath := targetPath + ".lock"
	fl := flock.New(lockPath)

	locked, err := fl.TryLock()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock on %s: %w", lockPath, err)
	}
	if !locked {
		return nil, nil
	}

	return &FileLockWrapper{fl: fl, lockPath: lockPath}, nil
}

func AcquireFileLockWithTimeout(targetPath string, timeout time.Duration) (*FileLockWrapper, error) {
	lockPath := targetPath + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to acquire lock on %s: %w", lockPath, err)
	}
	if !locked {
		return nil, nil
	}

	return &FileLockWrapper{fl: fl, lockPath: lockPath}, nil
}

func (w *FileLockWrapper) Release() {
	if w == nil || w.fl == nil {
		return
	}
	fl := w.fl
	w.fl = nil
	_ = fl.Unlock()
}

func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
