package util

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupStaleWorkDirs(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"stale", "fresh child", "boundary", "locked", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			now := time.Now()
			work := filepath.Join(root, ".work")
			nested := filepath.Join(work, "episode.mp3")
			if err := os.MkdirAll(nested, 0755); err != nil {
				t.Fatal(err)
			}
			audio := filepath.Join(root, "episode.mp3")
			child := filepath.Join(nested, "cache.wav")
			for _, path := range []string{audio, child} {
				if err := os.WriteFile(path, []byte("preserve audio"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "symlink" {
				if err := os.Symlink(audio, filepath.Join(nested, "link")); err != nil {
					t.Fatal(err)
				}
			}
			old := now.Add(-25 * time.Hour)
			for _, path := range []string{child, nested, work} {
				if err := os.Chtimes(path, old, old); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "fresh child" || scenario == "boundary" {
				modified := now
				if scenario == "boundary" {
					modified = now.Add(-24 * time.Hour)
				}
				if err := os.Chtimes(child, modified, modified); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "locked" {
				lock, err := AcquireFileLock(audio)
				if err != nil || lock == nil {
					t.Fatalf("lock: %v", err)
				}
				defer lock.Release()
			}
			count, err := CleanupStaleWorkDirs(root, now)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if scenario == "stale" {
				want = 1
			}
			if count != want {
				t.Fatalf("removed %d directories, want %d", count, want)
			}
			if _, err := os.Stat(audio); err != nil {
				t.Fatalf("source audio affected: %v", err)
			}
			_, err = os.Stat(work)
			if (want == 1) != os.IsNotExist(err) {
				t.Fatalf("work directory existence: %v", err)
			}
		})
	}
}
