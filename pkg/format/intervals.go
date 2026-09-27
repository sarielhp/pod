package format

import (
	"cmp"
	"math"
	"slices"

	"pod/pkg/types"
)

const maxAdSegments = 500

func sanitizeAdSegments(ads []types.AdSegment, totalDuration float64) []types.AdSegment {
	if totalDuration <= 0 {
		return ads
	}
	var out []types.AdSegment
	for _, a := range ads {
		if math.IsNaN(a.Start) || math.IsNaN(a.End) || math.IsInf(a.Start, 0) || math.IsInf(a.End, 0) {
			continue
		}
		if a.Start < 0 {
			a.Start = 0
		}
		if a.End > totalDuration {
			a.End = totalDuration
		}
		if a.End <= a.Start {
			continue
		}
		out = append(out, a)
		if len(out) >= maxAdSegments {
			break
		}
	}
	return out
}

func MergeIntervals(ads []types.AdSegment) []types.AdSegment {
	if len(ads) == 0 {
		return ads
	}

	sorted := make([]types.AdSegment, len(ads))
	copy(sorted, ads)
	sortAds(sorted)

	merged := []types.AdSegment{sorted[0]}

	for i := 1; i < len(sorted); i++ {
		last := &merged[len(merged)-1]
		ad := sorted[i]
		lastSt := last.Start
		lastEn := last.End
		adSt := ad.Start
		adEn := ad.End

		dur1 := lastEn - lastSt
		dur2 := adEn - adSt
		minDur := dur1
		if dur2 < minDur {
			minDur = dur2
		}

		allowedGap := 0.0
		switch {
		case minDur >= 30.0:
			allowedGap = 5.0
		case minDur >= 20.0:
			allowedGap = 4.0
		case minDur >= 10.0:
			allowedGap = 3.0
		}

		if adSt <= lastEn+allowedGap {
			if adEn > lastEn {
				last.End = adEn
			}
			if ad.Reason != "" && last.Reason == "" {
				last.Reason = ad.Reason
			}
		} else {
			merged = append(merged, ad)
		}
	}

	return merged
}

func sortAds(ads []types.AdSegment) {
	slices.SortFunc(ads, func(a, b types.AdSegment) int {
		if c := cmp.Compare(a.Start, b.Start); c != 0 {
			return c
		}
		return cmp.Compare(a.End, b.End)
	})
}

// CalculateKeepSegments returns the non-ad segments to retain over totalDuration.
func CalculateKeepSegments(totalDuration float64, ads []types.AdSegment) [][2]float64 {
	ads = sanitizeAdSegments(ads, totalDuration)
	sorted := make([]types.AdSegment, len(ads))
	copy(sorted, ads)
	sortAds(sorted)

	var keep [][2]float64
	currentStart := 0.0

	for _, ad := range sorted {
		adStart := ad.Start
		adEnd := ad.End

		if adStart > currentStart {
			keep = append(keep, [2]float64{currentStart, adStart})
		}
		if adEnd > currentStart {
			currentStart = adEnd
		}
	}

	if currentStart < totalDuration {
		keep = append(keep, [2]float64{currentStart, totalDuration})
	}

	return keep
}

func sortBounds(bounds [][2]float64) {
	slices.SortFunc(bounds, func(a, b [2]float64) int {
		if c := cmp.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return cmp.Compare(a[1], b[1])
	})
}

func mergeBounds(bounds [][2]float64) [][2]float64 {
	if len(bounds) == 0 {
		return bounds
	}

	merged := [][2]float64{bounds[0]}

	for i := 1; i < len(bounds); i++ {
		last := &merged[len(merged)-1]
		st := bounds[i][0]
		en := bounds[i][1]

		dur1 := (*last)[1] - (*last)[0]
		dur2 := en - st
		minDur := dur1
		if dur2 < minDur {
			minDur = dur2
		}

		allowedGap := 0.0
		switch {
		case minDur >= 30.0:
			allowedGap = 5.0
		case minDur >= 20.0:
			allowedGap = 4.0
		case minDur >= 10.0:
			allowedGap = 3.0
		}

		if st <= (*last)[1]+allowedGap {
			if en > (*last)[1] {
				(*last)[1] = en
			}
		} else {
			merged = append(merged, bounds[i])
		}
	}

	return merged
}

func equalMergedIntervals(a, b []types.MergedCutInterval) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Start != b[i].Start || a[i].End != b[i].End {
			return false
		}
	}
	return true
}
