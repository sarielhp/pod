package podcast

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func withPrivateHostsBlocked(t *testing.T) {
	t.Helper()
	SetAllowPrivateHosts(false)
	t.Cleanup(func() { SetAllowPrivateHosts(true) })
}

func TestGuardedDialRefusesPrivateAddresses(t *testing.T) {
	withPrivateHostsBlocked(t)
	dial := guardedDial(func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("dialer reached for a private address")
		return nil, nil
	})
	for _, addr := range []string{"127.0.0.1:80", "10.0.0.5:8080", "192.168.1.1:443", "172.16.0.1:80", "[::1]:80", "169.254.169.254:80", "[fe80::1]:80", "0.0.0.0:80"} {
		_, err := dial(context.Background(), "tcp", addr)
		if err == nil || !strings.Contains(err.Error(), "private or local") {
			t.Errorf("%s was not refused: %v", addr, err)
		}
	}
}

func TestGuardedDialPassesPublicAddressesAndHonoursTheOptOut(t *testing.T) {
	withPrivateHostsBlocked(t)
	var dialed string
	dial := guardedDial(func(_ context.Context, _ string, addr string) (net.Conn, error) {
		dialed = addr
		return nil, nil
	})
	if _, err := dial(context.Background(), "tcp", "93.184.216.34:443"); err != nil || dialed != "93.184.216.34:443" {
		t.Fatalf("public address blocked: %v (dialed %q)", err, dialed)
	}
	SetAllowPrivateHosts(true)
	if _, err := dial(context.Background(), "tcp", "127.0.0.1:80"); err != nil || dialed != "127.0.0.1:80" {
		t.Fatalf("opt-out not honoured: %v (dialed %q)", err, dialed)
	}
}

func TestEnclosureAndCoverDownloadsAreGuarded(t *testing.T) {
	withPrivateHostsBlocked(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret"))
	}))
	defer ts.Close()
	dir := t.TempDir()
	if err := NewDownloader().DownloadEpisode(context.Background(), ts.URL+"/e.mp3", filepath.Join(dir, "e.mp3"), nil); err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("episode download reached a loopback server: %v", err)
	}
	if err := downloadCoverImage(ts.URL+"/c.jpg", filepath.Join(dir, "c.jpg")); err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("cover download reached a loopback server: %v", err)
	}
}

func TestFeedFetchIsGuardedThroughRedirects(t *testing.T) {
	withPrivateHostsBlocked(t)
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<rss><channel><title>x</title></channel></rss>`))
	}))
	defer inner.Close()
	client := &http.Client{Transport: newGuardedTransport()}
	_, err := fetchFeedConditional(inner.URL+"/feed.xml", FeedFetchOptions{Client: client, MaxAttempts: 1})
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("feed fetch reached a loopback server: %v", err)
	}
}
