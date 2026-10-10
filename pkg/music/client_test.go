package music

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyWakeUploadAndCache(t *testing.T) {
	healthCalls, uploads := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			healthCalls++
			if healthCalls == 1 {
				w.Header().Set("Content-Type", "text/html")
				io.WriteString(w, "<html>Sablier waking</html>")
				return
			}
			io.WriteString(w, `{"status":"healthy"}`)
			return
		}
		if r.URL.Path != "/inference" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		uploads++
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		if string(data) != "original audio" || r.FormValue("include_frames") != "true" || r.FormValue("threshold") != "0.1" {
			t.Errorf("invalid multipart upload: %q", data)
		}
		io.WriteString(w, `{"duration":1,"frames":[{"time":0,"music_score":0.9}]}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	input, output := filepath.Join(dir, "audio.mp3"), filepath.Join(dir, "audio.yamnet.json")
	if err := os.WriteFile(input, []byte("original audio"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for range 2 {
		result, err := Classify(ctx, srv.URL, input, output)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Frames) != 1 {
			t.Fatal(result)
		}
	}
	if uploads != 1 || healthCalls != 2 {
		t.Fatalf("cache/wake calls = %d uploads, %d health", uploads, healthCalls)
	}
	if err := os.Chtimes(input, time.Unix(123, 0), time.Unix(123, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := Classify(ctx, srv.URL, input, output); err != nil {
		t.Fatal(err)
	}
	if uploads != 2 {
		t.Fatal("changed original reused stale classification")
	}
}

func TestClassifierCancellationAndInvalidCoverage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>waking</html>") }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := awaitReady(ctx, srv.Client(), srv.URL); err == nil {
		t.Fatal("cancelled readiness succeeded")
	}
	for _, raw := range []string{
		`{"duration":60,"frames":[{"time":0,"music_score":0.9}]}`,
		`{"duration":1,"frames":[{"time":0,"speech_score":2}]}`,
		`{"duration":1,"frames":[{"time":0.2}]}`,
		`{"duration":1,"frames":[]}`,
	} {
		var result Result
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			t.Fatal(err)
		}
		if err := validateResult(&result); err == nil {
			t.Errorf("invalid result accepted: %s", raw)
		}
	}
	if _, err := infer(context.Background(), "file:///tmp/model", "absent"); err == nil || !strings.Contains(err.Error(), "invalid yamnet_url") {
		t.Fatal(err)
	}
}
