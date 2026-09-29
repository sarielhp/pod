package transcribe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const wakingPage = `<!DOCTYPE html><html><head><title>Sablier</title></head><body>Starting whisper...</body></html>`

const transcriptJSON = `{"text":"hello there","segments":[{"start":0,"end":1,"text":"hello there"}]}`

// wakingServer answers the first `sleeping` requests as the Sablier waiting page
// does, with a web page and status 200, and every later one with a transcript.
func wakingServer(t *testing.T, sleeping int) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		n := int(atomic.AddInt32(&calls, 1))
		if n <= sleeping {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, wakingPage)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, transcriptJSON)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func fastWake(t *testing.T) {
	t.Helper()
	oldWake, oldUnit := wakeRetryDelay, retryUnit
	wakeRetryDelay, retryUnit = time.Millisecond, time.Millisecond
	t.Cleanup(func() { wakeRetryDelay, retryUnit = oldWake, oldUnit })
}

func transcribeVia(url string) error {
	pcm := make([]byte, 3200)
	_, err := TranscribeWhisperContext(context.Background(), "clip.wav", url, true, false, 1, 1, "", "", "", pcm)
	return err
}

func TestSleepingWhisperIsWokenByRetryingNotReportedAsAFailure(t *testing.T) {
	fastWake(t)
	// More waiting pages than the five ordinary attempts: waking must not use those up.
	srv, calls := wakingServer(t, 8)
	if err := transcribeVia(srv.URL); err != nil {
		t.Fatalf("a server that wakes up after 8 tries should succeed, got: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 9 {
		t.Errorf("want 8 waiting pages then the transcript (9 calls), got %d", got)
	}
}

func TestWhisperThatNeverWakesGivesUpAndShowsWhatItSent(t *testing.T) {
	fastWake(t)
	srv, calls := wakingServer(t, 1<<30)
	err := transcribeVia(srv.URL)
	if err == nil {
		t.Fatal("a server that never wakes must eventually fail")
	}
	if !strings.Contains(err.Error(), "waking up") || !strings.Contains(err.Error(), "Starting whisper") {
		t.Errorf("the error should say the server was waking and quote its page, got: %v", err)
	}
	// The wake-up allowance, then the ordinary attempts once it is spent.
	if got := int(atomic.LoadInt32(calls)); got < maxWakeRetries {
		t.Errorf("want at least %d tries before giving up, got %d", maxWakeRetries, got)
	}
}

func TestLooksLikeWakingPage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		contentType string
		body        string
		want        bool
	}{
		{"text/html", "anything", true},
		{"application/json", "  <html>waiting</html>", true},
		{"", "<!DOCTYPE html>", true},
		{"application/json", transcriptJSON, false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := looksLikeWakingPage(c.contentType, []byte(c.body)); got != c.want {
			t.Errorf("looksLikeWakingPage(%q, %q) = %v, want %v", c.contentType, c.body, got, c.want)
		}
	}
}

func TestCancellingWhileWhisperWakesStopsAtOnce(t *testing.T) {
	old := wakeRetryDelay
	wakeRetryDelay = time.Hour
	t.Cleanup(func() { wakeRetryDelay = old })
	srv, _ := wakingServer(t, 1<<30)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := TranscribeWhisperContext(ctx, "clip.wav", srv.URL, true, false, 1, 1, "", "", "", make([]byte, 3200))
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled transcription must return an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling should not wait out the retry delay")
	}
}
