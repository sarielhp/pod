package adremoval

import (
	"os"
	"testing"
	"time"

	"pod/pkg/detect"
)

// TestMain redirects the user cache directory into a throwaway one. Several
// code paths reach a process-wide cache keyed off XDG_CACHE_HOME, and a test
// run must never write into the real one.
func TestMain(m *testing.M) {
	detect.SetRetryBackoff(func(int) time.Duration { return 0 })

	dir, err := os.MkdirTemp("", "abs-test-cache-")
	if err == nil {
		os.Setenv("XDG_CACHE_HOME", dir)
	}
	code := m.Run()
	if dir != "" {
		// os.Exit skips deferred calls, so clean up before it.
		os.RemoveAll(dir)
	}
	os.Exit(code)
}
