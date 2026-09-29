package podcast

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"pod/pkg/config"
	"pod/pkg/util"
)

// PruneOptions selects what a keep-latest prune removes.
type PruneOptions struct {
	// Keep is how many of the newest audio files each podcast keeps.
	Keep int
	// SkipFavorites leaves favorite podcasts untouched, whatever their size.
	SkipFavorites bool
}

// PruneEpisode is one episode's audio that a prune would remove: the audio
// itself and the uncut original beside it. Transcripts, cuts and status files
// are deliberately not in it.
type PruneEpisode struct {
	Audio string
	Files []string
	Bytes int64
}

// PrunePlan is what pruning one podcast would remove.
type PrunePlan struct {
	Title    string
	Dir      string
	Kept     int
	Favorite bool
	// Skipped explains why the podcast was left alone, when it was.
	Skipped string
	Delete  []PruneEpisode
}

// Bytes is the disk space the plan would free.
func (p PrunePlan) Bytes() int64 {
	var total int64
	for _, ep := range p.Delete {
		total += ep.Bytes
	}
	return total
}

// PruneResult totals what ApplyPrune removed.
type PruneResult struct {
	Podcasts int
	Episodes int
	Bytes    int64
	Failures []error
}

// PlanKeepLatest lists, without touching anything, the audio each podcast would
// lose to keep only its newest opts.Keep files. Newest means most recently
// written, since pod records no separate download time.
func (l *Library) PlanKeepLatest(entries []PodcastDirEntry, opts PruneOptions) []PrunePlan {
	plans := make([]PrunePlan, 0, len(entries))
	for _, entry := range entries {
		plans = append(plans, planKeepLatest(entry, opts))
	}
	return plans
}

func planKeepLatest(entry PodcastDirEntry, opts PruneOptions) PrunePlan {
	plan := PrunePlan{Title: entry.Title, Dir: entry.Dir}
	cfg := config.LoadPodcastConfig(entry.Dir, config.PodcastConfig{})
	plan.Favorite = cfg.Favorite
	if opts.SkipFavorites && cfg.Favorite {
		plan.Skipped = "favorite"
		return plan
	}
	files := util.FindMP3Files(entry.Dir)
	plan.Kept = min(len(files), opts.Keep)
	for _, path := range oldestBeyond(files, opts.Keep) {
		plan.Delete = append(plan.Delete, pruneEpisodeFor(path))
	}
	return plan
}

// oldestBeyond returns the files that are not among the keep newest by
// modification time. Files written in the same instant fall back on name order,
// where a later date prefix means a newer episode.
func oldestBeyond(files []string, keep int) []string {
	if keep < 0 || len(files) <= keep {
		return nil
	}
	mtime := make(map[string]int64, len(files))
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			mtime[f] = fi.ModTime().UnixNano()
		}
	}
	sorted := append([]string(nil), files...)
	sort.Slice(sorted, func(i, j int) bool {
		if mtime[sorted[i]] != mtime[sorted[j]] {
			return mtime[sorted[i]] > mtime[sorted[j]]
		}
		return sorted[i] > sorted[j]
	})
	return sorted[keep:]
}

func pruneEpisodeFor(audio string) PruneEpisode {
	ep := PruneEpisode{Audio: audio, Files: []string{audio}}
	if precut := audio + ".precut"; util.FileExists(precut) {
		ep.Files = append(ep.Files, precut)
	}
	for _, f := range ep.Files {
		ep.Bytes += fileSize(f)
	}
	return ep
}

// ApplyPrune carries out the plans. Each episode is deleted under its file lock
// and never while it is processing remotely; a failure on one episode does not
// stop the rest. Deleted episodes are dropped from the podcast's processing
// queue, and the podcast's cached index is discarded so it is rebuilt.
func ApplyPrune(plans []PrunePlan) PruneResult {
	var res PruneResult
	for _, plan := range plans {
		removed := applyPodcastPrune(plan, &res)
		if len(removed) == 0 {
			continue
		}
		res.Podcasts++
		res.Episodes += len(removed)
		if err := dropFromQueue(plan.Dir, removed); err != nil {
			res.Failures = append(res.Failures, fmt.Errorf("%s: %w", plan.Title, err))
		}
		for _, path := range staleCacheFiles(plan.Dir, removed) {
			_ = os.Remove(path)
		}
	}
	return res
}

func applyPodcastPrune(plan PrunePlan, res *PruneResult) []string {
	var removed []string
	for _, ep := range plan.Delete {
		freed, deleted, err := pruneExpiredEpisode(ep.Audio, false)
		if err != nil {
			res.Failures = append(res.Failures, fmt.Errorf("%s: %w", plan.Title, err))
			continue
		}
		if deleted {
			res.Bytes += freed
			removed = append(removed, filepath.Base(ep.Audio))
		}
	}
	return removed
}
