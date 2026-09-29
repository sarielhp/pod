package podcast

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/config"
	"pod/pkg/util"
)

// PodcastUsage is one podcast's share of the library's disk space.
type PodcastUsage struct {
	Title    string `json:"title"`
	Episodes int    `json:"episodes"`
	Bytes    int64  `json:"bytes"`
	Favorite bool   `json:"favorite,omitempty"`
}

// LibraryStats describes what a podcast library holds and what it costs on disk.
type LibraryStats struct {
	Root           string `json:"root"`
	Podcasts       int    `json:"podcasts"`
	Favorites      int    `json:"favorites"`
	Subscriptions  int    `json:"subscriptions"`
	DisabledSubs   int    `json:"disabled_subscriptions"`
	Episodes       int    `json:"episodes"`
	AdsRemoved     int    `json:"episodes_with_ads_removed"`
	WithTranscript int    `json:"episodes_with_transcript"`

	TotalBytes      int64 `json:"total_bytes"`
	AudioBytes      int64 `json:"audio_bytes"`
	OriginalBytes   int64 `json:"original_bytes"`
	TranscriptBytes int64 `json:"transcript_bytes"`
	WorkBytes       int64 `json:"work_bytes"`
	OtherBytes      int64 `json:"other_bytes"`

	DiskTotal uint64 `json:"disk_total_bytes"`
	DiskFree  uint64 `json:"disk_free_bytes"`

	// Largest are the podcasts using the most space, biggest first.
	Largest []PodcastUsage `json:"largest"`

	// PrunableBytes is what keeping only the newest PrunableKeep episodes of
	// each non-favorite podcast would free.
	PrunableKeep  int   `json:"prunable_keep"`
	PrunableBytes int64 `json:"prunable_bytes"`
}

// StatsOptions tunes LibraryStats.
type StatsOptions struct {
	// Top is how many of the largest podcasts to list.
	Top int
	// PruneKeep is the episode count used to estimate reclaimable space.
	PruneKeep int
}

// Stats walks the library and totals its disk use. It reads only; a podcast
// directory reached twice through a symlink is counted once.
func (l *Library) Stats(opts StatsOptions) LibraryStats {
	st := LibraryStats{Root: l.cfg.PodcastsDir, PrunableKeep: opts.PruneKeep}
	entries := l.Podcasts()
	seen := map[string]bool{}
	var usage []PodcastUsage
	for _, entry := range entries {
		real, err := filepath.EvalSymlinks(entry.Dir)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		usage = append(usage, st.addPodcast(entry))
	}
	st.Podcasts = len(usage)
	sort.Slice(usage, func(i, j int) bool { return usage[i].Bytes > usage[j].Bytes })
	st.Largest = usage[:min(len(usage), max(opts.Top, 0))]
	st.addSubscriptions(l)
	st.DiskTotal, st.DiskFree = diskSpace(l.cfg.PodcastsDir)
	if opts.PruneKeep > 0 {
		st.PrunableBytes = prunableBytes(l.PlanKeepLatest(entries, PruneOptions{Keep: opts.PruneKeep, SkipFavorites: true}))
	}
	return st
}

func (st *LibraryStats) addSubscriptions(l *Library) {
	store, err := l.Subscriptions()
	if err != nil {
		return
	}
	for _, sub := range store.List() {
		st.Subscriptions++
		if sub.Disabled {
			st.DisabledSubs++
		}
	}
}

func prunableBytes(plans []PrunePlan) int64 {
	var total int64
	for _, p := range plans {
		total += p.Bytes()
	}
	return total
}

func (st *LibraryStats) addPodcast(entry PodcastDirEntry) PodcastUsage {
	use := PodcastUsage{Title: entry.Title}
	if config.LoadPodcastConfig(entry.Dir, config.PodcastConfig{}).Favorite {
		use.Favorite = true
		st.Favorites++
	}
	_ = filepath.WalkDir(entry.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		use.Bytes += info.Size()
		st.countFile(entry.Dir, path, info.Size(), &use)
		return nil
	})
	st.TotalBytes += use.Bytes
	return use
}

// countFile files one file under the category its name gives it.
func (st *LibraryStats) countFile(root, path string, size int64, use *PodcastUsage) {
	name := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(path, string(filepath.Separator)+util.WorkDirName+string(filepath.Separator)):
		st.WorkBytes += size
	case strings.HasSuffix(name, ".mp3.precut") || strings.HasSuffix(name, "precut.mp3"):
		st.OriginalBytes += size
		st.AdsRemoved++
	case strings.HasSuffix(name, ".mp3"):
		st.AudioBytes += size
		st.Episodes++
		use.Episodes++
	case isTranscriptFile(name):
		st.TranscriptBytes += size
		if strings.HasSuffix(name, ".transcript.json") {
			st.WithTranscript++
		}
	default:
		st.OtherBytes += size
	}
}

func isTranscriptFile(name string) bool {
	for _, suffix := range []string{".transcript.json", ".cuts.json", ".ads.truth.json", ".srt", ".txt", ".mp3.json"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
