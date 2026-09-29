package podcast

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pod/pkg/episode"
	"pod/pkg/util"
)

// rewindSidecarSuffixes are the names, relative to an episode's stem, of the
// files that only make sense beside its audio. A rewound episode leaves none of
// them behind, or fetch would find a transcript for audio it is about to
// download again.
var rewindSidecarSuffixes = []string{
	".mp3", ".mp3.precut", ".mp3.json", ".mp3.bak", ".mp3.lock", ".mp3.json.lock",
	".cuts.json", ".transcript.json", ".transcript.txt", ".srt", ".txt",
}

// RewindOptions selects what a rewind removes.
type RewindOptions struct {
	// Since is how far back to reach: episodes whose audio file was written
	// within this window are rewound.
	Since time.Duration
	// Target narrows the rewind to matching subscriptions. Empty means all.
	Target string
	// Now is the reference time; zero means the current time.
	Now time.Time
}

// RewindEpisode is one downloaded episode and everything stored beside it.
type RewindEpisode struct {
	Audio   string
	ModTime time.Time
	Files   []string
	Bytes   int64
}

// RewindPlan is what rewinding one subscription would remove.
type RewindPlan struct {
	Sub        Subscription
	PodDir     string
	Episodes   []RewindEpisode
	CacheFiles []string
	Skipped    []string
	Err        error
}

// RewindResult totals what ApplyRewind removed.
type RewindResult struct {
	Podcasts int
	Episodes int
	Files    int
	Bytes    int64
	Failures []error
}

// ParseRewindWindow reads a window such as "24h", "90m" or "7d". A bare number
// is rejected, since it does not say whether it counts hours or days.
func ParseRewindWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	var d time.Duration
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseFloat(days, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid rewind window %q", s)
		}
		d = time.Duration(n * float64(24*time.Hour))
	} else {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid rewind window %q (use e.g. 24h, 90m, 7d)", s)
		}
		d = parsed
	}
	if d <= 0 {
		return 0, fmt.Errorf("rewind window must be positive, got %q", s)
	}
	return d, nil
}

// Bytes is the disk space the plan would free.
func (p RewindPlan) Bytes() int64 {
	var total int64
	for _, ep := range p.Episodes {
		total += ep.Bytes
	}
	return total
}

// PlanRewind lists, without touching anything, the episodes downloaded inside
// the window. Age is the audio file's modification time, because pod records no
// separate download time; an episode whose audio was rewritten since (ad
// removal, retagging) looks newer than it is.
func (l *Library) PlanRewind(subs []Subscription, opts RewindOptions) []RewindPlan {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.Add(-opts.Since)
	var plans []RewindPlan
	for _, sub := range subs {
		if opts.Target != "" && !SubscriptionMatches(sub, opts.Target) {
			continue
		}
		plans = append(plans, planPodcastRewind(sub, l.PodcastDir(sub), cutoff))
	}
	return plans
}

func planPodcastRewind(sub Subscription, podDir string, cutoff time.Time) RewindPlan {
	plan := RewindPlan{Sub: sub, PodDir: podDir}
	audio, err := util.FindMP3FilesErr(podDir)
	if err != nil && !os.IsNotExist(err) {
		plan.Err = err
		return plan
	}
	sort.Strings(audio)
	for _, path := range audio {
		fi, statErr := os.Stat(path)
		if statErr != nil || !fi.ModTime().After(cutoff) {
			continue
		}
		if episode.IsEpisodeInRemoteFlight(path) {
			plan.Skipped = append(plan.Skipped, path+" (processing remotely)")
			continue
		}
		plan.Episodes = append(plan.Episodes, rewindEpisodeFor(path, fi.ModTime()))
	}
	if len(plan.Episodes) > 0 {
		plan.CacheFiles = podcastCacheFiles(podDir, plan.Episodes)
	}
	return plan
}

func rewindEpisodeFor(audio string, modTime time.Time) RewindEpisode {
	ep := RewindEpisode{Audio: audio, ModTime: modTime}
	stem := strings.TrimSuffix(filepath.Base(audio), filepath.Ext(audio))
	dir := filepath.Dir(audio)
	ep.Files = append(ep.Files, sidecarsIn(dir, stem, isRewindSidecar)...)
	ep.Files = append(ep.Files, sidecarsIn(filepath.Join(dir, util.WorkDirName), stem, isWorkFileOf)...)
	for _, path := range ep.Files {
		if fi, err := os.Stat(path); err == nil {
			ep.Bytes += fi.Size()
		}
	}
	return ep
}

func sidecarsIn(dir, stem string, belongs func(rest string) bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		rest, ok := strings.CutPrefix(entry.Name(), stem)
		if ok && belongs(rest) {
			found = append(found, filepath.Join(dir, entry.Name()))
		}
	}
	return found
}

func isRewindSidecar(rest string) bool {
	for _, suffix := range rewindSidecarSuffixes {
		if strings.EqualFold(rest, suffix) {
			return true
		}
	}
	return false
}

// isWorkFileOf accepts anything a pipeline stage left in .work/ under the
// episode's name, whatever its extension.
func isWorkFileOf(rest string) bool {
	return strings.HasPrefix(rest, ".")
}

