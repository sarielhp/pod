package adremoval

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/pipeline"
	"pod/pkg/types"
	"pod/pkg/util"
)

func whisperFirstSettings(cfg types.Config, opts types.ProcOptions) (types.Config, types.ProcOptions) {
	wp := config.GetActiveWhisperProfile(&cfg)
	if opts.WhisperEngine == string(types.WhisperEngineGemini) || (opts.WhisperEngine == "" && wp.Engine == types.WhisperEngineGemini) {
		cfg = config.PrepareWhisperFallbackConfig(cfg)
		wp = config.GetActiveWhisperProfile(&cfg)
		opts.WhisperEngine = string(wp.Engine)
	}
	if wp.URL != "" {
		cfg.WhisperURL = wp.URL
	}
	return cfg, opts
}

func saveReviewedTranscript(path string, td *types.TranscriptionData, tags map[string]string) error {
	data := map[string]any{}
	if saved, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(saved, &data); err != nil {
			return fmt.Errorf("decode existing transcript metadata: %w", err)
		}
	}
	raw, err := json.Marshal(td)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	delete(data, "cuts")
	for key, value := range fields {
		data[key] = value
	}
	for key, value := range tags {
		data["id3_"+key] = value
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(path, append(encoded, '\n'), 0644)
}

func recordGapReviewFailure(path string, failure error) {
	if err := episode.UpdateEpisodeStatus(path, func(st *types.EpisodeStatusFile) {
		st.Status = types.StateFailed
		st.CurrentStep = "gap-review"
		st.LastError = failure.Error()
	}); err != nil {
		util.Errorf("Record gap review failure: %v", err)
	}
}

func reviewMusicForRecut(job episodeJob) error {
	td, _, ok, failed := runLocalTranscriptionStep(job.sourceAudioFile, job.jsonFile, job.mainMP3File, job.totalDuration, job.cfg, job.opts, job.selectedProfile, time.Now(), job.rep)
	if failed {
		return fmt.Errorf("transcript gap review failed before recut")
	}
	if !ok {
		return fmt.Errorf("recut requires a reviewed transcript")
	}
	duration := job.totalDuration
	if td != nil && td.GapReview != nil && td.GapReview.Duration > duration {
		duration = td.GapReview.Duration
	}
	result := format.SaveDetectedCutsJSON(job.mainMP3File, duration, reviewedMusicCuts(job.cfg, td), nil, job.opts.Quiet, false, preservedTranscriptGaps(td)...)
	return result.Err
}

func verifyMusicSource(mainFile, precutFile string, duration float64, endpoint string) error {
	if endpoint == "" || util.FileExists(precutFile) {
		return nil
	}
	st, err := episode.LoadEpisodeStatus(episode.StatusPathFor(mainFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	originalDuration := 0.0
	if st != nil {
		originalDuration = st.Original.DurationSec
	}
	data, err := os.ReadFile(util.StripExt(mainFile) + ".cuts.json")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		var cuts types.CutsData
		if err := json.Unmarshal(data, &cuts); err != nil {
			return err
		}
		originalDuration = math.Max(originalDuration, cuts.OriginalDurationSec)
	}
	if originalDuration > duration+1 {
		return fmt.Errorf("music review requires the original .precut audio for %s", mainFile)
	}
	return nil
}

func recutReviewedEpisode(job episodeJob) error {
	if job.cfg.YamnetURL != "" {
		if err := reviewMusicForRecut(job); err != nil {
			return err
		}
	}
	return pipeline.HandleRecut(job.mainMP3File, job.sourceAudioFile, job.precutFile, job.outputFile, util.StripExt(job.mainMP3File), job.totalDuration, job.selectedProfile, job.cfg, job.opts, job.fileStartTime, job.rep)
}

func reviewedMusicCuts(cfg types.Config, td *types.TranscriptionData) []types.AdSegment {
	if cfg.YamnetURL == "" || td == nil || td.GapReview == nil {
		return nil
	}
	if !td.GapReview.Complete && td.GapReview.TranscriptKey == "" {
		return nil
	}
	return format.PreserveGaps(td.GapReview.MusicCuts, td.GapReview.Unresolved)
}

func preservedTranscriptGaps(td *types.TranscriptionData) []types.KeepSegment {
	if td == nil || td.GapReview == nil {
		return nil
	}
	return td.GapReview.Unresolved
}
