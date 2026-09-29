package format

import (
	"path/filepath"
	"testing"

	"pod/pkg/types"
)

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
