package gemini

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"pod/pkg/port"
	"pod/pkg/types"
)

func TestGeminiModelChain(t *testing.T) {
	t.Parallel()

	t.Run("unconfigured uses the pinned chain", func(t *testing.T) {
		t.Parallel()
		if got := GeminiModelChain(&types.Config{}); !reflect.DeepEqual(got, DefaultGeminiModelChain) {
			t.Errorf("chain = %v, want %v", got, DefaultGeminiModelChain)
		}
	})

	t.Run("a configured model leads but does not stand alone", func(t *testing.T) {
		t.Parallel()
		cfg := &types.Config{}
		cfg.GeminiModel = "gemini-9-flash"
		got := GeminiModelChain(cfg)
		if got[0] != "gemini-9-flash" {
			t.Errorf("configured model not first: %v", got)
		}
		if len(got) != len(DefaultGeminiModelChain)+1 {
			t.Errorf("fallbacks dropped: %v", got)
		}
	})

	t.Run("a configured model already in the chain is not duplicated", func(t *testing.T) {
		t.Parallel()
		cfg := &types.Config{}
		cfg.GeminiModel = DefaultGeminiModelChain[2]
		got := GeminiModelChain(cfg)
		if len(got) != len(DefaultGeminiModelChain) {
			t.Errorf("duplicate entry: %v", got)
		}
		if got[0] != DefaultGeminiModelChain[2] {
			t.Errorf("configured model not first: %v", got)
		}
	})

	t.Run("the chain never leads with an alias", func(t *testing.T) {
		t.Parallel()
		// An alias follows Google's newest model, which carries the smallest
		// free-tier allowance, and names nothing durable on a transcript.
		for _, m := range DefaultGeminiModelChain {
			if m == "gemini-flash-latest" || m == "gemini-flash-lite-latest" {
				t.Errorf("chain contains the moving alias %q", m)
			}
		}
	})
}

// The chain walking itself is pkg/port's job and is tested there. What
// belongs here is the translation: turning one protocol's response into the
// verdict the port acts on.
func TestStudioFailureClassification(t *testing.T) {
	t.Parallel()

	t.Run("a rate limit carries the delay the server asked for", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"error":{"message":"Quota exceeded. Please retry in 58.826548043s."}}`)
		got := studioFailure("gemini-3.8-flash", http.StatusTooManyRequests, body)
		if got.Verdict != port.RateLimited {
			t.Errorf("verdict = %v, want RateLimited", got.Verdict)
		}
		if got.RetryAfter < 59*time.Second || got.RetryAfter > 60*time.Second {
			t.Errorf("retry after = %v, want ~59.8s", got.RetryAfter)
		}
		if got.Daily {
			t.Error("a per-minute limit was reported as a daily one")
		}
	})

	t.Run("a retired model is skipped, not retried", func(t *testing.T) {
		t.Parallel()
		if got := studioFailure("old", http.StatusNotFound, nil); got.Verdict != port.ModelGone {
			t.Errorf("verdict = %v, want ModelGone", got.Verdict)
		}
	})

	t.Run("high demand is transient, not a quota", func(t *testing.T) {
		t.Parallel()
		if got := studioFailure("m", http.StatusServiceUnavailable, nil); got.Verdict != port.Overloaded {
			t.Errorf("verdict = %v, want Overloaded", got.Verdict)
		}
	})

	t.Run("a malformed request does not spend the chain", func(t *testing.T) {
		t.Parallel()
		// No other model would survive it, so trying them wastes quota.
		if got := studioFailure("m", http.StatusBadRequest, nil); got.Verdict != port.Fatal {
			t.Errorf("verdict = %v, want Fatal", got.Verdict)
		}
	})
}

func TestResolvedModelVersion(t *testing.T) {
	t.Parallel()
	mk := func(v string) *types.GeminiChunkResult {
		return &types.GeminiChunkResult{Payload: &types.GeminiResponsePayload{ModelVersion: v}}
	}
	if got := resolvedModelVersion(nil); got != "" {
		t.Errorf("empty results = %q", got)
	}
	if got := resolvedModelVersion([]*types.GeminiChunkResult{mk("x"), nil, mk("")}); got != "x" {
		t.Errorf("got %q, want x", got)
	}
	// After a mid-run switch the later model is the one that finished.
	if got := resolvedModelVersion([]*types.GeminiChunkResult{mk("old"), mk("new")}); got != "new" {
		t.Errorf("got %q, want new", got)
	}
}
