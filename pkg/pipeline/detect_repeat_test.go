package pipeline

import (
	"math"
	"testing"

	"pod/pkg/types"
)

func runWith(pairs ...float64) DetectResult {
	var segs []types.AdSegment
	for i := 0; i+1 < len(pairs); i += 2 {
		segs = append(segs, types.AdSegment{Start: pairs[i], End: pairs[i+1]})
	}
	return DetectResult{Segments: segs}
}

func TestSummarizeDetectRuns(t *testing.T) {
	t.Parallel()

	t.Run("no runs", func(t *testing.T) {
		t.Parallel()
		if got := SummarizeDetectRuns(nil); got.Runs != 0 || got.Deterministic() {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("a single run cannot demonstrate determinism", func(t *testing.T) {
		t.Parallel()
		s := SummarizeDetectRuns([]DetectResult{runWith(0, 10)})
		if s.Runs != 1 || s.Identical != 1 {
			t.Errorf("got %+v", s)
		}
		if s.Deterministic() {
			t.Error("one run reported as deterministic")
		}
	})

	t.Run("identical runs", func(t *testing.T) {
		t.Parallel()
		s := SummarizeDetectRuns([]DetectResult{runWith(0, 10), runWith(0, 10), runWith(0, 10)})
		if !s.Deterministic() {
			t.Errorf("got %+v", s)
		}
		if s.Identical != 3 || s.MinAgreement != 1 {
			t.Errorf("got %+v", s)
		}
	})

	t.Run("one divergent run drags down the minimum but not the mean", func(t *testing.T) {
		t.Parallel()
		// Two runs agree exactly; the third shares nothing. Pairs: 1, 0, 0.
		s := SummarizeDetectRuns([]DetectResult{runWith(0, 10), runWith(0, 10), runWith(50, 60)})
		if s.MinAgreement != 0 {
			t.Errorf("min = %v, want 0", s.MinAgreement)
		}
		if math.Abs(s.MeanAgreement-1.0/3.0) > 1e-9 {
			t.Errorf("mean = %v, want 1/3", s.MeanAgreement)
		}
		if s.Identical != 2 {
			t.Errorf("identical = %d, want 2", s.Identical)
		}
		if s.Deterministic() {
			t.Error("reported deterministic despite a divergent run")
		}
	})

	t.Run("counts and ad time are recorded per run", func(t *testing.T) {
		t.Parallel()
		s := SummarizeDetectRuns([]DetectResult{runWith(0, 10, 20, 25), runWith(0, 10)})
		if len(s.SegmentCounts) != 2 || s.SegmentCounts[0] != 2 || s.SegmentCounts[1] != 1 {
			t.Errorf("counts = %v", s.SegmentCounts)
		}
		if s.AdTimes[0] != 15 || s.AdTimes[1] != 10 {
			t.Errorf("ad times = %v", s.AdTimes)
		}
	})

	t.Run("runs finding nothing agree", func(t *testing.T) {
		t.Parallel()
		s := SummarizeDetectRuns([]DetectResult{runWith(), runWith()})
		if !s.Deterministic() {
			t.Errorf("got %+v", s)
		}
	})
}
