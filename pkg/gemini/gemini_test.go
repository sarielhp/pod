package gemini

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/vertexai/genai"
	"pod/pkg/types"
)

func TestGeminiPromptContent(t *testing.T) {
	t.Parallel()
	if len(types.GeminiAdRemovalPrompt) == 0 {
		t.Fatal("expected GeminiAdRemovalPrompt to be non-empty")
	}
	for _, expected := range []string{"advertisement", "music_interlude", "intro_outro", "cuts", "segments", "חסויות", "קודי קופון"} {
		if !strings.Contains(types.GeminiAdRemovalPrompt, expected) {
			t.Errorf("expected prompt to contain %q", expected)
		}
	}
}

func TestParseGeminiJSONStringValid(t *testing.T) {
	t.Parallel()
	rawJSON := `{
		"cuts": [
			{"start": 10.5, "end": 45.0, "type": "advertisement", "reason": "Sponsor Wolt"},
			{"start": 120.0, "end": 135.0, "type": "music_interlude", "reason": "Transition"}
		],
		"segments": [
			{"start": 0.0, "end": 10.5, "text": "Hello and welcome"},
			{"start": 45.0, "end": 60.0, "text": "ברוכים הבאים לפרק"}
		]
	}`

	payload, err := ParseGeminiJSONString(rawJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing valid JSON: %v", err)
	}
	if len(payload.Cuts) != 2 {
		t.Fatalf("expected 2 cuts, got %d", len(payload.Cuts))
	}
	if payload.Cuts[0].Start != 10.5 || payload.Cuts[0].Type != "advertisement" {
		t.Errorf("unexpected cut 0: %+v", payload.Cuts[0])
	}
	if len(payload.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(payload.Segments))
	}
	if payload.Segments[1].Text != "ברוכים הבאים לפרק" {
		t.Errorf("unexpected segment text: %s", payload.Segments[1].Text)
	}
}

func TestParseGeminiJSONStringMarkdownBlocks(t *testing.T) {
	t.Parallel()
	rawJSON := "```json\n{\n  \"cuts\": [],\n  \"segments\": [{\"start\": 1.0, \"end\": 2.0, \"text\": \"test\"}]\n}\n```"
	payload, err := ParseGeminiJSONString(rawJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload.Segments) != 1 || payload.Segments[0].Text != "test" {
		t.Errorf("unexpected payload: %+v", payload)
	}

	rawJSON2 := "```\n{\n  \"cuts\": [],\n  \"segments\": []\n}\n```"
	payload2, err := ParseGeminiJSONString(rawJSON2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload2.Cuts) != 0 || len(payload2.Segments) != 0 {
		t.Errorf("unexpected payload: %+v", payload2)
	}
}

func TestParseGeminiJSONStringErrors(t *testing.T) {
	t.Parallel()
	for _, invalid := range []string{"", "   \t  ", "{not-valid-json}", "random text"} {
		_, err := ParseGeminiJSONString(invalid)
		if err == nil {
			t.Errorf("expected error for input %q, got nil", invalid)
		}
	}
}

func TestParseGeminiContentResponse(t *testing.T) {
	t.Parallel()
	if _, err := ParseGeminiContentResponse(nil); err == nil {
		t.Error("expected error for nil response")
	}

	emptyCandidates := &genai.GenerateContentResponse{}
	if _, err := ParseGeminiContentResponse(emptyCandidates); err == nil {
		t.Error("expected error for empty candidates")
	}

	nilContent := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{Content: nil}},
	}
	if _, err := ParseGeminiContentResponse(nilContent); err == nil {
		t.Error("expected error for nil candidate content")
	}

	validResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text(`{"cuts":[{"start":1.0,"end":5.0,"type":"ad","reason":"sponsor"}],"segments":[{"start":5.0,"end":10.0,"text":"content"}]}`),
					},
				},
			},
		},
	}
	payload, err := ParseGeminiContentResponse(validResp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload.Cuts) != 1 || payload.Cuts[0].Start != 1.0 {
		t.Errorf("unexpected payload cuts: %+v", payload.Cuts)
	}
}

