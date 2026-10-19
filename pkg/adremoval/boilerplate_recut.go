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

const (
	skipRemote       = "it is being processed remotely"
	skipNoCuts       = "it has no cuts file"
	skipNoPrecut     = "its uncut original (.precut) is gone, so it cannot be recut safely"
	skipNoTranscript = "it has no transcript to match boilerplate against"
	skipMismatch     = "its transcript does not match the original audio"
	skipNoMatch      = "no recorded boilerplate matches it"
	skipCovered      = "its cuts already cover the boilerplate"
)

// A recut skips an episode for these reasons because of what its files hold,
// so the same files will be skipped the same way next time. Being busy or
// remote is a passing state and is not remembered.
var settledSkips = map[string]bool{
	skipNoCuts:       true,
	skipNoPrecut:     true,
	skipNoTranscript: true,
	skipMismatch:     true,
	skipNoMatch:      true,
	skipCovered:      true,
}

// boilerplateRecutPlan is what refreshing one episode with boilerplate would do.
type boilerplateRecutPlan struct {
	cuts      []types.AdSegment
	preview   format.CutsPreview
	preserved []types.KeepSegment
}

// applyBoilerplateRecut adds the planned boilerplate cuts to the episode's cuts
// file and recuts it from the uncut original. It calls no model: the ad cuts
// already on file are kept exactly as they are. If the recut fails the previous
// cuts file is put back.
func applyBoilerplateRecut(mainMP3File, precutFile, sourceAudioFile, outputFile, baseName string, totalDuration float64, plan boilerplateRecutPlan, cfg types.Config, opts types.ProcOptions, start time.Time, rep progress.Reporter) error {
	cutsFile := baseName + ".cuts.json"
	previous, _ := os.ReadFile(cutsFile)
	if res := format.SaveDetectedCutsJSON(mainMP3File, totalDuration, plan.cuts, nil, true, false, plan.preserved...); res.Err != nil {
		return res.Err
	}
	if err := pipeline.HandleRecut(mainMP3File, sourceAudioFile, precutFile, outputFile, baseName, totalDuration, types.LLMProfile{}, cfg, opts, start, rep); err != nil {
		restoreCuts(cutsFile, previous)
		return err
	}
	return nil
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
		return plan, skipRemote
	}
	if !util.FileExists(baseName + ".cuts.json") {
		return plan, skipNoCuts
	}
	if !util.FileExists(precutFile) && episode.IsEpisodeClean(mainMP3File) && hasRecordedCuts(baseName) {
		return plan, skipNoPrecut
	}
	td, err := pipeline.LoadTranscriptFile(baseName + ".transcript.json")
	if err != nil {
		return plan, skipNoTranscript
	}
	if !transcriptFitsAudio(td, totalDuration) {
		return plan, skipMismatch
	}
	plan.cuts = pipeline.BoilerplateCuts(filepath.Dir(mainMP3File), td)
	plan.preserved = preservedTranscriptGaps(td)
	plan.cuts = format.PreserveGaps(plan.cuts, plan.preserved)
	if len(plan.cuts) == 0 {
		return plan, skipNoMatch
	}
	plan.preview = format.PreviewCuts(mainMP3File, totalDuration, plan.cuts, plan.preserved...)
	if !plan.preview.Changed || plan.preview.AddedSec < minBoilerplateGainSec {
		return plan, skipCovered
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
