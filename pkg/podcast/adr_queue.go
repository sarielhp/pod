package podcast

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/pipeline"
	"pod/pkg/util"
)

// QueueItem is one episode waiting for ad removal. It carries where the audio
// is and enough identity to name it back to the user; Priority is filled in by
// SortQueueItems.
//
// ResolutionError records an entry whose audio could not be located, so a
// listing can show the problem rather than dropping the row.
type QueueItem struct {
	PodcastID       string  `json:"podcast_id"`
	EpisodeID       string  `json:"episode_id"`
	Title           string  `json:"title"`
	AudioPath       string  `json:"audio_path"`
	PodcastDir      string  `json:"podcast_dir"`
	Filename        string  `json:"filename"`
	DurationSec     float64 `json:"duration_sec"`
	PublishedAt     string  `json:"published_at,omitempty"`
	ResolutionError string  `json:"resolution_error,omitempty"`
	Priority        int     `json:"priority"`
}

// QueueFilename is how an episode is recorded in a podcast's queue: relative
// to the podcast directory when it sits inside it, and the bare base name
// otherwise.
func QueueFilename(podDir, audioPath string) string {
	rel, err := filepath.Rel(podDir, audioPath)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return filepath.Base(audioPath)
}

// queueEntryMatchesAudio reports whether a queue entry names a given audio file.
func queueEntryMatchesAudio(podDir, queuedFilename, audioPath string) bool {
	path, err := pipeline.ResolveQueueAudioPath(podDir, queuedFilename)
	return err == nil && filepath.Clean(path) == filepath.Clean(audioPath)
}

// SortQueueItems orders items by ad-removal priority, highest first, keeping
// equal priorities in their original order.
func SortQueueItems(items []QueueItem) {
	for i := range items {
		items[i].Priority = EpisodePriority(items[i].PodcastDir, items[i].AudioPath)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Priority > items[j].Priority })
}

// QueuePodcasts lists the library's podcasts without assigning short IDs, so
// that reading the queue never writes to a podcast's configuration.
func (l *Library) QueuePodcasts() []PodcastDirEntry {
	return ScanPodcastDirsReadOnly(l.cfg.PodcastsDir)
}

// PodcastQueue returns the ad-removal queue of one podcast.
func PodcastQueue(p PodcastDirEntry) ([]QueueItem, error) {
	filenames, err := pipeline.ReadQueue(p.Dir)
	if err != nil {
		return nil, err
	}
	var list []QueueItem
	for _, fn := range filenames {
		mp3Path, err := pipeline.ResolveQueueAudioPath(p.Dir, fn)
		if err != nil {
			return nil, fmt.Errorf("queue %s: %w", p.Dir, err)
		}
		title := EpisodeTitleFromPath(mp3Path)
		if title == "" {
			title = util.StripExt(fn)
		}
		list = append(list, QueueItem{
			PodcastID:  p.ShortID,
			EpisodeID:  EpisodeShortIDReadOnly(p.Dir, p.ShortID, mp3Path),
			Title:      title,
			AudioPath:  mp3Path,
			PodcastDir: p.Dir,
			Filename:   fn,
		})
	}
	return list, nil
}

// QueueItems returns the ad-removal queue across the library, or just the
// part of it that target names — a podcast, or a single queued episode.
func (l *Library) QueueItems(target string) ([]QueueItem, error) {
	if target == "" {
		var all []QueueItem
		for _, p := range l.QueuePodcasts() {
			items, err := PodcastQueue(p)
			if err != nil {
				return nil, err
			}
			all = append(all, items...)
		}
		return all, nil
	}

	res, err := l.ResolveQueueTarget(target)
	if err != nil {
		return nil, err
	}
	switch {
	case res.IsPodcast():
		return PodcastQueue(PodcastDirEntry{
			Dir:        res.Podcast.Dir,
			FolderName: res.Podcast.FolderName,
			Title:      res.Podcast.Title,
			ShortID:    res.Podcast.ShortID,
		})
	case res.IsEpisode():
		return l.queuedEpisode(res)
	}
	return nil, fmt.Errorf("unrecognized target %q", target)
}

