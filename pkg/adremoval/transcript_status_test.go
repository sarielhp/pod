package adremoval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateTranscriptAdDetectionStatusReplacesAReadOnlyTranscript(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	dir := t.TempDir()
	jsonFile := filepath.Join(dir, "ep.transcript.json")
	if err := os.WriteFile(jsonFile, []byte(`{"text":"hello","segments":[]}`), 0444); err != nil {
		t.Fatal(err)
	}
	if err := updateTranscriptAdDetectionStatus(jsonFile, true, "done", "model-x", "", 3); err != nil {
		t.Fatalf("rewrite of a read-only transcript (the atomic rename path) failed: %v", err)
	}
	raw, err := os.ReadFile(jsonFile)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("transcript is not valid JSON after rewrite: %v", err)
	}
	if data["text"] != "hello" || data["ad_detection_status"] != "done" || data["ad_segments_count"] != float64(3) {
		t.Fatalf("rewritten transcript lost fields: %v", data)
	}
}
