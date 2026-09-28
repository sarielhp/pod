package gemini

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/progress"
	"pod/pkg/types"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

// A missing input makes the chunk split fail before any upload, so the only
// observable side effect of ProcessWithGeminiConfig here is what it announces.
func TestProcessWithGeminiConfigReportsInsteadOfPrinting(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	missing := filepath.Join(t.TempDir(), "missing.mp3")
	var rep progress.Lines
	var err error
	out := captureStdout(t, func() {
		_, _, err = ProcessWithGeminiConfig(context.Background(), missing, types.Config{}, 1, &rep)
	})
	if err == nil || strings.Contains(err.Error(), "cooldown") {
		t.Fatalf("expected the chunk split to fail on a missing file, got err=%v", err)
	}
	if out != "" {
		t.Errorf("wrote to stdout instead of the reporter:\n%s", out)
	}
	if !strings.Contains(strings.Join(rep.Info, "\n"), "Gemini") {
		t.Errorf("the backend announcement did not reach the reporter; got %q", rep.All())
	}
}
