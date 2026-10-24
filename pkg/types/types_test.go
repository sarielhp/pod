package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAdSegmentJSON(t *testing.T) {
	t.Parallel()
	seg := AdSegment{
		Start:  10.5,
		End:    30.2,
		Reason: "sponsor plug",
	}
	data, err := json.Marshal(seg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed AdSegment
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Start != 10.5 || parsed.End != 30.2 || parsed.Reason != "sponsor plug" {
		t.Errorf("Unexpected unmarshaled segment: %+v", parsed)
	}
}

func TestConfigFlags(t *testing.T) {
	t.Parallel()
	var cfg Config
	if !cfg.IsGeminiAPIKeyEnabled() {
		t.Error("expected default GeminiAPIKeyEnabled to be true")
	}
	if !cfg.IsOpenRouterAPIKeyEnabled() {
		t.Error("expected default OpenRouterAPIKeyEnabled to be true")
	}
	if cfg.IsSpeculativeTranscriptionEnabled() {
		t.Error("expected default SpeculativeTranscription to be false")
	}
	defaultSvcs := cfg.GetCompetingServices()
	if len(defaultSvcs) != 2 || defaultSvcs[0] != "gemini" || defaultSvcs[1] != "whisper" {
		t.Errorf("unexpected default competing services: %v", defaultSvcs)
	}

	f := false
	tr := true
	cfg.GeminiAPIKeyEnabled = &f
	cfg.OpenRouterAPIKeyEnabled = &tr
	cfg.SpeculativeTranscription = &tr
	cfg.CompetingServices = []string{"local", "docker"}

	if cfg.IsGeminiAPIKeyEnabled() {
		t.Error("expected GeminiAPIKeyEnabled to be false")
	}
	if !cfg.IsOpenRouterAPIKeyEnabled() {
		t.Error("expected OpenRouterAPIKeyEnabled to be true")
	}
	if !cfg.IsSpeculativeTranscriptionEnabled() {
		t.Error("expected SpeculativeTranscription to be true")
	}
	svcs := cfg.GetCompetingServices()
	if len(svcs) != 2 || svcs[0] != "local" || svcs[1] != "docker" {
		t.Errorf("unexpected competing services: %v", svcs)
	}

	cfg.SpeculativeTranscription = &f
	if cfg.IsSpeculativeTranscriptionEnabled() {
		t.Error("expected SpeculativeTranscription to be false when explicitly disabled")
	}
}

func TestEpisodeAudioMetaContentHashRoundTrip(t *testing.T) {
	t.Parallel()

	st := EpisodeStatusFile{
		MediaFile: "ep.mp3",
		Cleaned:   EpisodeAudioMeta{Filename: "ep.mp3", ContentHash: "0123456789abcdef"},
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var parsed EpisodeStatusFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if parsed.Cleaned.ContentHash != "0123456789abcdef" {
		t.Errorf("content_hash = %q, want 0123456789abcdef", parsed.Cleaned.ContentHash)
	}

	// A record written before hashing existed omits the field and must still
	// parse, and an empty hash must not be serialised into new records.
	var legacy EpisodeStatusFile
	if err := json.Unmarshal([]byte(`{"media_file":"ep.mp3","cleaned":{"filename":"ep.mp3"}}`), &legacy); err != nil {
		t.Fatalf("Unmarshal legacy failed: %v", err)
	}
	if legacy.Cleaned.ContentHash != "" {
		t.Errorf("legacy content_hash = %q, want empty", legacy.Cleaned.ContentHash)
	}
	empty, err := json.Marshal(EpisodeStatusFile{MediaFile: "ep.mp3"})
	if err != nil {
		t.Fatalf("Marshal empty failed: %v", err)
	}
	if strings.Contains(string(empty), "content_hash") {
		t.Errorf("empty content_hash should be omitted: %s", empty)
	}
}

func TestEpisodeStatusFileFavorite(t *testing.T) {
	t.Parallel()
	st := EpisodeStatusFile{}
	if st.IsFavorite() {
		t.Errorf("expected initially not favorite")
	}

	st.SetFavorite(true)
	if !st.IsFavorite() || !st.Favorite {
		t.Errorf("expected favorite to be true")
	}

	data, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed EpisodeStatusFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !parsed.IsFavorite() || !parsed.Favorite {
		t.Errorf("expected parsed status file to be favorite")
	}

	jsonUS := []byte(`{"media_file":"test.mp3","favorite":true}`)
	var parsedUS EpisodeStatusFile
	if err := json.Unmarshal(jsonUS, &parsedUS); err != nil {
		t.Fatalf("Unmarshal US failed: %v", err)
	}
	if !parsedUS.IsFavorite() || !parsedUS.Favorite {
		t.Errorf("expected favorite:true in JSON to set favorite")
	}

	st.SetFavorite(false)
	if st.IsFavorite() || st.Favorite {
		t.Errorf("expected SetFavorite(false) to clear favorite")
	}
}