func TestConvertGeminiToAbsTypes(t *testing.T) {
	t.Parallel()
	tdEmpty, adsEmpty := ConvertGeminiToAbsTypes(nil)
	if tdEmpty == nil || len(adsEmpty) != 0 {
		t.Errorf("expected empty result for nil payload, got td=%v ads=%v", tdEmpty, adsEmpty)
	}

	payload := &types.GeminiResponsePayload{
		Cuts: []types.GeminiCutItem{
			{Start: 12.0, End: 30.5, Type: "advertisement", Reason: "Wolt plug"},
			{Start: 50.0, End: 60.0, Type: "music_interlude", Reason: ""},
			{Start: 70.0, End: 80.0, Type: "", Reason: "Generic ad"},
		},
		Segments: []types.GeminiSegmentItem{
			{Start: 0.0, End: 12.0, Text: "Intro words."},
			{Start: 30.5, End: 50.0, Text: "Main discussion."},
		},
	}

	td, ads := ConvertGeminiToAbsTypes(payload)
	if len(td.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(td.Segments))
	}
	if td.Segments[0].Start != 0.0 || td.Segments[0].End != 12.0 || td.Segments[0].Text != "Intro words." {
		t.Errorf("unexpected segment 0: %+v", td.Segments[0])
	}
	if td.Text != "Intro words. Main discussion." {
		t.Errorf("unexpected full text: %q", td.Text)
	}

	if len(ads) != 3 {
		t.Fatalf("expected 3 ads, got %d", len(ads))
	}
	if ads[0].Reason != "[advertisement] Wolt plug" {
		t.Errorf("unexpected ad 0 reason: %q", ads[0].Reason)
	}
	if ads[1].Reason != "[music_interlude]" {
		t.Errorf("unexpected ad 1 reason: %q", ads[1].Reason)
	}
	if ads[2].Reason != "Generic ad" {
		t.Errorf("unexpected ad 2 reason: %q", ads[2].Reason)
	}
}

func TestComputeGeminiChunks(t *testing.T) {
	t.Parallel()
	c1 := ComputeGeminiChunks(1200.0, 1800.0)
	if len(c1) != 1 || c1[0].StartSec != 0 || c1[0].DurSec != 1200.0 {
		t.Errorf("unexpected chunks for 1200s: %+v", c1)
	}

	c2 := ComputeGeminiChunks(1800.0, 1800.0)
	if len(c2) != 1 || c2[0].StartSec != 0 || c2[0].DurSec != 1800.0 {
		t.Errorf("unexpected chunks for 1800s: %+v", c2)
	}

	c3 := ComputeGeminiChunks(3700.0, 1800.0)
	if len(c3) != 3 {
		t.Fatalf("expected 3 chunks for 3700s, got %d", len(c3))
	}
	if c3[0].StartSec != 0 || c3[0].DurSec != 1800.0 {
		t.Errorf("unexpected chunk 0: %+v", c3[0])
	}
	if c3[1].StartSec != 1800.0 || c3[1].DurSec != 1800.0 {
		t.Errorf("unexpected chunk 1: %+v", c3[1])
	}
	if c3[2].StartSec != 3600.0 || c3[2].DurSec != 100.0 {
		t.Errorf("unexpected chunk 2: %+v", c3[2])
	}

	// A zero chunk length means "use the default", whatever it currently is.
	// Deriving the expectation from the constant keeps this test about the
	// fallback rather than about the value.
	total := DefaultGeminiChunkSec*2 + 200
	c4 := ComputeGeminiChunks(total, 0)
	if len(c4) != 3 {
		t.Fatalf("expected 3 chunks for %.0fs at the default chunk size, got %d", total, len(c4))
	}
	if c4[0].DurSec != DefaultGeminiChunkSec {
		t.Errorf("default chunk length not applied: %+v", c4[0])
	}
}

