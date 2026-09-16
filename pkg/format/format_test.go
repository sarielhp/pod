package format

import (
	"path/filepath"
	"testing"

	"pod/pkg/types"
)

func TestFormatClock(t *testing.T) {
	t.Parallel()
	if got := FormatClock(0); got != "00:00" {
		t.Errorf("FormatClock(0) = %q; want 00:00", got)
	}
	if got := FormatClock(65); got != "01:05" {
		t.Errorf("FormatClock(65) = %q; want 01:05", got)
	}
	if got := FormatClock(3665); got != "01:01:05" {
		t.Errorf("FormatClock(3665) = %q; want 01:01:05", got)
	}
}

func TestFormatSRTTime(t *testing.T) {
	t.Parallel()
	if got := FormatSRTTime(0); got != "00:00:00,000" {
		t.Errorf("FormatSRTTime(0) = %q; want 00:00:00,000", got)
	}
	if got := FormatSRTTime(3661.5); got != "01:01:01,500" {
		t.Errorf("FormatSRTTime(3661.5) = %q; want 01:01:01,500", got)
	}
}

func TestCalculateKeepSegments(t *testing.T) {
	t.Parallel()
	ads := []types.AdSegment{
		{Start: 10, End: 20},
		{Start: 30, End: 40},
	}
	keep := calculateKeepSegments(50, ads)
	if len(keep) != 3 {
		t.Fatalf("expected 3 keep segments, got %d", len(keep))
	}
	if keep[0] != [2]float64{0, 10} || keep[1] != [2]float64{20, 30} || keep[2] != [2]float64{40, 50} {
		t.Errorf("unexpected keep segments: %+v", keep)
	}
}

func TestSaveCutsJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	mainFile := filepath.Join(tmpDir, "podcast.mp3")

	ads := []types.AdSegment{
		{Start: 10.0, End: 20.0, Reason: "Sponsor A"},
	}

	res := SaveCutsJSON(mainFile, 100.0, ads, nil, true)
	if !res.Changed {
		t.Error("expected cuts result to be Changed=true on first save")
	}

	raw, merged, data := loadExistingCuts(res.CutsFile)
	if len(raw) != 1 || len(merged) != 1 || data == nil {
		t.Fatalf("expected loaded cuts to have 1 entry: raw=%d merged=%d", len(raw), len(merged))
	}

	// Saving again with same cuts should return Changed=false
	res2 := SaveCutsJSON(mainFile, 100.0, ads, nil, true)
	if res2.Changed {
		t.Error("expected unchanged cuts to return Changed=false")
	}
}
