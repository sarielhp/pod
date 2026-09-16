package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloaderSuccess(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("User-Agent"), "pod") {
			http.Error(w, "invalid user agent", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("dummy audio mp3 content"))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	destFile := filepath.Join(tmpDir, "episode.mp3")

	d := NewDownloader()
	err := d.DownloadEpisode(context.Background(), ts.URL+"/audio.mp3", destFile, nil)
	if err != nil {
		t.Fatalf("DownloadEpisode failed: %v", err)
	}

	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != "dummy audio mp3 content" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestDownloaderHTTPError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	destFile := filepath.Join(tmpDir, "missing.mp3")

	d := NewDownloader()
	err := d.DownloadEpisode(context.Background(), ts.URL+"/missing.mp3", destFile, nil)
	if err == nil {
		t.Fatalf("expected error on 404, got nil")
	}

	if _, err := os.Stat(destFile); err == nil {
		t.Fatalf("destination file should not exist on failure")
	}
}

func TestDownloaderContextCancelled(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("too slow"))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	destFile := filepath.Join(tmpDir, "cancelled.mp3")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	d := NewDownloader()
	err := d.DownloadEpisode(ctx, ts.URL, destFile, nil)
	if err == nil {
		t.Fatalf("expected error on cancelled context")
	}
}

func TestDownloadCoverImage(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake cover image bytes"))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "cover.jpg")
	if err := downloadCoverImage(ts.URL+"/cover.jpg", dest); err != nil {
		t.Fatalf("downloadCoverImage failed: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "fake cover image bytes" {
		t.Errorf("unexpected cover file data: %s, err: %v", string(data), err)
	}
}
