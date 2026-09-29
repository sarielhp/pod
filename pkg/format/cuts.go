package format

import (
	"encoding/json"
	"fmt"
	"os"

	"pod/pkg/types"
	"pod/pkg/util"
)

func buildCutEntries(combined []types.AdSegment) []types.CutEntry {
	formattedRaw := make([]types.CutEntry, 0, len(combined))
	for _, ad := range combined {
		entry := types.CutEntry{
			StartSec:       util.RoundFloat(ad.Start, 2),
			EndSec:         util.RoundFloat(ad.End, 2),
			DurationSec:    util.RoundFloat(ad.End-ad.Start, 2),
			StartFormatted: FormatClock(ad.Start),
			EndFormatted:   FormatClock(ad.End),
		}
		if ad.Reason != "" {
			entry.Reason = ad.Reason
		}
		formattedRaw = append(formattedRaw, entry)
	}
	return formattedRaw
}

func buildMergedAndKeepIntervals(totalDuration float64, combined []types.AdSegment) ([]types.MergedCutInterval, [][2]float64, []types.KeepSegment) {
	combined = sanitizeAdSegments(combined, totalDuration)
	allBounds := make([][2]float64, 0, len(combined))
	for _, ad := range combined {
		allBounds = append(allBounds, [2]float64{ad.Start, ad.End})
	}
	sortBounds(allBounds)

	newMerged := mergeBounds(allBounds)
	formattedMerged := make([]types.MergedCutInterval, 0, len(newMerged))
	var mergedAds []types.AdSegment
	for _, b := range newMerged {
		formattedMerged = append(formattedMerged, types.MergedCutInterval{
			Start: util.RoundFloat(b[0], 2),
			End:   util.RoundFloat(b[1], 2),
		})
		mergedAds = append(mergedAds, types.AdSegment{Start: b[0], End: b[1]})
	}

	keep := CalculateKeepSegments(totalDuration, mergedAds)
	keepIntervals := make([]types.KeepSegment, 0, len(keep))
	for _, k := range keep {
		keepIntervals = append(keepIntervals, types.KeepSegment{
			Start: util.RoundFloat(k[0], 2),
			End:   util.RoundFloat(k[1], 2),
		})
	}
	return formattedMerged, keep, keepIntervals
}

func loadExistingCuts(cutsFile string) ([]types.AdSegment, []types.MergedCutInterval, *types.CutsData) {
	var existingRaw []types.AdSegment
	var existingMerged []types.MergedCutInterval
	var existingCutsData *types.CutsData

	if util.FileExists(cutsFile) {
		data, err := os.ReadFile(cutsFile)
		if err == nil {
			var existing types.CutsData
			if json.Unmarshal(data, &existing) == nil {
				existingCutsData = &existing
				existingMerged = append(existingMerged, existing.MergedCutIntervals...)
				for _, c := range existing.CutIntervals {
					existingRaw = append(existingRaw, types.AdSegment{Start: c.StartSec, End: c.EndSec, Reason: c.Reason})
				}
			}
		}
	}
	return existingRaw, existingMerged, existingCutsData
}

func SaveCutsJSON(mainFile string, totalDuration float64, adSegments []types.AdSegment, profile *types.LLMProfile, quiet bool) types.CutsResult {
	return SaveDetectedCutsJSON(mainFile, totalDuration, adSegments, profile, quiet, false)
}

