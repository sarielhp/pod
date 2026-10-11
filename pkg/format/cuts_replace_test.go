package format

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/types"
)

func TestSavedCutsPreserveGapsAfterMergingOldAndNewCuts(t *testing.T) {
	t.Parallel()
	for _, replace := range []bool{false, true} {
		mp3 := filepath.Join(t.TempDir(), "ep.mp3")
		if result := SaveCutsJSON(mp3, 100, []types.AdSegment{{Start: 0, End: 100, Reason: "old ad"}}, nil, true); result.Err != nil {
			t.Fatal(result.Err)
		}
		gap := types.KeepSegment{Start: 40.004, End: 42.006}
		fresh := []types.AdSegment{{Start: 0, End: 40, Reason: "before"}, {Start: 42, End: 100, Reason: "after"}, {Start: 41, End: 41.5, Reason: "inside gap"}}
		for attempt := 0; attempt < 2; attempt++ {
			result := SaveDetectedCutsJSON(mp3, 100, fresh, nil, true, replace, gap)
			if result.Err != nil || math.Abs(keptSeconds(result.KeepSegments)-2.01) > 0.001 {
				t.Fatalf("replace=%v kept=%+v err=%v", replace, result.KeepSegments, result.Err)
			}
			var saved types.CutsData
			data, err := os.ReadFile(result.CutsFile)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			for _, cut := range saved.MergedCutIntervals {
				if cut.Start < gap.End && cut.End > gap.Start {
					t.Fatalf("merged cut crosses preserved gap: %+v", cut)
				}
			}
			for _, cut := range saved.CutIntervals {
				if cut.StartSec < gap.End && cut.EndSec > gap.Start {
					t.Fatalf("raw cut crosses preserved gap: %+v", cut)
				}
			}
			if len(saved.KeepIntervals) != 1 || saved.KeepIntervals[0].Start > gap.Start || saved.KeepIntervals[0].End < gap.End {
				t.Fatalf("cached keep intervals lost gap: %+v", saved.KeepIntervals)
			}
		}
	}
}

func TestSaveDetectedCutsJSONReplaceDiscardsEarlierCuts(t *testing.T) {
	t.Parallel()
	mp3 := filepath.Join(t.TempDir(), "ep.mp3")
	bad := []types.AdSegment{{Start: 3, End: 340, Reason: "wrongly huge"}}
	good := []types.AdSegment{{Start: 30, End: 70, Reason: "real plug"}}

	SaveCutsJSON(mp3, 1000, bad, nil, true)

	merged := SaveDetectedCutsJSON(mp3, 1000, good, nil, true, false)
	if kept := keptSeconds(merged.KeepSegments); kept > 700 {
		t.Fatalf("without replace the earlier bad cut must survive, kept %.0fs", kept)
	}

	replaced := SaveDetectedCutsJSON(mp3, 1000, good, nil, true, true)
	if !replaced.Changed {
		t.Error("replacing a different cut set should report a change")
	}
	if kept := keptSeconds(replaced.KeepSegments); kept != 960 {
		t.Fatalf("with replace only the new cut should apply, kept %.0fs of 1000", kept)
	}
}

func keptSeconds(segs [][2]float64) float64 {
	total := 0.0
	for _, s := range segs {
		total += s[1] - s[0]
	}
	return total
}