func TestMergeGeminiChunkResults(t *testing.T) {
	t.Parallel()
	r1 := &types.GeminiChunkResult{
		Index:    0,
		StartSec: 0.0,
		Payload: &types.GeminiResponsePayload{
			Cuts:     []types.GeminiCutItem{{Start: 10.0, End: 30.0, Type: "advertisement", Reason: "Ad 1"}},
			Segments: []types.GeminiSegmentItem{{Start: 0.0, End: 50.0, Text: "Part 1."}},
		},
	}
	r2 := &types.GeminiChunkResult{
		Index:    1,
		StartSec: 1800.0,
		Payload: &types.GeminiResponsePayload{
			Cuts:     []types.GeminiCutItem{{Start: 5.0, End: 25.0, Type: "music_interlude", Reason: "Music"}},
			Segments: []types.GeminiSegmentItem{{Start: 0.0, End: 40.0, Text: "Part 2."}},
		},
	}

	merged := MergeGeminiChunkResults([]*types.GeminiChunkResult{r1, r2, nil})
	if len(merged.Cuts) != 2 {
		t.Fatalf("expected 2 cuts, got %d", len(merged.Cuts))
	}
	if merged.Cuts[0].Start != 10.0 || merged.Cuts[0].End != 30.0 {
		t.Errorf("unexpected cut 0: %+v", merged.Cuts[0])
	}
	if merged.Cuts[1].Start != 1805.0 || merged.Cuts[1].End != 1825.0 {
		t.Errorf("unexpected cut 1 with offset: %+v", merged.Cuts[1])
	}

	if len(merged.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(merged.Segments))
	}
	if merged.Segments[1].Start != 1800.0 || merged.Segments[1].End != 1840.0 {
		t.Errorf("unexpected segment 1 with offset: %+v", merged.Segments[1])
	}
}

func TestPrepareGeminiChunksSingle(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mockAudio := filepath.Join(tempDir, "ep.mp3")
	chunks := []types.GeminiChunkInfo{{Index: 0, StartSec: 0, DurSec: 600.0}}
	prepared, cleanup, err := PrepareGeminiChunks(mockAudio, chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	if len(prepared) != 1 || prepared[0].FilePath != mockAudio {
		t.Errorf("expected single chunk to point directly to original file, got %+v", prepared)
	}
}

func TestSplitAudioChunkInvalidFile(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	workDir := filepath.Join(tempDir, ".work")
	_ = os.MkdirAll(workDir, 0755)
	outChunk := filepath.Join(workDir, "chunk.mp3")
	err := SplitAudioChunk(filepath.Join(tempDir, "missing.mp3"), outChunk, 0, 10)
	if err == nil {
		t.Error("expected error when splitting missing file, got nil")
	}
}

func TestParseGeminiStudioResponse(t *testing.T) {
	t.Parallel()
	validBody := []byte(`{
		"candidates": [
			{
				"content": {
					"parts": [
						{"text": "{\"cuts\": [{\"start\": 5.0, \"end\": 15.0, \"type\": \"ad\", \"reason\": \"sponsor\"}], \"segments\": [{\"start\": 15.0, \"end\": 30.0, \"text\": \"hello\"}]}"}
					]
				}
			}
		]
	}`)
	payload, err := ParseGeminiStudioResponse(validBody)
	if err != nil {
		t.Fatalf("unexpected error parsing studio response: %v", err)
	}
	if len(payload.Cuts) != 1 || payload.Cuts[0].Start != 5.0 {
		t.Errorf("unexpected cuts: %+v", payload.Cuts)
	}
	if len(payload.Segments) != 1 || payload.Segments[0].Text != "hello" {
		t.Errorf("unexpected segments: %+v", payload.Segments)
	}

	invalidBody := []byte(`{"candidates": []}`)
	if _, err := ParseGeminiStudioResponse(invalidBody); err == nil {
		t.Error("expected error for empty candidates in studio response")
	}
}

func TestDeleteGeminiStudioFileNoop(t *testing.T) {
	t.Parallel()
	DeleteGeminiStudioFile(context.Background(), "", "")
	DeleteGeminiStudioFile(context.Background(), "key", "")
}

func TestGeminiStudioNoKeyInURLError(t *testing.T) {
	t.Parallel()
	apiKey := "AIzaSySuperSecretKey12345"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "test.mp3")
	_ = os.WriteFile(audioPath, []byte("audio"), 0644)

	_, _, err := UploadAudioToGeminiStudio(ctx, apiKey, audioPath)
	if err == nil {
		t.Fatal("expected error with canceled context, got nil")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Errorf("error leaked apiKey: %v", err)
	}

	_, err = CallGeminiStudioProcessor(ctx, apiKey, "gemini-1.5-flash", "files/test")
	if err == nil {
		t.Fatal("expected error with canceled context, got nil")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Errorf("error leaked apiKey: %v", err)
	}
}

