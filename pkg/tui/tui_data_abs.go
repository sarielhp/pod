package tui

import (
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/episode"
	"pod/pkg/podcast"
)

func loadTUIPodcastsABS(podcastsDir string, b backend.Backend) ([]tuiPodcast, error) {
	podcasts, err := loadTUIPodcasts(podcastsDir)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return podcasts, nil
	}

	allItems, err := b.Podcasts()
	if err != nil {
		return podcasts, nil
	}

	itemByRel := buildABSItemsIndex(allItems)

	for i := range podcasts {
		pod := &podcasts[i]
		itemSummary, ok := itemByRel[pod.name]
		if !ok {
			itemSummary, ok = itemByRel[filepath.Base(pod.dir)]
		}
		if ok {
			enrichSinglePodcastABS(pod, b, itemSummary)
		}
	}

	return podcasts, nil
}

func buildABSItemsIndex(items []backend.Podcast) map[string]backend.Podcast {
	itemByRel := make(map[string]backend.Podcast, len(items)*2)
	for _, item := range items {
		itemByRel[item.RelPath] = item
		cleanRel := filepath.Base(item.RelPath)
		itemByRel[cleanRel] = item
		if item.Media.Metadata.Title != "" {
			itemByRel[item.Media.Metadata.Title] = item
		}
	}
	return itemByRel
}

func enrichSinglePodcastABS(pod *tuiPodcast, b backend.Backend, itemSummary backend.Podcast) {
	fullItem, err := b.GetPodcast(itemSummary.ID)
	if err != nil || fullItem == nil {
		return
	}
	pod.absData = fullItem
	if fullItem.Media.Metadata.Author != "" {
		pod.author = fullItem.Media.Metadata.Author
	}
	if fullItem.Media.Metadata.Description != "" {
		pod.description = fullItem.Media.Metadata.Description
	}
	if fullItem.Media.Metadata.FeedURL != "" {
		pod.feedURL = fullItem.Media.Metadata.FeedURL
	}

	cDir := podcast.CacheDirForPodcast(pod.dir)
	coverDest := filepath.Join(cDir, "cover.jpg")
	_ = b.DownloadCover(fullItem.ID, coverDest)

	quarantined := episode.QuarantineAbandonedDuplicates(pod.dir, fullItem.Media.Episodes)
	pod.notice = episode.FormatQuarantinedSummary(quarantined, pod.name)

	episodeMap := buildABSEpisodeMap(fullItem.Media.Episodes)
	matchAndEnrichEpisodes(pod.episodes, episodeMap)

	sort.Slice(pod.episodes, func(x, y int) bool {
		return pod.episodes[x].displayDate().After(pod.episodes[y].displayDate())
	})

	savePodcastToCache(pod)
}

func buildABSEpisodeMap(episodes []backend.Episode) map[string]*backend.Episode {
	episodeMap := make(map[string]*backend.Episode, len(episodes)*3)
	for epIdx := range episodes {
		ep := &episodes[epIdx]
		if ep.AudioFile != nil && ep.AudioFile.Metadata != nil {
			if ep.AudioFile.Metadata.Filename != "" {
				episodeMap[ep.AudioFile.Metadata.Filename] = ep
				episodeMap[episode.NormalizeEpisodeTitle(ep.AudioFile.Metadata.Filename)] = ep
			}
			if ep.AudioFile.Metadata.RelPath != "" {
				cleanRel := filepath.Base(ep.AudioFile.Metadata.RelPath)
				episodeMap[cleanRel] = ep
				episodeMap[episode.NormalizeEpisodeTitle(cleanRel)] = ep
			}
		}
		if ep.Title != "" {
			episodeMap[ep.Title] = ep
			episodeMap[ep.Title+".mp3"] = ep
			episodeMap[episode.NormalizeEpisodeTitle(ep.Title)] = ep
		}
	}
	return episodeMap
}

func matchAndEnrichEpisodes(episodes []tuiEpisode, episodeMap map[string]*backend.Episode) {
	for j := range episodes {
		ep := &episodes[j]
		var matchedEp *backend.Episode
		if absEp, exists := episodeMap[ep.filename]; exists {
			matchedEp = absEp
		} else if absEp, exists := episodeMap[strings.TrimSuffix(ep.filename, ".mp3")]; exists {
			matchedEp = absEp
		} else if absEp, exists := episodeMap[episode.NormalizeEpisodeTitle(ep.filename)]; exists {
			matchedEp = absEp
		}
		if matchedEp != nil {
			ep.absData = matchedEp
			if matchedEp.Title != "" {
				ep.title = matchedEp.Title
			}
			if pub := episode.ParseABSEpisodePublishedAt(matchedEp); pub > 0 {
				ep.publishedAt = pub
			}
			if matchedEp.Duration > 0 {
				ep.duration = matchedEp.Duration
			}
			if matchedEp.Season != "" {
				ep.season = matchedEp.Season
			}
			if matchedEp.Episode != "" {
				ep.episode = matchedEp.Episode
			}
		}
	}
}
