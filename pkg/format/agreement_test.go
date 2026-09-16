package format

import (
	"math"
	"testing"

	"pod/pkg/types"
)

func seg(pairs ...float64) []types.AdSegment {
	var out []types.AdSegment
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, types.AdSegment{Start: pairs[i], End: pairs[i+1]})
	}
	return out
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestIntervalCoverageCountsOverlapOnce(t *testing.T) {
	t.Parallel()
	if got := IntervalCoverage(nil); got != 0 {
		t.Errorf("empty = %v", got)
	}
	if got := IntervalCoverage(seg(0, 10, 5, 15)); !near(got, 15) {
		t.Errorf("overlapping = %v, want 15", got)
	}
	if got := IntervalCoverage(seg(0, 10, 20, 30)); !near(got, 20) {
		t.Errorf("disjoint = %v, want 20", got)
	}
	// A zero-length or inverted segment contributes nothing.
	if got := IntervalCoverage(seg(5, 5, 10, 8)); got != 0 {
		t.Errorf("degenerate = %v, want 0", got)
	}
}

func TestIntervalOverlap(t *testing.T) {
	t.Parallel()
	if got := IntervalOverlap(seg(0, 10), seg(20, 30)); got != 0 {
		t.Errorf("disjoint = %v, want 0", got)
	}
	if got := IntervalOverlap(seg(0, 10), seg(5, 20)); !near(got, 5) {
		t.Errorf("partial = %v, want 5", got)
	}
	// Touching but not overlapping is zero shared time.
	if got := IntervalOverlap(seg(0, 10), seg(10, 20)); got != 0 {
		t.Errorf("touching = %v, want 0", got)
	}
	if got := IntervalOverlap(seg(0, 10, 20, 30), seg(5, 25)); !near(got, 10) {
		t.Errorf("multi = %v, want 10", got)
	}
}

func TestIntervalAgreement(t *testing.T) {
	t.Parallel()
	t.Run("identical sets agree completely", func(t *testing.T) {
		t.Parallel()
		if got := IntervalAgreement(seg(0, 10, 30, 40), seg(0, 10, 30, 40)); !near(got, 1) {
			t.Errorf("got %v, want 1", got)
		}
	})
	t.Run("two empty sets agree completely", func(t *testing.T) {
		t.Parallel()
		// Both runs finding no advertisements is agreement, not a divide by
		// zero and not a disagreement.
		if got := IntervalAgreement(nil, nil); got != 1 {
			t.Errorf("got %v, want 1", got)
		}
	})
	t.Run("one empty set agrees with nothing", func(t *testing.T) {
		t.Parallel()
		if got := IntervalAgreement(seg(0, 10), nil); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})
	t.Run("disjoint sets agree with nothing", func(t *testing.T) {
		t.Parallel()
		if got := IntervalAgreement(seg(0, 10), seg(20, 30)); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})
	t.Run("half overlap", func(t *testing.T) {
		t.Parallel()
		// shared 5, union 15
		if got := IntervalAgreement(seg(0, 10), seg(5, 15)); !near(got, 5.0/15.0) {
			t.Errorf("got %v, want %v", got, 5.0/15.0)
		}
	})
	t.Run("splitting one advertisement still agrees", func(t *testing.T) {
		t.Parallel()
		// The point of measuring shared time rather than segment counts: one
		// run reports a single break, the other reports it as two adjacent
		// pieces covering the same seconds.
		if got := IntervalAgreement(seg(0, 60), seg(0, 30, 30, 60)); !near(got, 1) {
			t.Errorf("got %v, want 1", got)
		}
	})
	t.Run("symmetric", func(t *testing.T) {
		t.Parallel()
		a, b := seg(0, 10, 50, 70), seg(5, 60)
		if !near(IntervalAgreement(a, b), IntervalAgreement(b, a)) {
			t.Errorf("not symmetric")
		}
	})
	t.Run("the gap tolerance of MergeIntervals does not leak in", func(t *testing.T) {
		t.Parallel()
		// MergeIntervals would close a gap of a few seconds between long
		// segments. Agreement must not, or two runs would be reported as
		// matching because the merger hid the difference.
		a := seg(0, 40)
		b := seg(0, 40, 43, 80)
		if got := IntervalAgreement(a, b); near(got, 1) {
			t.Errorf("gap tolerance leaked: got %v", got)
		}
	})
}
