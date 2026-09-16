package pipeline

import (
	"fmt"

	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/types"
)

// DetectStability reports how much a detector agreed with itself across
// repeated runs over identical input.
//
// This exists because ad detection is not reproducible in general: the
// detector sends a non-zero temperature and no seed, and some providers are
// nondeterministic regardless. A single run therefore tells you what the
// model said once, not what it thinks. Repeating it turns an anecdote about a
// provider being unreliable into a number.
type DetectStability struct {
	Runs          int
	SegmentCounts []int
	AdTimes       []float64

	// MinAgreement and MeanAgreement are over every pair of runs, as shared
	// ad time divided by claimed ad time. MinAgreement is the honest headline:
	// a detector is only as reproducible as its worst pair.
	MinAgreement  float64
	MeanAgreement float64

	// Identical counts runs whose coverage matched the first run exactly.
	Identical int
}

// Deterministic reports whether every pair of runs agreed completely.
func (s DetectStability) Deterministic() bool {
	return s.Runs > 1 && s.MinAgreement >= 1
}

// DetectFileRepeated runs detection over the same transcript several times.
func DetectFileRepeated(req DetectRequest, runs int, cfg types.Config, opts types.ProcOptions, rep progress.Reporter) ([]DetectResult, DetectStability, error) {
	r := progress.Or(rep)
	if runs < 1 {
		runs = 1
	}

	var results []DetectResult
	for i := 0; i < runs; i++ {
		if runs > 1 {
			r.Detailf("run %d of %d", i+1, runs)
		}
		res, err := DetectFile(req, cfg, opts, rep)
		if err != nil {
			// Report the runs that did succeed: a detector that fails
			// intermittently is itself a finding, and discarding the earlier
			// results would hide it.
			if len(results) > 0 {
				return results, SummarizeDetectRuns(results), fmt.Errorf("run %d of %d failed: %w", i+1, runs, err)
			}
			return nil, DetectStability{}, err
		}
		results = append(results, res)
	}
	return results, SummarizeDetectRuns(results), nil
}

// SummarizeDetectRuns measures how much a set of runs agreed.
func SummarizeDetectRuns(results []DetectResult) DetectStability {
	s := DetectStability{Runs: len(results), MinAgreement: 1, MeanAgreement: 1}
	if len(results) == 0 {
		return DetectStability{}
	}
	for _, res := range results {
		s.SegmentCounts = append(s.SegmentCounts, len(res.Segments))
		s.AdTimes = append(s.AdTimes, format.IntervalCoverage(res.Segments))
	}
	if len(results) == 1 {
		s.Identical = 1
		return s
	}

	total, pairs := 0.0, 0
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			a := format.IntervalAgreement(results[i].Segments, results[j].Segments)
			if a < s.MinAgreement {
				s.MinAgreement = a
			}
			total += a
			pairs++
		}
	}
	s.MeanAgreement = total / float64(pairs)

	for _, res := range results {
		if format.IntervalAgreement(results[0].Segments, res.Segments) >= 1 {
			s.Identical++
		}
	}
	return s
}
