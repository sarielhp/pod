package podcast

import (
	"fmt"
	"time"

	"pod/pkg/backend"
	"pod/pkg/types"
)

type PodcastFreqResult struct {
	Title       string
	Item        backend.Podcast
	Freq        types.PodcastFrequencyInfo
	PodDir      string
	Disabled    bool
	PolicySaved bool
	Err         error
}

// freqCacheEpisodeLimit caps how much publication history is cached per feed.
// The analysis only needs enough recent episodes to establish a cadence, and
// some feeds carry thousands.
const freqCacheEpisodeLimit = 100

func getEpisodesForFrequency(client backend.Backend, item backend.Podcast, podcastsDir string, refresh bool, feedCache *FeedCacheManager) ([]backend.FeedEpisode, error) {
	if feedCache == nil {
		feedCache = defaultFeedCache()
	}
	feedURL := item.Media.Metadata.FeedURL
	podDir := findPodcastDirForItem(item, podcastsDir)

	if !refresh {
		if feedURL != "" {
			if entry := feedCache.Get(feedURL); entry != nil && !entry.IsExpired(FeedCacheDefaultTTL) {
				if eps := entry.FeedEpisodes(); len(eps) > 0 {
					return eps, nil
				}
			}
		}
		if podDir != "" {
			if eps := loadCachedFeedEpisodes(podDir); len(eps) > 0 {
				return eps, nil
			}
		}
	}

	if client != nil && feedURL != "" {
		if feedEpisodes, err := client.PodcastFeedEpisodes(feedURL); err == nil && len(feedEpisodes) > 0 {
			takeCount := min(freqCacheEpisodeLimit, len(feedEpisodes))
			cachedEps := feedEpisodes[:takeCount]
			feedCache.Put(feedURL, &FeedCacheEntry{
				FeedURL:     feedURL,
				LastChecked: time.Now(),
				PubDates:    pubDatesFromEpisodes(cachedEps),
			})
			// Without this the entry lived only for the current process, so the
			// cache never spared a later run any work.
			_ = feedCache.Save()
			return cachedEps, nil
		}
	}

	if len(item.Media.Episodes) > 0 {
		var eps []backend.FeedEpisode
		for _, ep := range item.Media.Episodes {
			eps = append(eps, backend.FeedEpisode{
				Title:       ep.Title,
				PubDate:     ep.PubDate,
				PublishedAt: ep.PublishedAt,
			})
		}
		return eps, nil
	}

	if podDir != "" {
		if eps := loadCachedFeedEpisodes(podDir); len(eps) > 0 {
			return eps, nil
		}
	}

	return nil, fmt.Errorf("no episodes available")
}

func loadCachedFeedEpisodes(podDir string) []backend.FeedEpisode {
	if cache, _ := LoadPodcastCache(podDir); cache != nil && len(cache.Episodes) > 0 {
		eps := make([]backend.FeedEpisode, len(cache.Episodes))
		for i, ep := range cache.Episodes {
			eps[i] = backend.FeedEpisode{Title: ep.Title, PublishedAt: ep.PublishedAt}
		}
		return eps
	}
	return nil
}
