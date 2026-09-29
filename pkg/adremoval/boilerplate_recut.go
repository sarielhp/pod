package adremoval

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"time"

	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/pipeline"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

// minBoilerplateGainSec is the least extra audio worth a recut. Matched
// boilerplate often lines up with an earlier cut to within a fraction of a
// second, and re-encoding an episode for that gains the listener nothing.
const minBoilerplateGainSec = 2.0

// boilerplateRecutPlan is what refreshing one episode with boilerplate would do.
type boilerplateRecutPlan struct {
	cuts    []types.AdSegment
	preview format.CutsPreview
}

// recutWithBoilerplate adds the podcast's recorded boilerplate to an episode's
// existing cuts and recuts it from the uncut original. It calls no model: the
// ad cuts already on file are kept exactly as they are.
func recutWithBoilerplate(mainMP3File, precutFile, sourceAudioFile, outputFile, baseName string, totalDuration float64, selectedProfile types.LLMProfile, cfg types.Config, opts types.ProcOptions, fileStartTime time.Time, rep progress.Reporter) (Report, error) {
	name := filepath.Base(mainMP3File)
	plan, skip := planBoilerplateRecut(mainMP3File, precutFile, baseName, totalDuration)
	if skip != "" {
		rep.Infof("%s: skipped, %s", name, skip)
		return Report{}, errSkipped
	}
	if opts.DryRun {
		rep.Infof("[dry-run] %s: would add %s of boilerplate (%d passage(s)) and recut", name, format.FormatClock(plan.preview.AddedSec), len(plan.cuts))
		return Report{Processed: true}, nil
	}

	cutsFile := baseName + ".cuts.json"
	previous, _ := os.ReadFile(cutsFile)
	if res := format.SaveDetectedCutsJSON(mainMP3File, totalDuration, plan.cuts, nil, true, false); res.Err != nil {
		return Report{}, res.Err
	}
	rep.Infof("%s: adding %s of boilerplate (%d passage(s))", name, format.FormatClock(plan.preview.AddedSec), len(plan.cuts))
	if err := pipeline.HandleRecut(mainMP3File, sourceAudioFile, precutFile, outputFile, baseName, totalDuration, selectedProfile, cfg, opts, fileStartTime, rep); err != nil {
		restoreCuts(cutsFile, previous)
		return Report{Processed: true}, err
	}
	return Report{Processed: true}, nil
}

// restoreCuts puts back the cuts file as it was, so a recut that failed does not
// leave cuts on file that the audio does not have. Otherwise a rerun would see
// the boilerplate already recorded and never apply it.
func restoreCuts(path string, previous []byte) {
	if previous != nil {
		_ = util.WriteFileAtomic(path, previous, 0o644)
	}
}

// planBoilerplateRecut decides whether an episode can be refreshed, and if not,
// why. Everything it refuses is a case where recutting would be wrong or
// pointless, so that a run over a whole podcast can simply move on.
func planBoilerplateRecut(mainMP3File, precutFile, baseName string, totalDuration float64) (boilerplateRecutPlan, string) {
	var plan boilerplateRecutPlan
	if episode.IsEpisodeInRemoteFlight(mainMP3File) {
		return plan, "it is being processed remotely"
	}
	if !util.FileExists(baseName + ".cuts.json") {
		return plan, "it has no cuts file"
	}
	if !util.FileExists(precutFile) && episode.IsEpisodeClean(mainMP3File) && hasRecordedCuts(baseName) {
		return plan, "its uncut original (.precut) is gone, so it cannot be recut safely"
	}
	td, err := pipeline.LoadTranscriptFile(baseName + ".transcript.json")
	if err != nil {
		return plan, "it has no transcript to match boilerplate against"
	}
	if !transcriptFitsAudio(td, totalDuration) {
		return plan, "its transcript does not match the original audio"
	}
	plan.cuts = pipeline.BoilerplateCuts(filepath.Dir(mainMP3File), td)
	if len(plan.cuts) == 0 {
		return plan, "no recorded boilerplate matches it"
	}
	plan.preview = format.PreviewCuts(mainMP3File, totalDuration, plan.cuts)
	if !plan.preview.Changed || plan.preview.AddedSec < minBoilerplateGainSec {
		return plan, "its cuts already cover the boilerplate"
	}
	return plan, ""
}

// hasRecordedCuts reports whether the episode's cuts file records anything
// cut. An episode whose file records nothing still is its own uncut original,
// so it needs no separate .precut to be recut from.
func hasRecordedCuts(baseName string) bool {
	data, err := os.ReadFile(baseName + ".cuts.json")
	if err != nil {
		return false
	}
	var cuts types.CutsData
	if json.Unmarshal(data, &cuts) != nil {
		return true
	}
	return len(cuts.CutIntervals) > 0 || len(cuts.MergedCutIntervals) > 0
}

// transcriptFitsAudio guards against cutting audio with a transcript made from
// different audio. A transcript of the original ends near where the original
// does; one made from the already-cut file would end minutes earlier, and every
// timestamp in it would then point at the wrong place.
func transcriptFitsAudio(td *types.TranscriptionData, audioSec float64) bool {
	end := pipeline.TranscriptDuration(td)
	if end <= 0 || audioSec <= 0 {
		return false
	}
	return math.Abs(end-audioSec) <= math.Max(60, 0.05*audioSec)
}
