package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteFileAtomicSavesANameNearTheLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Hebrew letters take two bytes each: 116 of them and ".transcript.json" is 248
	// bytes, inside the 255-byte limit but with no room for a suffix on the target.
	name := strings.Repeat("ש", 116) + ".transcript.json"
	if len(name) < 240 || len(name) > 255 {
		t.Fatalf("test premise: want a name of 240-255 bytes, got %d", len(name))
	}
	path := filepath.Join(dir, name)
	if err := WriteFileAtomic(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("a name that fits must be writable: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "{}" {
		t.Errorf("content wrong: %q, %v", got, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp.*")); len(left) != 0 {
		t.Errorf("no temporary file may be left behind, found %v", left)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("only the target should remain, got %d entries", len(entries))
	}
}

func TestWriteFileAtomicWritersInOneDirectoryDoNotCollide(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join(dir, fmt.Sprintf("file%02d.json", i))
			if err := WriteFileAtomic(path, []byte(fmt.Sprint(i)), 0o644); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for i := 0; i < 64; i++ {
		got, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("file%02d.json", i)))
		if err != nil || string(got) != fmt.Sprint(i) {
			t.Errorf("file %d holds %q (%v): a writer clobbered another's temporary file", i, got, err)
		}
	}
}
