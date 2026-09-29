package detect

import (
	"math"
	"testing"

	"pod/pkg/types"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestScoreSegments(t *testing.T) {
	t.Parallel()
	truth := []types.AdSegment{{Start: 0, End: 30}, {Start: 100, End: 160}}
	detected := []types.AdSegment{{Start: 10, End: 40}, {Start: 100, End: 160}, {Start: 200, End: 220}}
	s := ScoreSegments(detected, truth)

	if !near(s.TruthSec, 90) || !near(s.DetectedSec, 110) || !near(s.OverlapSec, 80) {
		t.Fatalf("unexpected seconds: %+v", s)
	}
	if !near(s.FalsePositiveSec, 30) || !near(s.MissedSec, 10) {
		t.Errorf("wrong error seconds: %+v", s)
	}
	if !near(s.Precision, 80.0/110) || !near(s.Recall, 80.0/90) {
		t.Errorf("wrong ratios: %+v", s)
	}
}

func TestScoreCountsOverlappingSegmentsOnce(t *testing.T) {
	t.Parallel()
	truth := []types.AdSegment{{Start: 0, End: 60}}
	repeated := []types.AdSegment{{Start: 0, End: 60}, {Start: 10, End: 50}, {Start: 0, End: 60}}
	s := ScoreSegments(repeated, truth)
	if !near(s.DetectedSec, 60) || !near(s.Precision, 1) || !near(s.Recall, 1) {
		t.Errorf("repeats must not inflate the detected time: %+v", s)
	}
}

func TestScoreOfNothingAgainstNothingIsPerfect(t *testing.T) {
	t.Parallel()
	s := ScoreSegments(nil, nil)
	if !near(s.Precision, 1) || !near(s.Recall, 1) {
		t.Errorf("an ad-free episode with no detections is a correct answer: %+v", s)
	}
	missed := ScoreSegments(nil, []types.AdSegment{{Start: 0, End: 30}})
	if !near(missed.Recall, 0) || !near(missed.Precision, 1) {
		t.Errorf("detecting nothing is precise but has no recall: %+v", missed)
	}
	over := ScoreSegments([]types.AdSegment{{Start: 0, End: 30}}, nil)
	if !near(over.Precision, 0) || !near(over.FalsePositiveSec, 30) {
		t.Errorf("cutting an ad-free episode is all false positive: %+v", over)
	}
}

func TestScoreAddWeighsByTime(t *testing.T) {
	t.Parallel()
	long := ScoreSegments([]types.AdSegment{{Start: 0, End: 900}}, []types.AdSegment{{Start: 0, End: 1000}})
	short := ScoreSegments([]types.AdSegment{{Start: 0, End: 10}}, []types.AdSegment{{Start: 0, End: 10}})
	sum := long.Add(short)
	if !near(sum.Recall, 910.0/1010) || !near(sum.MissedSec, 100) {
		t.Errorf("totals should be summed seconds, not averaged ratios: %+v", sum)
	}
}