func podcastCacheFiles(podDir string, episodes []RewindEpisode) []string {
	names := make([]string, 0, len(episodes))
	for _, ep := range episodes {
		names = append(names, filepath.Base(ep.Audio))
	}
	return staleCacheFiles(podDir, names)
}

// staleCacheFiles are the cached index of a podcast and the cached details of
// the named episodes, which go stale when those episodes are removed. Both are
// rebuilt on demand.
func staleCacheFiles(podDir string, audioNames []string) []string {
	cacheDir := CacheDirForPodcast(podDir)
	files := []string{filepath.Join(cacheDir, "index.json")}
	for _, name := range audioNames {
		files = append(files, filepath.Join(cacheDir, "details", detailFileName(name)))
	}
	return files
}

// ApplyRewind carries out the plans: deletes each episode's files, drops them
// from the podcast's processing queue, resets the feed's freshness markers so
// the next check sees the episodes as new, and republishes the affected feeds.
func (l *Library) ApplyRewind(plans []RewindPlan) RewindResult {
	var res RewindResult
	var rewound []Subscription
	for _, plan := range plans {
		if plan.Err != nil || len(plan.Episodes) == 0 {
			continue
		}
		n, failures := l.rewindPodcast(plan, &res)
		res.Failures = append(res.Failures, failures...)
		if n == 0 {
			continue
		}
		res.Podcasts++
		res.Episodes += n
		rewound = append(rewound, plan.Sub)
	}
	res.Failures = append(res.Failures, l.republishRewound(rewound)...)
	return res
}

func (l *Library) rewindPodcast(plan RewindPlan, res *RewindResult) (int, []error) {
	var failures []error
	var removed []string
	for _, ep := range plan.Episodes {
		files, bytes, err := removeRewindEpisode(ep)
		res.Files += files
		res.Bytes += bytes
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", plan.Sub.Title, err))
			continue
		}
		removed = append(removed, filepath.Base(ep.Audio))
	}
	for _, path := range plan.CacheFiles {
		_ = os.Remove(path)
	}
	if err := dropFromQueue(plan.PodDir, removed); err != nil {
		failures = append(failures, fmt.Errorf("%s: %w", plan.Sub.Title, err))
	}
	if len(removed) > 0 && l.feedCache != nil {
		l.feedCache.ResetFreshness(plan.Sub.FeedURL)
	}
	return len(removed), failures
}

func removeRewindEpisode(ep RewindEpisode) (int, int64, error) {
	lock, err := util.AcquireFileLock(ep.Audio)
	if err != nil {
		return 0, 0, fmt.Errorf("lock %s: %w", ep.Audio, err)
	}
	if lock == nil {
		return 0, 0, fmt.Errorf("%s is being processed by another pod instance", ep.Audio)
	}
	defer lock.Release()
	defer os.Remove(ep.Audio + ".lock")
	if episode.IsEpisodeInRemoteFlight(ep.Audio) {
		return 0, 0, fmt.Errorf("%s is processing remotely", ep.Audio)
	}

	files, freed := 0, int64(0)
	for _, path := range ep.Files {
		size := fileSize(path)
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return files, freed, fmt.Errorf("remove %s: %w", path, err)
		}
		files++
		freed += size
	}
	return files, freed, nil
}

func fileSize(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.Size()
	}
	return 0
}

func dropFromQueue(podDir string, filenames []string) error {
	if len(filenames) == 0 {
		return nil
	}
	gone := make(map[string]bool, len(filenames))
	for _, name := range filenames {
		gone[name] = true
	}
	return episode.UpdateQueue(podDir, func(entries []string) []string {
		var kept []string
		for _, entry := range entries {
			if !gone[filepath.Base(entry)] {
				kept = append(kept, entry)
			}
		}
		return kept
	})
}

func (l *Library) republishRewound(subs []Subscription) []error {
	if len(subs) == 0 {
		return nil
	}
	var failures []error
	if l.feedCache != nil {
		if err := l.feedCache.Save(); err != nil {
			failures = append(failures, fmt.Errorf("save feed cache: %w", err))
		}
	}
	for _, sub := range subs {
		if err := l.Publish(sub, nil); err != nil {
			failures = append(failures, fmt.Errorf("%s: republish: %w", sub.Title, err))
		}
	}
	store, err := l.Subscriptions()
	if err != nil {
		return append(failures, err)
	}
	if err := l.PublishCatalog(store.List()); err != nil {
		failures = append(failures, fmt.Errorf("republish catalog: %w", err))
	}
	return failures
}

// ResetFreshness forgets what the cache remembers about a feed being up to
// date, so the next check downloads the whole feed and finds the episodes new.
// It keeps the publication history and cover, which republishing still needs.
// It reports whether the feed had an entry.
func (m *FeedCacheManager) ResetFreshness(feedURL string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[feedURL]
	if !ok || entry == nil {
		return false
	}
	entry.ETag = ""
	entry.LastModified = ""
	entry.LatestGUID = ""
	entry.LastBuildDate = ""
	entry.ChannelPubDate = ""
	entry.EpisodeCount = 0
	m.dirty = true
	return true
}
