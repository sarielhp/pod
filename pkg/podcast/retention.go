package podcast

import (
	"fmt"
	"os"
	"time"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/util"
)

// KeepPolicyPruneResult summarizes MP3 audio cleanup performed under a
// podcast's keep policy.
type KeepPolicyPruneResult struct {
	PodcastDir           string   `json:"podcast_dir"`
	PodcastTitle         string   `json:"podcast_title"`
	Policy               string   `json:"policy"`
	RetentionDays        int      `json:"retention_days"`
	TotalEpisodes        int      `json:"total_episodes"`
	ExpiredEpisodes      int      `json:"expired_episodes"`
	DeletedEpisodes      int      `json:"deleted_episodes"`
	PreservedTranscripts int      `json:"preserved_transcripts"`
	DeletedFiles         []string `json:"deleted_files"`
	FreedBytes           int64    `json:"freed_bytes"`
	Errors               []error  `json:"errors,omitempty"`
}

// ApplyPodcastKeepPolicy prunes expired MP3 audio files in podDir based on the
// podcast's keep policy, while strictly preserving transcripts, ID3 tags, and
// episode metadata.
func ApplyPodcastKeepPolicy(podDir, title string, cfg config.PodcastConfig, now time.Time, dryRun bool) (KeepPolicyPruneResult, error) {
	policy := cfg.EffectiveKeepPolicy()
	days := cfg.EffectiveCleanupDays()
	res := KeepPolicyPruneResult{
		PodcastDir:    podDir,
		PodcastTitle:  title,
		Policy:        policy,
		RetentionDays: days,
	}

	if days <= 0 || policy == config.KeepPolicyAlways {
		return res, nil
	}

	mp3Files := util.FindMP3Files(podDir)
	res.TotalEpisodes = len(mp3Files)
	if len(mp3Files) == 0 {
		return res, nil
	}

	cutoff := now.AddDate(0, 0, -days)
	for _, path := range mp3Files {
		expired, _ := isEpisodeExpired(path, cutoff)
		if !expired {
			continue
		}
		res.ExpiredEpisodes++
		freed, deleted, err := pruneExpiredEpisode(path, dryRun)
		if err != nil {
			res.Errors = append(res.Errors, err)
			continue
		}
		if deleted {
			res.FreedBytes += freed
			res.DeletedEpisodes++
			res.DeletedFiles = append(res.DeletedFiles, path)
			if hasAssociatedTranscript(path) {
				res.PreservedTranscripts++
			}
		}
	}
	return res, nil
}

// isEpisodeExpired ages an episode from the later of its publication date and
// the file's own mtime. The feed's pubDate alone would let a back-catalogue
// feed, or a hostile one, have a file deleted the moment it finished
// downloading, and then re-downloaded and re-deleted on every later run.
func isEpisodeExpired(path string, cutoff time.Time) (bool, time.Time) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, time.Time{}
	}
	age := fi.ModTime()
	if pubTime := GetEpisodePublicationTime(path); pubTime.After(age) {
		age = pubTime
	}
	return age.Before(cutoff), age
}

func hasAssociatedTranscript(path string) bool {
	base := util.StripExt(path)
	return util.FileExists(base+".transcript.json") ||
		util.FileExists(base+".transcript.txt") ||
		util.FileExists(base+".srt") ||
		util.FileExists(base+".txt")
}

func pruneExpiredEpisode(path string, dryRun bool) (int64, bool, error) {
	if episode.IsEpisodeInRemoteFlight(path) {
		return 0, false, nil
	}
	precut := path + ".precut"
	var bytesFreed int64
	if fi, err := os.Stat(path); err == nil {
		bytesFreed += fi.Size()
	}
	if fi, err := os.Stat(precut); err == nil {
		bytesFreed += fi.Size()
	}
	if dryRun {
		return bytesFreed, true, nil
	}

	lock, err := util.AcquireFileLock(path)
	if err != nil {
		return 0, false, fmt.Errorf("lock %s: %w", path, err)
	}
	if lock == nil {
		return 0, false, fmt.Errorf("%s is being processed by another pod instance", path)
	}
	defer lock.Release()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return 0, false, fmt.Errorf("remove %s: %w", path, err)
	}
	if util.FileExists(precut) {
		_ = os.Remove(precut)
	}
	return bytesFreed, true, nil
}

// PrunePodcastKeepPolicy executes keep policy pruning for a podcast directory.
func (l *Library) PrunePodcastKeepPolicy(podDir, title string, dryRun bool) (KeepPolicyPruneResult, error) {
	podCfg := config.LoadPodcastConfig(podDir, config.DefaultPodcastConfig(nil))
	return ApplyPodcastKeepPolicy(podDir, title, podCfg, time.Now(), dryRun)
}

// PruneLibraryKeepPolicies runs keep policy pruning across all podcasts in the library.
func (l *Library) PruneLibraryKeepPolicies(dryRun bool) ([]KeepPolicyPruneResult, error) {
	if l.cfg.PodcastsDir == "" {
		return nil, nil
	}
	entries := ScanPodcastDirs(l.cfg.PodcastsDir)
	results := make([]KeepPolicyPruneResult, 0, len(entries))
	for _, entry := range entries {
		res, err := l.PrunePodcastKeepPolicy(entry.Dir, entry.Title, dryRun)
		if err != nil {
			res.Errors = append(res.Errors, err)
		}
		results = append(results, res)
	}
	return results, nil
}
