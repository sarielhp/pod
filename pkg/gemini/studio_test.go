package gemini

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func useStudioBaseURL(t *testing.T, base string) {
	t.Helper()
	old := studioBaseURL
	studioBaseURL = base
	t.Cleanup(func() { studioBaseURL = old })
}

func TestUploadAudioToGeminiStudioReleasesThePipeWhenTheRequestCannotBeBuilt(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "chunk.wav")
	if err := os.WriteFile(audio, []byte("RIFF"), 0644); err != nil {
		t.Fatal(err)
	}
	useStudioBaseURL(t, "http://[::1]:namedport")

	before := runtime.NumGoroutine()
	_, _, err := UploadAudioToGeminiStudio(context.Background(), "test-key", audio)
	if err == nil {
		t.Fatal("an unparseable upload URL must fail the upload")
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("the multipart writer goroutine is still blocked on the pipe: %d goroutines, %d before the call", runtime.NumGoroutine(), before)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
