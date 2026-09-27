package tui

import (
	"pod/pkg/episode"
	"pod/pkg/podcast"
)

// downloadQueueItemFor builds the download-queue entry for one episode.
//
// Every screen that enqueues a download goes through this. They each used to
// build the struct inline, and the four copies had drifted: the Latest
// Episodes screen lost the enclosure URL and the GUID fallback, so an episode
// queued from there reached the worker with no URL to fetch. The standalone
// backend skips an episode with no enclosure and returns no error, so the
// queue marked it completed and the episode was never downloaded.
func downloadQueueItemFor(podTitle, podDir, podID string, ep tuiEpisode) podcast.DownloadQueueItem {
	guid := ep.guid
	pubDate := ""
	pubAt := ep.publishedAt
	if ep.absData != nil {
		if ep.absData.ID != "" {
			guid = ep.absData.ID
		}
		pubDate = ep.absData.PubDate
		if at := episode.ParseABSEpisodePublishedAt(ep.absData); at > 0 {
			pubAt = at
		}
	}
	return podcast.DownloadQueueItem{
		PodcastTitle: podTitle,
		PodcastDir:   podDir,
		PodcastID:    podID,
		EpisodeTitle: ep.displayTitle(),
		GUID:         guid,
		PubDate:      pubDate,
		PublishedAt:  pubAt,
		DurationSec:  ep.duration,
		EnclosureURL: ep.enclosureURL,
	}
}

// podcastBackendID is the backend's own id for a podcast, when it has one.
func podcastBackendID(pod *tuiPodcast) string {
	if pod != nil && pod.absData != nil {
		return pod.absData.ID
	}
	return ""
}
