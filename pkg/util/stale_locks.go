package util

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// CleanupStaleLocks deletes the empty lock files under root, and one directory
// below it, that no process holds: the leftovers of older versions, which never
// removed their lock files, and of processes that died while holding one. A lock
// file that is held is left alone. It returns how many were deleted.
func CleanupStaleLocks(root string) int {
	removed := 0
	dirs := []string{root}
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
	}
	for _, dir := range dirs {
		removed += cleanupLocksIn(dir)
	}
	return removed
}

func cleanupLocksIn(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".lock") && removeIfUnheld(filepath.Join(dir, e.Name())) {
			removed++
		}
	}
	return removed
}

// removeIfUnheld deletes an empty lock file if it can take the lock itself,
// which proves no one else holds it.
func removeIfUnheld(path string) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || fi.Size() != 0 {
		return false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return false
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if !holdsPath(f, path) {
		return false
	}
	return os.Remove(path) == nil
}