func (l *Library) queuedEpisode(res *ResolvedID) ([]QueueItem, error) {
	items, err := PodcastQueue(PodcastDirEntry{
		Dir:        res.Episode.PodcastDir,
		FolderName: filepath.Base(res.Episode.PodcastDir),
		Title:      res.Episode.PodcastTitle,
		ShortID:    res.Episode.PodcastShortID,
	})
	if err != nil {
		return nil, err
	}
	var matched []QueueItem
	for _, it := range items {
		if queueEntryMatchesAudio(it.PodcastDir, it.Filename, res.Episode.Path) {
			matched = append(matched, it)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("episode %q [%s] is not in the AdR queue", res.Episode.Filename, res.Episode.ShortID)
	}
	return matched, nil
}

// ResolveQueueTarget turns a query into the podcast or queued episode it
// names. It differs from Resolve in that episodes are matched only among
// queue-eligible audio, and no short ID is assigned as a side effect.
func (l *Library) ResolveQueueTarget(query string) (*ResolvedID, error) {
	podcasts := l.QueuePodcasts()
	matchedPod, err := MatchLocalPodcasts(podcasts, query)
	if err != nil {
		if errors.Is(err, ErrAmbiguousPodcast) {
			return nil, err
		}
	} else if matchedPod != nil {
		return &ResolvedID{
			Type: ResolvedTypePodcast,
			Podcast: &ResolvedPodcast{
				Dir:        matchedPod.Dir,
				Title:      matchedPod.Title,
				ShortID:    matchedPod.ShortID,
				FolderName: matchedPod.FolderName,
			},
		}, nil
	}

	matches := matchQueueEpisodes(podcasts, query)
	if len(matches) == 1 {
		return matches[0], nil
	}
	return nil, fmt.Errorf("queue target %q matches %d episodes; use a unique ID or path", query, len(matches))
}

func matchQueueEpisodes(podcasts []PodcastDirEntry, query string) []*ResolvedID {
	var matches []*ResolvedID
	for _, p := range podcasts {
		for _, path := range util.FindMP3Files(p.Dir) {
			if !pipeline.IsQueueAudioPath(path) {
				continue
			}
			id := EpisodeShortIDReadOnly(p.Dir, p.ShortID, path)
			title := EpisodeTitleFromPath(path)
			if !strings.EqualFold(query, id) && query != path && query != filepath.Base(path) &&
				query != title && query != QueueFilename(p.Dir, path) {
				continue
			}
			matches = append(matches, &ResolvedID{
				Type: ResolvedTypeEpisode,
				Episode: &ResolvedEpisode{
					Path:           path,
					Filename:       filepath.Base(path),
					ShortID:        id,
					Title:          title,
					PodcastDir:     p.Dir,
					PodcastTitle:   p.Title,
					PodcastShortID: p.ShortID,
				},
			})
		}
	}
	return matches
}

// EnqueuePodcast adds every uncleaned episode of a podcast to its ad-removal
// queue, skipping those already queued, and returns how many were added.
func EnqueuePodcast(podDir string) (int, error) {
	var candidates []string
	for _, mp3 := range util.FindMP3Files(podDir) {
		if pipeline.IsQueueAudioPath(mp3) && !pipeline.IsEpisodeClean(mp3) {
			candidates = append(candidates, QueueFilename(podDir, mp3))
		}
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	added := 0
	err := pipeline.UpdateQueue(podDir, func(entries []string) []string {
		existing := make(map[string]bool, len(entries))
		for _, e := range entries {
			existing[strings.ToLower(e)] = true
		}
		for _, fn := range candidates {
			if existing[strings.ToLower(fn)] {
				continue
			}
			entries = append(entries, fn)
			existing[strings.ToLower(fn)] = true
			added++
		}
		return entries
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

// ClearPodcastQueue empties one podcast's ad-removal queue, also clearing the
// stored priority of everything that was in it.
func ClearPodcastQueue(podDir string) error {
	entries, err := pipeline.ReadQueue(podDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path, err := pipeline.ResolveQueueAudioPath(podDir, entry)
		if err != nil {
			continue
		}
		if err := pipeline.ClearQueuePriority(path); err != nil {
			return err
		}
	}
	return pipeline.UpdateQueue(podDir, func([]string) []string { return []string{} })
}

// ClearAllQueues empties every podcast queue in the library and returns how
// many podcasts had a queue file. The count is of queue files present, not of
// non-empty queues: a podcast whose queue is already empty still counts, which
// is what the reported total has always meant.
func (l *Library) ClearAllQueues() (int, error) {
	cleared := 0
	for _, p := range l.QueuePodcasts() {
		if _, err := os.Stat(filepath.Join(p.Dir, pipeline.QueueFileName)); err != nil {
			continue
		}
		if err := ClearPodcastQueue(p.Dir); err != nil {
			return cleared, err
		}
		cleared++
	}
	return cleared, nil
}
