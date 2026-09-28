// Package podtest holds the setup a test binary needs before it runs anything
// that could reach the network or sleep on a retry.
//
// It exists because the suite used to do neither: feed fetches went to the
// real internet, and a failed fetch waited out a one- then two-second backoff.
// A single test that named an unreachable feed cost three and a half seconds,
// and the whole suite's timing depended on DNS.
package podtest

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// offlineTransport serves loopback requests — httptest servers live there —
// and fails everything else immediately rather than waiting for DNS or a
// connect timeout.
type offlineTransport struct{ inner http.RoundTripper }

func (t offlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isLoopback(req.URL.Hostname()) {
		return t.inner.RoundTrip(req)
	}
	return nil, fmt.Errorf("podtest: refusing to reach %q; tests must not use the network", req.URL.Host)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// OfflineTransport serves loopback and refuses everything else. Install it
// with podcast.SetFeedTransport from a TestMain, alongside a zero retry delay.
//
// It deliberately does not wire itself up: podcast's own tests need it too, and
// importing podcast here would be a cycle.
func OfflineTransport() http.RoundTripper {
	return offlineTransport{inner: http.DefaultTransport}
}

// IsolateMain runs a package's tests with HOME, XDG_CONFIG_HOME and
// XDG_CACHE_HOME pointed at a throwaway directory, so nothing a test does can
// reach the user's real ~/.config/pod or ~/.cache/pod, and returns the exit
// code for os.Exit. setup runs after the environment is redirected and before
// any test; it is where a package installs OfflineTransport and zeroes its
// retry delays, since podtest cannot import those packages itself.
//
// It exists because pkg/player's tests once persisted a play queue into the
// user's real config directory: the package had no TestMain, and each of the
// five packages that did had hand-rolled only the cache redirect.
func IsolateMain(m *testing.M, setup func()) int {
	dir, err := os.MkdirTemp("", "pod-test-home-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "podtest: cannot create isolated home: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)
	for _, kv := range [][2]string{
		{"HOME", dir},
		{"XDG_CONFIG_HOME", filepath.Join(dir, ".config")},
		{"XDG_CACHE_HOME", filepath.Join(dir, ".cache")},
	} {
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			fmt.Fprintf(os.Stderr, "podtest: cannot set %s: %v\n", kv[0], err)
			return 1
		}
	}
	if setup != nil {
		setup()
	}
	return m.Run()
}

// RequireFFmpeg skips the calling test when ffmpeg is not installed, saying
// so loudly, and fails it instead when POD_TEST_REQUIRE_FFMPEG is set, so a
// CI machine can insist that the ffmpeg-backed tests really ran rather than
// quietly reporting a green suite that never exercised the cutter.
func RequireFFmpeg(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		return
	}
	if os.Getenv("POD_TEST_REQUIRE_FFMPEG") != "" {
		t.Fatalf("ffmpeg is not installed and POD_TEST_REQUIRE_FFMPEG is set")
		return
	}
	t.Skip("SKIPPED: ffmpeg is not installed, so this ffmpeg-backed test did not run (set POD_TEST_REQUIRE_FFMPEG=1 to fail instead)")
}
