package gemini

import (
	"reflect"
	"testing"

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

func TestModelSelectorAdvancesOncePerModel(t *testing.T) {
	t.Parallel()
	sel := newModelSelector([]string{"a", "b", "c"})

	if got := sel.current(); got != "a" {
		t.Fatalf("current = %q, want a", got)
	}

	// Several chunks failing on the same model must cost one step in total,
	// otherwise one exhausted model skips the whole chain.
	next, ok := sel.advancePast("a")
	if !ok || next != "b" {
		t.Fatalf("advancePast(a) = %q, %v; want b, true", next, ok)
	}
	next, ok = sel.advancePast("a")
	if !ok || next != "b" {
		t.Fatalf("stale failure advanced the chain: got %q, %v", next, ok)
	}

	if next, ok = sel.advancePast("b"); !ok || next != "c" {
		t.Fatalf("advancePast(b) = %q, %v; want c, true", next, ok)
	}
	if next, ok = sel.advancePast("c"); ok {
		t.Fatalf("chain should be exhausted, got %q", next)
	}
	if got := sel.current(); got != "" {
		t.Errorf("current after exhaustion = %q, want empty", got)
	}
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
