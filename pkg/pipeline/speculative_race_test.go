package pipeline

import (
	"context"
	"errors"
	"testing"

	"pod/pkg/transcribe"
	"pod/pkg/types"
)

func TestAwaitRaceResultsGeminiWins(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan SpeculativeCandidateResult, 2)
	resultCh <- SpeculativeCandidateResult{
		ServiceName: "Gemini",
		TD:          &types.TranscriptionData{Text: "Gemini text"},
		Ads:         []types.AdSegment{{Start: 10, End: 20, Reason: "Sponsor"}},
		IsGemini:    true,
	}

	td, ads, geminiWon, err := awaitRaceResults(ctx, cancel, resultCh, 2, true)
	if err != nil || !geminiWon {
		t.Fatalf("expected Gemini to win: err=%v, geminiWon=%v", err, geminiWon)
	}
	if td.Text != "Gemini text" || len(ads) != 1 {
		t.Errorf("unexpected race payload: td=%+v, ads=%+v", td, ads)
	}
}

func TestAwaitRaceResultsGemini503FallbackToLocal(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan SpeculativeCandidateResult, 2)
	resultCh <- SpeculativeCandidateResult{
		ServiceName: "Gemini",
		Err:         errors.New("chunk 0 studio processing failed:\n   gemini studio generateContent HTTP 503: high demand"),
		IsGemini:    true,
	}
	resultCh <- SpeculativeCandidateResult{
		ServiceName: "whisper-gpu",
		TD:          &types.TranscriptionData{Text: "Local text"},
		IsGemini:    false,
	}

	td, ads, geminiWon, err := awaitRaceResults(ctx, cancel, resultCh, 2, true)
	if err != nil || geminiWon {
		t.Fatalf("expected local to win after Gemini 503: err=%v, geminiWon=%v", err, geminiWon)
	}
	if td.Text != "Local text" || len(ads) != 0 {
		t.Errorf("unexpected local winner payload: td=%+v, ads=%+v", td, ads)
	}
}

func TestResolveSpeculativeRacersDefaultDisabled(t *testing.T) {
	t.Parallel()
	cfg := types.Config{
		WhisperConfig: types.WhisperConfig{
			WhisperURL: "http://127.0.0.1:8088/inference",
			WhisperProfiles: []types.WhisperProfile{
				{ID: 1, Name: "whisper-local", Engine: types.WhisperEngineLocal, CliBinary: "whisper-cli"},
			},
		},
	}
	opts := types.ProcOptions{}
	if cfg.IsSpeculativeTranscriptionEnabled() {
		t.Fatalf("expected speculative transcription to be disabled by default")
	}
	racers := ResolveSpeculativeRacers(cfg, opts, "en")
	if len(racers) != 0 {
		t.Errorf("expected 0 racers when speculative transcription is disabled, got %+v", racers)
	}
}

func TestResolveSpeculativeRacersCustomServices(t *testing.T) {
	t.Parallel()
	origUsable := transcribe.WhisperProfileUsable
	transcribe.WhisperProfileUsable = func(wp types.WhisperProfile) bool { return true }
	t.Cleanup(func() { transcribe.WhisperProfileUsable = origUsable })

	enabled := true
	cfg := types.Config{
		SpeculativeConfig: types.SpeculativeConfig{
			SpeculativeTranscription: &enabled,
			CompetingServices:        []string{"local", "docker"},
		},
		WhisperConfig: types.WhisperConfig{
			WhisperProfiles: []types.WhisperProfile{
				{ID: 1, Name: "local-cli", Engine: types.WhisperEngineLocal, CliBinary: "whisper-cli"},
				{ID: 2, Name: "docker-server", Engine: types.WhisperEngineDocker, URL: "http://127.0.0.1:8088/inference"},
			},
		},
	}
	opts := types.ProcOptions{}
	racers := ResolveSpeculativeRacers(cfg, opts, "en")
	if len(racers) != 2 {
		t.Fatalf("expected 2 racers for local and docker, got %d", len(racers))
	}
	if racers[0].Name != "local-cli" || racers[1].Name != "docker-server" {
		t.Errorf("unexpected racers: %+v", racers)
	}
}
