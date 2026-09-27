package cli

import (
	"fmt"
	"path/filepath"
	"pod/pkg/audio"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strings"
	"time"
)

func collectQueueDisplayItems(p podcast.PodcastDirEntry) ([]queueEpisodeItem, error) {
	entries, err := episode.ReadQueue(p.Dir)
	if err != nil {
		return nil, err
	}
	metadata := queueEpisodeMetadata(p.Dir)
	items := make([]queueEpisodeItem, 0, len(entries))
	for _, entry := range entries {
		item := queueEpisodeItem{PodcastID: p.ShortID, PodcastDir: p.Dir, Filename: entry}
		path, err := episode.ResolveQueueAudioPath(p.Dir, entry)
		if err != nil {
			item.Title = "Unresolved queue entry"
			if strings.Contains(err.Error(), "ambiguous") {
				item.Title = "Ambiguous legacy entry"
			}
			item.ResolutionError = err.Error()
		} else {
			item.AudioPath = path
			item.EpisodeID = podcast.EpisodeShortIDReadOnly(p.Dir, p.ShortID, path)
			populateQueueEpisodeDetails(&item, metadata[filepath.Clean(path)])
		}
		items = append(items, item)
	}
	return items, nil
}

func queueEpisodeMetadata(dir string) map[string]podcast.CachedEpisodeSummary {
	metadata := make(map[string]podcast.CachedEpisodeSummary)
	cache, _ := podcast.LoadPodcastCache(dir)
	if cache == nil {
		return metadata
	}
	for _, ep := range cache.Episodes {
		path := ep.Path
		if path == "" {
			path = ep.Filename
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		metadata[filepath.Clean(path)] = ep
	}
	return metadata
}

func populateQueueEpisodeDetails(item *queueEpisodeItem, ep podcast.CachedEpisodeSummary) {
	item.Title = strings.TrimSpace(ep.Title)
	if item.Title == "" {
		item.Title = podcast.EpisodeTitleFromPath(item.AudioPath)
	}
	item.DurationSec = ep.DurationSec
	if ep.PublishedAt > 0 {
		item.PublishedAt = time.UnixMilli(ep.PublishedAt).Format(time.RFC3339)
	}
	if date, ok := podcast.SourcePublicationTime(item.AudioPath); ok {
		item.PublishedAt = ""
		if !date.IsZero() {
			item.PublishedAt = date.UTC().Format(time.RFC3339)
		}
	}
	if st, err := episode.LoadEpisodeStatus(episode.StatusPathFor(item.AudioPath)); err == nil && st != nil {
		if st.Original.DurationSec > 0 {
			item.DurationSec = st.Original.DurationSec
		}
		if st.Cleaned.DurationSec > 0 {
			item.DurationSec = st.Cleaned.DurationSec
		}
	}
	if item.DurationSec <= 0 {
		item.DurationSec = audio.GetAudioDuration(item.AudioPath)
	}
}

func queueDisplayCells(item queueEpisodeItem) []string {
	id, length, published := item.EpisodeID, "—", "—"
	if id == "" {
		id = "—"
	}
	if item.DurationSec > 0 {
		minutes := int(item.DurationSec / 60)
		length = fmt.Sprintf("%2d:%02d", minutes/60, minutes%60)
	}
	if date, err := time.Parse(time.RFC3339, item.PublishedAt); err == nil {
		published = queuePublicationTime(date, time.Now())
	}
	return []string{item.PodcastID, id, fmt.Sprint(item.Priority), length, published, shortenedQueueTitle(item.Title)}
}

func queuePublicationTime(date, now time.Time) string {
	local := date.In(now.Location())
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	age := int(today.Sub(day).Hours() / 24)
	label := local.Format("2006-01-02")
	switch {
	case age == 0:
		label = "today"
	case age == 1:
		label = "yesterday"
	case age >= 2 && age <= 10:
		label = fmt.Sprintf("-%dd", age)
	case age < 0:
		label = fmt.Sprintf("+%dd", -age)
	}
	return fmt.Sprintf("%s %2d:%02d", label, local.Hour(), local.Minute())
}

func shortenedQueueTitle(title string) string {
	var words []string
	for _, word := range strings.Fields(title) {
		key := strings.ToLower(strings.Trim(word, "\"'“”‘’.,:;!?()[]{}"))
		if key != "the" && key != "show" && key != "with" {
			words = append(words, word)
		}
	}
	shortened := strings.Join(words, " ")
	if shortened == "" {
		shortened = title
	}
	return util.TruncateDisplayName(shortened, 48)
}
