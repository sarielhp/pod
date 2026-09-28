package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDeleteGeminiStudioFileReportsAFailedDelete(t *testing.T) {
	var gotMethod, gotPath string
	status := http.StatusInternalServerError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer srv.Close()
	useStudioBaseURL(t, srv.URL)

	err := DeleteGeminiStudioFile("test-key", "files/abc123")
	if err == nil {
		t.Fatal("a 500 from the delete endpoint must be reported")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should carry the status and the API message, got %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/v1beta/files/abc123" {
		t.Fatalf("request was %s %s, want DELETE /v1beta/files/abc123", gotMethod, gotPath)
	}

	status = http.StatusOK
	if err := DeleteGeminiStudioFile("test-key", "files/abc123"); err != nil {
		t.Fatalf("a successful delete must not error: %v", err)
	}
}

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
