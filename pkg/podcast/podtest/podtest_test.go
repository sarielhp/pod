package podtest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type recordingTB struct {
	testing.TB
	skipped string
	fatal   string
}

func (r *recordingTB) Helper()                        {}
func (r *recordingTB) Skip(args ...any)               { r.skipped = fmt.Sprint(args...) }
func (r *recordingTB) Fatalf(format string, a ...any) { r.fatal = fmt.Sprintf(format, a...) }

func TestRequireFFmpegIsLoudWhenFFmpegIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("POD_TEST_REQUIRE_FFMPEG", "")
	rec := &recordingTB{TB: t}
	RequireFFmpeg(rec)
	if !strings.Contains(rec.skipped, "SKIPPED") || !strings.Contains(rec.skipped, "POD_TEST_REQUIRE_FFMPEG") || rec.fatal != "" {
		t.Fatalf("without the env var a missing ffmpeg must skip with a visible message: skip=%q fatal=%q", rec.skipped, rec.fatal)
	}

	t.Setenv("POD_TEST_REQUIRE_FFMPEG", "1")
	rec = &recordingTB{TB: t}
	RequireFFmpeg(rec)
	if rec.fatal == "" || rec.skipped != "" {
		t.Fatalf("with the env var a missing ffmpeg must fail: skip=%q fatal=%q", rec.skipped, rec.fatal)
	}
}

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
