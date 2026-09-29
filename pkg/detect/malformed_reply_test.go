package detect

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pod/pkg/types"
)

const transcriptLineReply = `[95.4s -> 147.1s, "Host read sponsor plug for Acme"]`

func TestDetectAdsReasksAfterTranscriptFormatReply(t *testing.T) {
	t.Parallel()
	srv, calls := adStubServer(t, transcriptLineReply, `[{"start": 95.4, "end": 147.1, "reason": "Acme plug"}]`)
	profile := types.LLMProfile{Name: "stub", Type: "openrouter", URL: srv.URL, Model: "stub"}

	segs, err := DetectAdsLLMTimeout("transcript", profile, "key", 5*time.Second)
	if err != nil || len(segs) != 1 || segs[0].Start != 95.4 {
		t.Fatalf("expected the re-ask to recover the segment, got %+v (err=%v)", segs, err)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Errorf("expected 2 calls, got %d", got)
	}
}

func TestDetectAdsGivesUpOnPersistentlyMalformedReply(t *testing.T) {
	t.Parallel()
	srv, calls := adStubServer(t, transcriptLineReply)
	profile := types.LLMProfile{Name: "stub", Type: "openrouter", URL: srv.URL, Model: "stub"}

	_, err := DetectAdsLLMTimeout("transcript", profile, "key", 5*time.Second)
	if err == nil {
		t.Fatal("expected an error once the retries are spent")
	}
	if want := int32(1 + MalformedReplyRetries); atomic.LoadInt32(calls) != want {
		t.Errorf("expected %d calls, got %d", want, atomic.LoadInt32(calls))
	}
	if !strings.Contains(err.Error(), "95.4s") {
		t.Errorf("error should quote the offending text, got: %v", err)
	}
}

func TestDetectAdsReasksAfterImplausiblyLongSegment(t *testing.T) {
	t.Parallel()
	lumped := `[{"start": 95, "end": 3000, "reason": "one giant ad"}]`
	split := `[{"start": 95, "end": 147, "reason": "plug"}, {"start": 2900, "end": 2950, "reason": "plug"}]`
	srv, calls := adStubServer(t, lumped, split)
	profile := types.LLMProfile{Name: "stub", Type: "openrouter", URL: srv.URL, Model: "stub"}

	segs, err := DetectAdsLLMTimeout("transcript", profile, "key", 5*time.Second)
	if err != nil || len(segs) != 2 {
		t.Fatalf("expected the split answer, got %+v (err=%v)", segs, err)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Errorf("expected 2 calls, got %d", got)
	}
}

func TestDetectAdsReturnsLumpedAnswerWhenReaskDoesNotHelp(t *testing.T) {
	t.Parallel()
	srv, _ := adStubServer(t, `[{"start": 95, "end": 3000, "reason": "one giant ad"}]`)
	profile := types.LLMProfile{Name: "stub", Type: "openrouter", URL: srv.URL, Model: "stub"}

	segs, err := DetectAdsLLMTimeout("transcript", profile, "key", 5*time.Second)
	if err != nil || len(segs) != 1 {
		t.Fatalf("the answer should pass through for the cutter's own floor to judge, got %+v (err=%v)", segs, err)
	}
}

func TestExtractJSONArrayErrorsShowWhereTheReplyWentWrong(t *testing.T) {
	t.Parallel()
	_, err := ExtractJSONArray("Here you go: " + transcriptLineReply)
	if err == nil || !strings.Contains(err.Error(), "Host read sponsor") {
		t.Errorf("unmarshal error should quote the surrounding text, got: %v", err)
	}
	_, err = ExtractJSONArray(`[{"start": 1, "end": 2, "reason": "cut off`)
	if err == nil || !strings.Contains(err.Error(), "cut off") {
		t.Errorf("truncation error should show how the reply ended, got: %v", err)
	}
}
