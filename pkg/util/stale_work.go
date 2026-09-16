package util

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func CleanupStaleWorkDirs(root string, now time.Time) (int, error) {
	if root == "" {
		return 0, nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return 0, err
	}
	removed := 0
	var failures []error
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			failures = append(failures, walkErr)
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() == ".work" {
			deleted, err := removeStaleWorkDir(root, path, now.Add(-24*time.Hour))
			if err != nil {
				failures = append(failures, fmt.Errorf("cleanup %s: %w", path, err))
			}
			if deleted {
				removed++
			}
			return filepath.SkipDir
		}
		if path != root && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		return nil
	})
	return removed, errors.Join(append(failures, err)...)
}

func workDirIsStale(path string, cutoff time.Time) (bool, error) {
	stale := true
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || !info.ModTime().Before(cutoff) {
			stale = false
			return fs.SkipAll
		}
		return nil
	})
	return stale && err == nil, err
}

func removeStaleWorkDir(root, path string, cutoff time.Time) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || filepath.Base(path) != ".work" {
		return false, err
	}
	stale, err := workDirIsStale(path, cutoff)
	if err != nil || !stale {
		return false, err
	}
	release, err := lockWorkDirAudio(root, filepath.Dir(path))
	if err != nil || release == nil {
		return false, err
	}
	defer release()
	stale, err = workDirIsStale(path, cutoff)
	if err != nil || !stale {
		return false, err
	}
	if err := os.RemoveAll(path); err != nil {
		return false, err
	}
	return true, nil
}

func lockWorkDirAudio(root, dir string) (func(), error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var locks []*FileLockWrapper
	release := func() {
		for _, lock := range locks {
			lock.Release()
		}
	}
	for parent := dir; ; parent = filepath.Dir(parent) {
		for _, name := range []string{".worker", ".collect"} {
			target := filepath.Join(parent, name)
			_, err := os.Lstat(target + ".lock")
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				release()
				return nil, err
			}
			lock, err := AcquireFileLock(target)
			if err != nil || lock == nil {
				release()
				return nil, err
			}
			locks = append(locks, lock)
		}
		if parent == root || parent == filepath.Dir(parent) {
			break
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".mp3") {
			continue
		}
		lock, err := AcquireFileLock(filepath.Join(dir, entry.Name()))
		if err != nil || lock == nil {
			release()
			return nil, err
		}
		locks = append(locks, lock)
	}
	return release, nil
}