// SaveDetectedCutsJSON saves detected ads to the episode's .cuts.json. Normally
// they are merged into the cuts already there, so a later run only ever adds.
// With replace, the previous cuts are discarded: a forced re-detection has to be
// able to remove a bad cut, and merging would keep it forever.
func SaveDetectedCutsJSON(mainFile string, totalDuration float64, adSegments []types.AdSegment, profile *types.LLMProfile, quiet, replace bool) types.CutsResult {
	base := util.StripExt(mainFile)
	cutsFile := base + ".cuts.json"

	var existingRaw []types.AdSegment
	var existingMerged []types.MergedCutInterval
	var existingCutsData *types.CutsData
	if !replace {
		existingRaw, existingMerged, existingCutsData = loadExistingCuts(cutsFile)
	}
	combined := append(existingRaw, adSegments...)
	combined = sanitizeAdSegments(combined, totalDuration)

	formattedRaw := buildCutEntries(combined)
	formattedMerged, keep, keepIntervals := buildMergedAndKeepIntervals(totalDuration, combined)

	if existingCutsData != nil && equalMergedIntervals(existingMerged, formattedMerged) {
		if !quiet {
			fmt.Println("No new ad cuts were discovered (cut set remains unchanged).")
		}
		return types.CutsResult{
			CutsFile:     cutsFile,
			KeepSegments: keep,
			Changed:      false,
		}
	}

	totalCutSec := 0.0
	for _, mc := range formattedMerged {
		totalCutSec += mc.End - mc.Start
	}
	totalCutSec = util.RoundFloat(totalCutSec, 2)

	llmInfo := "Unknown"
	if profile != nil {
		llmInfo = fmt.Sprintf("%s (%s)", profile.Name, profile.Model)
	} else if existingCutsData != nil && existingCutsData.LLMUsed != "" {
		llmInfo = existingCutsData.LLMUsed
	}

	cutsData := types.CutsData{
		Version:             1,
		Generator:           "pod",
		LLMUsed:             llmInfo,
		TargetFile:          util.FilepathBase(mainFile),
		OriginalDurationSec: util.RoundFloat(totalDuration, 2),
		TotalCutDurationSec: totalCutSec,
		CutIntervals:        formattedRaw,
		MergedCutIntervals:  formattedMerged,
		KeepIntervals:       keepIntervals,
	}

	data, err := json.MarshalIndent(cutsData, "", "  ")
	if err != nil {
		return failedCutsResult(cutsFile, keep, fmt.Errorf("serialize cuts metadata %s: %w", cutsFile, err), quiet)
	}
	if err := util.WriteFileAtomic(cutsFile, append(data, '\n'), 0644); err != nil {
		return failedCutsResult(cutsFile, keep, fmt.Errorf("write cuts metadata %s: %w", cutsFile, err), quiet)
	}

	if !quiet {
		fmt.Printf("Saved updated cut metadata (.json) to: '%s'\n", cutsFile)
	}

	return types.CutsResult{
		CutsFile:     cutsFile,
		KeepSegments: keep,
		Changed:      true,
	}
}

func failedCutsResult(cutsFile string, keep [][2]float64, err error, quiet bool) types.CutsResult {
	if !quiet {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	return types.CutsResult{CutsFile: cutsFile, KeepSegments: keep, Err: err}
}

// CutsPreview says what merging more cuts into an episode's cuts file would do.
type CutsPreview struct {
	Changed  bool
	AddedSec float64
}

// PreviewCuts reports, without writing anything, whether adding adSegments to the
// episode's existing cuts would change the set of intervals cut, and by how many
// seconds it would grow.
func PreviewCuts(mainFile string, totalDuration float64, adSegments []types.AdSegment) CutsPreview {
	existingRaw, existingMerged, existing := loadExistingCuts(util.StripExt(mainFile) + ".cuts.json")
	combined := sanitizeAdSegments(append(existingRaw, adSegments...), totalDuration)
	merged, _, _ := buildMergedAndKeepIntervals(totalDuration, combined)
	return CutsPreview{
		Changed:  existing == nil || !equalMergedIntervals(existingMerged, merged),
		AddedSec: mergedSeconds(merged) - mergedSeconds(existingMerged),
	}
}

func mergedSeconds(intervals []types.MergedCutInterval) float64 {
	total := 0.0
	for _, iv := range intervals {
		total += iv.End - iv.Start
	}
	return total
}
