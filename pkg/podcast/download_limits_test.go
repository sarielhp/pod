package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func limitedDownloader(max int64) *Downloader {
	d := NewDownloader()
	d.MaxBytes = max
	return d
}

func TestDownloaderRejectsABodyOverTheLimit(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		f, _ := w.(http.Flusher)
		for i := 0; i < 8; i++ {
			_, _ = w.Write([]byte(strings.Repeat("x", 100)))
			if f != nil {
				f.Flush()
			}
		}
	}))
	defer ts.Close()
	dest := filepath.Join(t.TempDir(), "ep.mp3")
	err := limitedDownloader(500).DownloadEpisode(context.Background(), ts.URL+"/a.mp3", dest, nil)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("an 800-byte body under a 500-byte cap was accepted: %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("oversized download was installed")
	}
	if entries, _ := os.ReadDir(filepath.Join(filepath.Dir(dest), ".work")); len(entries) != 0 {
		t.Fatalf(".work still holds %d file(s) after the rejected download", len(entries))
	}
}

func TestDownloaderRejectsADeclaredLengthOverTheLimit(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write([]byte("abc"))
	}))
	defer ts.Close()
	dest := filepath.Join(t.TempDir(), "ep.mp3")
	err := limitedDownloader(500).DownloadEpisode(context.Background(), ts.URL+"/a.mp3", dest, nil)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("a declared 1 MB body under a 500-byte cap was accepted: %v", err)
	}
}

func TestDownloaderRejectsAnHTMLPageServedAsAudio(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>login required</html>"))
	}))
	defer ts.Close()
	dest := filepath.Join(t.TempDir(), "ep.mp3")
	err := NewDownloader().DownloadEpisode(context.Background(), ts.URL+"/a.mp3", dest, nil)
	if err == nil || !strings.Contains(err.Error(), "text/html") {
		t.Fatalf("an HTML page was saved as an episode: %v", err)
	}
}

func TestDownloaderAcceptsOctetStream(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("ID3audio"))
	}))
	defer ts.Close()
	dest := filepath.Join(t.TempDir(), "ep.mp3")
	if err := NewDownloader().DownloadEpisode(context.Background(), ts.URL+"/a.mp3", dest, nil); err != nil {
		t.Fatalf("octet-stream audio rejected: %v", err)
	}
}

func TestFetchFeedRejectsAFeedOverTheLimit(t *testing.T) {
	orig := maxFeedSize
	maxFeedSize = 256
	t.Cleanup(func() { maxFeedSize = orig })
	body := `<?xml version="1.0"?><rss><channel><title>Big</title><description>` + strings.Repeat("y", 400) + `</description></channel></rss>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	_, err := fetchFeedConditional(ts.URL+"/feed.xml", FeedFetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("a feed over the size limit was parsed instead of rejected: %v", err)
	}
}
