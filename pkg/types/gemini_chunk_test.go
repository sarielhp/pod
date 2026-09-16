package types

import "testing"

func TestGeminiChunkSec(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		geminiChunk  int
		whisperChunk int
		wantPlain    float64
		wantCapped   float64
	}{
		{"unset uses the safe default", 0, 0, DefaultGeminiChunkSec, DefaultGeminiChunkSec},
		{"explicit setting wins", 600, 0, 600, 600},
		{"shorter whisper chunk is honoured", 0, 300, DefaultGeminiChunkSec, 300},
		{"longer whisper chunk is ignored", 0, 1800, DefaultGeminiChunkSec, DefaultGeminiChunkSec},
		{"whisper chunk cannot exceed an explicit setting", 600, 1800, 600, 600},
		{"negative values fall back", -1, -1, DefaultGeminiChunkSec, DefaultGeminiChunkSec},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{}
			cfg.GeminiChunkSec = tc.geminiChunk
			if got := cfg.GetGeminiChunkSec(); got != tc.wantPlain {
				t.Errorf("GetGeminiChunkSec() = %v, want %v", got, tc.wantPlain)
			}
			if got := cfg.GeminiChunkSecCapped(tc.whisperChunk); got != tc.wantCapped {
				t.Errorf("GeminiChunkSecCapped(%d) = %v, want %v", tc.whisperChunk, got, tc.wantCapped)
			}
		})
	}
}

// A chunk of 1800s makes Gemini return an empty candidate with blockReason
// "OTHER": the verbatim transcript does not fit in one response. 1200s was
// measured to work, so the default must stay below it.
func TestDefaultGeminiChunkSecStaysBelowTheMeasuredCeiling(t *testing.T) {
	t.Parallel()
	if DefaultGeminiChunkSec > 1200 {
		t.Errorf("DefaultGeminiChunkSec = %v, above the 1200s that was measured to answer", DefaultGeminiChunkSec)
	}
}
