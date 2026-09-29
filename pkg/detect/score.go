package detect

import (
	"sort"

	"pod/pkg/types"
)

// Score compares detected ad segments with a labelled truth, in seconds.
//
// The two errors are not equal. FalsePositiveSec is programme audio a cut would
// delete, which the listener never gets back; MissedSec is an ad left in, which
// costs a few seconds of patience. Precision is therefore the figure to protect.
type Score struct {
	TruthSec         float64 `json:"truth_sec"`
	DetectedSec      float64 `json:"detected_sec"`
	OverlapSec       float64 `json:"overlap_sec"`
	FalsePositiveSec float64 `json:"false_positive_sec"`
	MissedSec        float64 `json:"missed_sec"`
	Precision        float64 `json:"precision"`
	Recall           float64 `json:"recall"`
}

// ScoreSegments scores detected against truth. Overlapping segments within
// either list are counted once, so a model that repeats a segment is not
// rewarded or punished for it.
func ScoreSegments(detected, truth []types.AdSegment) Score {
	d, t := unionIntervals(detected), unionIntervals(truth)
	s := Score{TruthSec: totalSeconds(t), DetectedSec: totalSeconds(d), OverlapSec: overlapSeconds(d, t)}
	s.FalsePositiveSec = s.DetectedSec - s.OverlapSec
	s.MissedSec = s.TruthSec - s.OverlapSec
	s.Precision, s.Recall = 1, 1
	if s.DetectedSec > 0 {
		s.Precision = s.OverlapSec / s.DetectedSec
	}
	if s.TruthSec > 0 {
		s.Recall = s.OverlapSec / s.TruthSec
	}
	return s
}

// Add accumulates another score, for judging a set of episodes together.
// Ratios are recomputed from the summed seconds, so long episodes weigh more
// than short ones, as they do in the listener's time.
func (s Score) Add(o Score) Score {
	sum := Score{
		TruthSec:    s.TruthSec + o.TruthSec,
		DetectedSec: s.DetectedSec + o.DetectedSec,
		OverlapSec:  s.OverlapSec + o.OverlapSec,
	}
	sum.FalsePositiveSec = sum.DetectedSec - sum.OverlapSec
	sum.MissedSec = sum.TruthSec - sum.OverlapSec
	sum.Precision, sum.Recall = 1, 1
	if sum.DetectedSec > 0 {
		sum.Precision = sum.OverlapSec / sum.DetectedSec
	}
	if sum.TruthSec > 0 {
		sum.Recall = sum.OverlapSec / sum.TruthSec
	}
	return sum
}

func unionIntervals(segs []types.AdSegment) [][2]float64 {
	var spans [][2]float64
	for _, seg := range segs {
		if seg.End > seg.Start {
			spans = append(spans, [2]float64{seg.Start, seg.End})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var out [][2]float64
	for _, span := range spans {
		last := len(out) - 1
		if last >= 0 && span[0] <= out[last][1] {
			out[last][1] = max(out[last][1], span[1])
			continue
		}
		out = append(out, span)
	}
	return out
}

func totalSeconds(spans [][2]float64) float64 {
	total := 0.0
	for _, span := range spans {
		total += span[1] - span[0]
	}
	return total
}

func overlapSeconds(a, b [][2]float64) float64 {
	total := 0.0
	for i, j := 0, 0; i < len(a) && j < len(b); {
		lo, hi := max(a[i][0], b[j][0]), min(a[i][1], b[j][1])
		if hi > lo {
			total += hi - lo
		}
		if a[i][1] < b[j][1] {
			i++
		} else {
			j++
		}
	}
	return total
}
