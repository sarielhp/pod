package podtest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"localhost":   true,
		"127.0.0.1":   true,
		"127.8.9.10":  true,
		"::1":         true,
		"[::1]":       true,
		"10.0.0.1":    false,
		"192.168.1.1": false,
		"example.com": false,
		"":            false,
	}
	for host, want := range cases {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestOfflineTransportServesLoopbackAndRefusesTheRest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	client := &http.Client{Transport: OfflineTransport()}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("loopback request refused: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("loopback status = %d", resp.StatusCode)
	}

	_, err = client.Get("http://example.invalid/feed.xml")
	if err == nil || !strings.Contains(err.Error(), "refusing to reach") {
		t.Fatalf("non-loopback request was not refused: %v", err)
	}
}

func TestIsolateMainRedirectsHomeAndXDGDirs(t *testing.T) {
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		v := os.Getenv(key)
		if v == "" {
			t.Errorf("%s is unset inside an isolated test binary", key)
			continue
		}
		if !strings.Contains(v, "pod-test-home-") {
			t.Errorf("%s = %q does not point at the throwaway home", key, v)
		}
	}
}
