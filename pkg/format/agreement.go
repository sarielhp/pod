package format

import "pod/pkg/types"

// coalesceIntervals merges overlapping and touching intervals with no gap
// tolerance.
//
// MergeIntervals deliberately closes small gaps, because a cut that leaves a
// two-second stub of an advertisement is worse than one that takes a little
// extra. That behaviour is wrong for measuring agreement: it would report two
// models as agreeing because the merger papered over the difference between
// them. Comparison needs the intervals exactly as given.
func coalesceIntervals(ads []types.AdSegment) []types.AdSegment {
	if len(ads) == 0 {
		return nil
	}
	sorted := make([]types.AdSegment, 0, len(ads))
	for _, a := range ads {
		if a.End > a.Start {
			sorted = append(sorted, a)
		}
	}
	if len(sorted) == 0 {
		return nil
	}
	sortAds(sorted)

	out := []types.AdSegment{sorted[0]}
	for _, a := range sorted[1:] {
		last := &out[len(out)-1]
		if a.Start <= last.End {
			if a.End > last.End {
				last.End = a.End
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

// IntervalCoverage is the total time covered by a set of segments, counting
// overlapping segments once.
func IntervalCoverage(ads []types.AdSegment) float64 {
	total := 0.0
	for _, a := range coalesceIntervals(ads) {
		total += a.End - a.Start
	}
	return total
}

// IntervalOverlap is the total time covered by both sets.
func IntervalOverlap(a, b []types.AdSegment) float64 {
	x, y := coalesceIntervals(a), coalesceIntervals(b)
	total := 0.0
	i, j := 0, 0
	for i < len(x) && j < len(y) {
		start := x[i].Start
		if y[j].Start > start {
			start = y[j].Start
		}
		end := x[i].End
		if y[j].End < end {
			end = y[j].End
		}
		if end > start {
			total += end - start
		}
		if x[i].End < y[j].End {
			i++
		} else {
			j++
		}
	}
	return total
}

// IntervalAgreement is how much two sets of segments coincide, from 0 (no
// shared time) to 1 (identical coverage).
//
// It is the Jaccard index over the timeline — shared time divided by time
// either one claims — rather than a comparison of segment counts or
// boundaries. That is the right question for ad detection: two runs that
// split the same advertisement break differently have found the same
// advertisements, while two that agree on the count and disagree on where
// have not. Two sets that are both empty agree completely.
func IntervalAgreement(a, b []types.AdSegment) float64 {
	shared := IntervalOverlap(a, b)
	union := IntervalCoverage(a) + IntervalCoverage(b) - shared
	if union <= 0 {
		return 1
	}
	return shared / union
}
