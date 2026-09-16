package detect

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"pod/pkg/port"
	"pod/pkg/types"
)

func TestModelChainFor(t *testing.T) {
	t.Parallel()

	t.Run("google's endpoint gets the fallback chain", func(t *testing.T) {
		t.Parallel()
		got := ModelChainFor(types.LLMProfile{
			URL:   "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
			Model: "gemini-3.8-flash",
		})
		if !reflect.DeepEqual(got, types.DefaultGeminiModelChain) {
			t.Errorf("chain = %v, want %v", got, types.DefaultGeminiModelChain)
		}
	})

	t.Run("an alias is replaced by pinned models", func(t *testing.T) {
		t.Parallel()
		// gemini-flash-latest follows whatever is newest, which is also
		// whatever is most rate-limited, and records nothing durable.
		got := ModelChainFor(types.LLMProfile{
			URL:   "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
			Model: "gemini-flash-latest",
		})
		for _, m := range got {
			if strings.HasSuffix(m, "-latest") {
				t.Errorf("chain still leads with an alias: %v", got)
			}
		}
		if len(got) == 0 {
			t.Fatal("alias profile produced an empty chain")
		}
	})

	t.Run("another provider is never substituted", func(t *testing.T) {
		t.Parallel()
		// An OpenRouter profile names a vendor model the user picked. Quietly
		// answering with a different one is precisely the substitution this
		// codebase has already been bitten by.
		got := ModelChainFor(types.LLMProfile{
			URL:   "https://openrouter.ai/api/v1/chat/completions",
			Model: "deepseek/deepseek-v4-flash",
		})
		if len(got) != 1 || got[0] != "deepseek/deepseek-v4-flash" {
			t.Errorf("chain = %v, want the single configured model", got)
		}
	})
}

func TestPortsSeparateProvidersServingTheSameModel(t *testing.T) {
	t.Parallel()
	// Measured, not assumed: OpenRouter served gemini-2.5-flash while
	// Google's own endpoint was refusing every request. One meter each.
	google := port.EndpointName("https://generativelanguage.googleapis.com/v1beta/openai/chat/completions")
	openrouter := port.EndpointName("https://openrouter.ai/api/v1/chat/completions")
	if google == openrouter {
		t.Fatalf("both resolved to %q", google)
	}
	if google != geminiPortName {
		t.Errorf("google port = %q, want %q — detection and transcription must share it", google, geminiPortName)
	}
}

func TestChatFailureClassification(t *testing.T) {
	t.Parallel()
	quota := []byte(`{"error":{"code":429,"message":"Quota exceeded. Please retry in 58.8s."}}`)

	got := chatFailure(http.StatusTooManyRequests, quota)
	if got.Verdict != port.RateLimited {
		t.Errorf("429 verdict = %v", got.Verdict)
	}
	if got.RetryAfter < 59*time.Second {
		t.Errorf("retry hint lost: %v", got.RetryAfter)
	}
	if got.Daily {
		t.Error("a per-minute limit was read as daily")
	}

	daily := []byte(`{"error":{"message":"Quota exceeded for requests per day"}}`)
	if !chatFailure(http.StatusTooManyRequests, daily).Daily {
		t.Error("daily quota not recognised")
	}
	if v := chatFailure(http.StatusNotFound, nil).Verdict; v != port.ModelGone {
		t.Errorf("404 verdict = %v, want ModelGone", v)
	}
	if v := chatFailure(http.StatusBadGateway, nil).Verdict; v != port.Overloaded {
		t.Errorf("502 verdict = %v, want Overloaded", v)
	}
	// A bad request is not survivable by another model, so it must not spend
	// the chain.
	if v := chatFailure(http.StatusBadRequest, nil).Verdict; v != port.Fatal {
		t.Errorf("400 verdict = %v, want Fatal", v)
	}
}

func TestSummariseErrorBody(t *testing.T) {
	t.Parallel()
	// The real shape: 39 lines of JSON whose one useful sentence was buried.
	body := []byte(`{
  "error": {
    "code": 429,
    "message": "You exceeded your current quota.\nPlease retry in 58.8s.",
    "status": "RESOURCE_EXHAUSTED",
    "details": [{"@type": "type.googleapis.com/google.rpc.Help"}]
  }
}`)
	got := summariseErrorBody(body)
	if strings.Contains(got, "@type") || strings.Contains(got, "\n") {
		t.Errorf("body not summarised: %q", got)
	}
	if !strings.Contains(got, "retry in 58.8s") {
		t.Errorf("the useful part was dropped: %q", got)
	}
	// Some gateways wrap the object in an array.
	if got := summariseErrorBody([]byte(`[{"error":{"message":"wrapped"}}]`)); got != "wrapped" {
		t.Errorf("array-wrapped error = %q", got)
	}
	if got := summariseErrorBody(nil); got == "" {
		t.Error("empty body produced an empty message")
	}
	if got := summariseErrorBody([]byte("plain text failure")); got != "plain text failure" {
		t.Errorf("non-JSON body = %q", got)
	}
}

func TestKeywordExtractionIsBounded(t *testing.T) {
	t.Parallel()
	// Keywords only sharpen the whisper prompt, so this must never be what
	// stalls a run. With the endpoint refusing, the retries and the model
	// chain previously left pod waiting for minutes with nothing started.
	if KeywordExtractionBudget > time.Minute {
		t.Errorf("budget %v is too long for an optional step", KeywordExtractionBudget)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"service unavailable"}}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// The backoff is already zeroed for this package in TestMain. Setting it
	// here and restoring it on return would reset the global out from under
	// every other parallel test in the package, which cost the suite a minute.
	start := time.Now()
	got := ExtractKeywordsLLM("some text", types.LLMProfile{URL: srv.URL, Model: "m"}, "", true)
	elapsed := time.Since(start)

	if got != "" {
		t.Errorf("expected no keywords from a failing endpoint, got %q", got)
	}
	// The point is that it returns at all, and returns empty rather than
	// failing the transcription that depends on it.
	if elapsed > KeywordExtractionBudget+5*time.Second {
		t.Errorf("took %v, past the %v budget", elapsed, KeywordExtractionBudget)
	}
}
