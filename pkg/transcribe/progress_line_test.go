package transcribe

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// captureStdout returns what f prints to standard output.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
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
	f()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func TestProgressLineIsEndedSoTheNextMessageStartsFresh(t *testing.T) {
	old := progressInterval
	progressInterval = 5 * time.Millisecond
	t.Cleanup(func() { progressInterval = old })

	out := captureStdout(t, func() {
		stop := startProgressTicker(false, time.Now())
		time.Sleep(40 * time.Millisecond)
		stop()
		stop() // stopping twice is harmless
		os.Stdout.WriteString("Saved transcript\n")
	})
	if !strings.Contains(out, "Elapsed") {
		t.Fatalf("the ticker should have drawn, got %q", out)
	}
	if !strings.Contains(out, "   \nSaved transcript") {
		t.Errorf("the message after the ticker must start on its own line, got %q", out)
	}
}

func TestQuietAndInstantRunsDrawNothing(t *testing.T) {
	out := captureStdout(t, func() {
		startProgressTicker(true, time.Now())()
		startProgressTicker(false, time.Now())() // stopped before the first redraw
	})
	if out != "" {
		t.Errorf("nothing should be printed, got %q", out)
	}
}
