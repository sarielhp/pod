package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/audio"
	"pod/pkg/config"
	"pod/pkg/types"
	"pod/pkg/util"
)

func StatusPathFor(audioPath string) string {
	return audioPath + ".json"
}

func LoadEpisodeStatus(path string) (*types.EpisodeStatusFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st types.EpisodeStatusFile
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("invalid episode status json in %s: %w", path, err)
	}
	if st.IsFavorite() {
		st.SetFavorite(true)
	}
	return &st, nil
}

func SaveEpisodeStatus(path string, st *types.EpisodeStatusFile) error {
	applySourcePublication(strings.TrimSuffix(path, ".json"), st)
	if st.IsFavorite() {
		st.SetFavorite(true)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if st.CreatedAt == "" {
		st.CreatedAt = st.UpdatedAt
	}
	if st.Version == 0 {
		st.Version = 1
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal episode status: %w", err)
	}
	return util.WriteFileAtomic(path, append(data, '\n'), 0644)
}

func GetOrCreateEpisodeStatus(audioPath string) *types.EpisodeStatusFile {
	statPath := StatusPathFor(audioPath)
	if st, err := LoadEpisodeStatus(statPath); err == nil && st != nil {
		applySourcePublication(audioPath, st)
		return st
	}
	now := time.Now().UTC().Format(time.RFC3339)
	fname := filepath.Base(audioPath)
	var sz int64
	if fi, err := os.Stat(audioPath); err == nil {
		sz = fi.Size()
	}
	dur := audio.GetAudioDuration(audioPath)
	st := &types.EpisodeStatusFile{
		Version:   1,
		MediaFile: fname,
		Status:    types.StateDownloaded,
		CreatedAt: now,
		UpdatedAt: now,
		Original: types.EpisodeAudioMeta{
			Filename:    fname,
			DurationSec: dur,
			SizeBytes:   sz,
		},
	}

	applyInitialFavoriteStatus(audioPath, st)
	populatePrecutOrCutsMeta(st, audioPath, fname, dur, sz)
	populateAdsFromCutsFile(st, util.StripExt(audioPath)+".cuts.json")

	if err := SaveEpisodeStatus(statPath, st); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize episode status file '%s': %v\n", statPath, err)
	}
	return st
}

func applyInitialFavoriteStatus(audioPath string, st *types.EpisodeStatusFile) {
	podDir := filepath.Dir(audioPath)
	cfgPath := filepath.Join(podDir, config.PodcastConfigFileName)
	if !util.FileExists(cfgPath) {
		return
	}
	podCfg := config.LoadPodcastConfig(podDir, config.PodcastConfig{})
	if !podCfg.Favorite || podCfg.FavoriteSince == nil {
		return
	}
	fi, err := os.Stat(audioPath)
	if err != nil {
		return
	}
	if fi.ModTime().UTC().Before(*podCfg.FavoriteSince) {
		return
	}
	st.SetFavorite(true)
}

func populatePrecutOrCutsMeta(st *types.EpisodeStatusFile, audioPath, fname string, dur float64, sz int64) {
	base := util.StripExt(audioPath)
	cutsFile := base + ".cuts.json"
	transcriptFile := base + ".transcript.json"
	precutFile := audioPath + ".precut"

	if util.FileExists(precutFile) {
		var origSz int64
		if fi, err := os.Stat(precutFile); err == nil {
			origSz = fi.Size()
		}
		origDur := audio.GetAudioDuration(precutFile)
		st.Status = types.StateDone
		st.Original = types.EpisodeAudioMeta{
			Filename:    filepath.Base(precutFile),
			DurationSec: origDur,
			SizeBytes:   origSz,
		}
		st.Cleaned = types.EpisodeAudioMeta{
			Filename:      fname,
			DurationSec:   dur,
			SizeBytes:     sz,
			AdDurationSec: origDur - dur,
		}
	} else if util.FileExists(cutsFile) && util.FileExists(transcriptFile) {
		data, err := os.ReadFile(cutsFile)
		var cd types.CutsData
		if err == nil && json.Unmarshal(data, &cd) == nil && len(cd.CutIntervals) == 0 {
			st.Status = types.StateDone
			st.Cleaned = types.EpisodeAudioMeta{
				Filename:    fname,
				DurationSec: dur,
				SizeBytes:   sz,
			}
		}
	}
}

func populateAdsFromCutsFile(st *types.EpisodeStatusFile, cutsFile string) {
	if !util.FileExists(cutsFile) {
		return
	}
	data, err := os.ReadFile(cutsFile)
	if err != nil {
		return
	}
	var cd types.CutsData
	if json.Unmarshal(data, &cd) != nil || len(cd.CutIntervals) == 0 {
		return
	}
	for _, c := range cd.CutIntervals {
		st.Ads = append(st.Ads, types.EpisodeAdCut{
			Start:  c.StartSec,
			End:    c.EndSec,
			Reason: c.Reason,
		})
	}
}

var statusUpdateMu util.SyncMutex

func UpdateEpisodeStatus(audioPath string, mutate func(*types.EpisodeStatusFile)) error {
	statusUpdateMu.Lock()
	defer statusUpdateMu.Unlock()

	statPath := StatusPathFor(audioPath)
	lock, err := util.AcquireFileLockWithTimeout(statPath, 5*time.Second)
	if err != nil || lock == nil {
		return fmt.Errorf("status file is locked: %w", err)
	}
	defer lock.Release()

	st := GetOrCreateEpisodeStatus(audioPath)
	mutate(st)
	if err := SaveEpisodeStatus(statPath, st); err != nil {
		return fmt.Errorf("could not record status for %s: %w", audioPath, err)
	}
	return nil
}

func IsEpisodeCompleted(audioPath string) bool {
	statPath := StatusPathFor(audioPath)
	st, err := LoadEpisodeStatus(statPath)
	if err == nil && st != nil {
		if st.AdDetectionSuccessful != nil && !*st.AdDetectionSuccessful {
			return false
		}
		if st.Status == types.StateNeedsAdR || st.Status == types.StateFailed {
			return false
		}
		if st.Status == types.StateDone || st.Status == types.StateCopiedBack || st.Status == types.StateArchived {
			return true
		}
	}
	base := util.StripExt(audioPath)
	cutsFile := base + ".cuts.json"
	transcriptFile := base + ".transcript.json"
	precutFile := audioPath + ".precut"
	if util.FileExists(cutsFile) && util.FileExists(transcriptFile) {
		if util.FileExists(precutFile) {
			return true
		}
		data, err := os.ReadFile(cutsFile)
		var cd types.CutsData
		if err == nil && json.Unmarshal(data, &cd) == nil && len(cd.CutIntervals) == 0 {
			return true
		}
	}
	return false
}

func IsEpisodeInRemoteFlight(audioPath string) bool {
	statPath := StatusPathFor(audioPath)
	st, err := LoadEpisodeStatus(statPath)
	if err == nil && st != nil {
		switch st.Status {
		case types.StateQueuedRemote, types.StateTranscribingRemotely, types.StateCuttingRemotely, types.StateReadyForCopyBack, types.StateAwaitingTranscription:
			return true
		}
	}
	return false
}
