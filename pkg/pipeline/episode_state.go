package pipeline

import (
	"encoding/json"
	"os"
	"strings"
	"unicode/utf8"

	"pod/pkg/audio"
	"pod/pkg/types"
	"pod/pkg/util"
)

// IsEpisodeClean reports whether an episode has finished ad removal.
func IsEpisodeClean(mp3Path string) bool {
	if !hasNonEmptyTranscript(mp3Path) {
		return false
	}
	return IsEpisodeCompleted(mp3Path)
}

func hasNonEmptyTranscript(mp3Path string) bool {
	path := util.StripExt(mp3Path) + ".transcript.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var transcript struct {
		Text                  string `json:"text"`
		AdDetectionSuccessful *bool  `json:"ad_detection_successful"`
	}
	if json.Unmarshal(data, &transcript) != nil {
		return false
	}
	return (transcript.AdDetectionSuccessful == nil || *transcript.AdDetectionSuccessful) && utf8.RuneCountInString(strings.TrimSpace(transcript.Text)) >= 50
}

// EpisodeDurations reports an episode's original and post-cut durations,
// falling back to the cuts file and then to the audio itself.
func EpisodeDurations(mp3Path string, st *types.EpisodeStatusFile) (float64, float64) {
	origDur := 0.0
	cleanDur := 0.0
	if st != nil {
		origDur = st.Original.DurationSec
		cleanDur = st.Cleaned.DurationSec
		if cleanDur == 0 && (st.Status == types.StateDone || st.Status == types.StateCopiedBack) {
			cleanDur = origDur
		}
	}
	if origDur == 0 {
		cutsFile := util.StripExt(mp3Path) + ".cuts.json"
		if data, err := os.ReadFile(cutsFile); err == nil {
			var cd types.CutsData
			if json.Unmarshal(data, &cd) == nil && cd.OriginalDurationSec > 0 {
				origDur = cd.OriginalDurationSec
				cleanDur = cd.OriginalDurationSec - cd.TotalCutDurationSec
			}
		}
	}
	if origDur == 0 {
		origDur = audio.GetAudioDuration(mp3Path)
		if st != nil && (st.Status == types.StateDone || st.Status == types.StateCopiedBack || IsEpisodeCompleted(mp3Path)) {
			cleanDur = origDur
		}
	}
	return origDur, cleanDur
}