func TestFormatGeminiErrorBody(t *testing.T) {
	t.Parallel()
	jsonErr := []byte(`{"error":{"code":503,"message":"Model high demand","status":"UNAVAILABLE"}}`)
	got := FormatGeminiErrorBody(jsonErr)
	expected := "Model high demand (UNAVAILABLE)"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	dailyErr := []byte(`{
		"error": {
			"code": 429,
			"message": "You exceeded your current quota, please check your plan and billing details.",
			"status": "RESOURCE_EXHAUSTED",
			"details": [{
				"metadata": {
					"consumer": "projects/10394829",
					"quota_limit": "GenerateContentRequestsPerDayPerProjectPerRegion",
					"quota_limit_value": "1500"
				}
			}]
		}
	}`)
	gotDaily := FormatGeminiErrorBody(dailyErr)
	if !strings.Contains(gotDaily, "Free Tier: Daily quota exhausted") || !strings.Contains(gotDaily, "Project: 10394829") {
		t.Errorf("expected daily quota details in formatted error, got: %s", gotDaily)
	}
	if !IsGeminiDailyQuotaExhausted(dailyErr) {
		t.Errorf("expected IsGeminiDailyQuotaExhausted to be true")
	}

	rateLimitErr := []byte(`{
		"error": {
			"code": 429,
			"message": "Rate limit exceeded",
			"status": "RESOURCE_EXHAUSTED",
			"details": [{
				"metadata": {
					"consumer": "projects/999",
					"quota_limit": "GenerateContentRequestsPerMinutePerProjectPerRegion",
					"quota_limit_value": "15"
				}
			}]
		}
	}`)
	gotRate := FormatGeminiErrorBody(rateLimitErr)
	if !strings.Contains(gotRate, "Free Tier: Rate limit exceeded (15 req/min limit)") {
		t.Errorf("expected rate limit details in formatted error, got: %s", gotRate)
	}
	if IsGeminiDailyQuotaExhausted(rateLimitErr) {
		t.Errorf("expected IsGeminiDailyQuotaExhausted to be false for rate limit error")
	}

	plainErr := []byte("plain error message")
	gotPlain := FormatGeminiErrorBody(plainErr)
	if gotPlain != "plain error message" {
		t.Errorf("expected %q, got %q", "plain error message", gotPlain)
	}
}

func TestGeminiUploadAudioToGCSNonExistentFile(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "missing.mp3")
	ctx := context.Background()
	_, err := UploadAudioToGCS(ctx, "test-bucket", nonExistent)
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestGeminiDeleteGCSObjectEmptyPrefix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	DeleteGCSObject(ctx, "test-bucket", "")
	DeleteGCSObject(ctx, "test-bucket", "short")
}

func TestProcessGeminiChunksParallelCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	chunks := []types.GeminiChunkInfo{
		{Index: 0, FilePath: "test.mp3", StartSec: 0, DurSec: 100},
		{Index: 1, FilePath: "test.mp3", StartSec: 100, DurSec: 100},
	}
	_, err := ProcessGeminiChunksParallel(ctx, chunks, types.Config{})
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
}

func TestExtractGeminiRetryDelay(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"error": {
			"code": 429,
			"message": "You exceeded your current quota, please check your plan and billing details.\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 20, model: gemini-3.8-flash\nPlease retry in 9.229033412s.",
			"status": "RESOURCE_EXHAUSTED"
		}
	}`)
	delay := ExtractGeminiRetryDelay(body)
	if delay < 10*time.Second || delay > 11*time.Second {
		t.Errorf("expected ~10.2s delay, got %v", delay)
	}

	delayWithUnit := ExtractGeminiRetryDelay([]byte("Please retry in 15s."))
	if delayWithUnit != 16*time.Second {
		t.Errorf("expected 16s delay, got %v", delayWithUnit)
	}

	delayMinuteUnit := ExtractGeminiRetryDelay([]byte("please retry in 2m"))
	if delayMinuteUnit != 121*time.Second {
		t.Errorf("expected 121s delay, got %v", delayMinuteUnit)
	}

	delayNone := ExtractGeminiRetryDelay([]byte("random error"))
	if delayNone != 0 {
		t.Errorf("expected 0 delay, got %v", delayNone)
	}
}

func TestIsGeminiDailyQuotaExhaustedWithRetryIn(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"error": {
			"code": 429,
			"message": "You exceeded your current quota, please check your plan and billing details.\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 20, model: gemini-3.8-flash\nPlease retry in 9.229033412s.",
			"status": "RESOURCE_EXHAUSTED"
		}
	}`)
	if IsGeminiDailyQuotaExhausted(body) {
		t.Errorf("expected IsGeminiDailyQuotaExhausted to be false when retry-in is present")
	}
}
