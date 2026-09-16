package podcast

import (
	"fmt"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
)

func selectEpisodesByDownloadPolicy(sortedCatalog []backend.FeedEpisode, isDownloaded func(ep backend.FeedEpisode) bool, policy string, k int, oldest bool) ([]backend.FeedEpisode, []string) {
	normPolicy := config.NormalizeDownloadPolicy(policy)
	if k <= 0 {
		k = 3
	}

	switch normPolicy {
	case config.DownloadPolicyNone:
		return nil, []string{"policy: none"}
	case config.DownloadPolicyLatest:
		return selectLatestEpisode(sortedCatalog, isDownloaded, oldest)
	case config.DownloadPolicyLatestK:
		return selectLatestKEpisodes(sortedCatalog, isDownloaded, k, oldest)
	case config.DownloadPolicyAll:
		return selectAllEpisodes(sortedCatalog, isDownloaded, oldest)
	case config.DownloadPolicyNew:
		return selectNewEpisodes(sortedCatalog, nil, isDownloaded, nil)
	default:
		return nil, nil
	}
}

func selectNewEpisodes(sortedCatalog []backend.FeedEpisode, downloadedIndices []int, isDownloaded func(backend.FeedEpisode) bool, favoriteSince *time.Time) ([]backend.FeedEpisode, []string) {
	if len(sortedCatalog) == 0 {
		return nil, nil
	}

	if len(downloadedIndices) == 0 && isDownloaded != nil {
		for idx, ep := range sortedCatalog {
			if isDownloaded(ep) {
				downloadedIndices = append(downloadedIndices, idx)
			}
		}
	}

	if len(downloadedIndices) > 0 {
		maxIdx := -1
		for _, idx := range downloadedIndices {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		var newEpisodes []backend.FeedEpisode
		if maxIdx+1 < len(sortedCatalog) {
			for _, ep := range sortedCatalog[maxIdx+1:] {
				if hasDownloadableEnclosure(ep) && !isDownloaded(ep) {
					newEpisodes = append(newEpisodes, ep)
				}
			}
		}
		if len(newEpisodes) > 0 {
			for i, j := 0, len(newEpisodes)-1; i < j; i, j = i+1, j-1 {
				newEpisodes[i], newEpisodes[j] = newEpisodes[j], newEpisodes[i]
			}
			return newEpisodes, []string{fmt.Sprintf("%d new episode(s) (policy: new)", len(newEpisodes))}
		}
		return nil, nil
	}

	var newEpisodes []backend.FeedEpisode
	if favoriteSince != nil {
		cutoffMS := favoriteSince.UnixMilli()
		for _, ep := range sortedCatalog {
			if GetPubMS(ep) >= cutoffMS && hasDownloadableEnclosure(ep) && !isDownloaded(ep) {
				newEpisodes = append(newEpisodes, ep)
			}
		}
	}
	if len(newEpisodes) == 0 {
		return nil, nil
	}
	for i, j := 0, len(newEpisodes)-1; i < j; i, j = i+1, j-1 {
		newEpisodes[i], newEpisodes[j] = newEpisodes[j], newEpisodes[i]
	}
	return newEpisodes, []string{fmt.Sprintf("%d new episode(s) (policy: new)", len(newEpisodes))}
}

func hasDownloadableEnclosure(ep backend.FeedEpisode) bool {
	return (ep.Enclosure != nil && ep.Enclosure.URL != "") || ep.EnclosureURL != ""
}

func selectLatestEpisode(sortedCatalog []backend.FeedEpisode, isDownloaded func(ep backend.FeedEpisode) bool, oldest bool) ([]backend.FeedEpisode, []string) {
	if len(sortedCatalog) == 0 {
		return nil, nil
	}
	latestEp := sortedCatalog[len(sortedCatalog)-1]
	if oldest {
		latestEp = sortedCatalog[0]
	}
	if hasDownloadableEnclosure(latestEp) && !isDownloaded(latestEp) {
		return []backend.FeedEpisode{latestEp}, []string{"1 latest episode (policy: latest)"}
	}
	return nil, nil
}

func selectLatestKEpisodes(sortedCatalog []backend.FeedEpisode, isDownloaded func(ep backend.FeedEpisode) bool, k int, oldest bool) ([]backend.FeedEpisode, []string) {
	if len(sortedCatalog) == 0 {
		return nil, nil
	}
	var toDownload []backend.FeedEpisode
	if oldest {
		endIdx := min(len(sortedCatalog), k)
		for _, ep := range sortedCatalog[:endIdx] {
			if hasDownloadableEnclosure(ep) && !isDownloaded(ep) {
				toDownload = append(toDownload, ep)
			}
		}
	} else {
		startIdx := max(0, len(sortedCatalog)-k)
		for _, ep := range sortedCatalog[startIdx:] {
			if hasDownloadableEnclosure(ep) && !isDownloaded(ep) {
				toDownload = append(toDownload, ep)
			}
		}
		for i, j := 0, len(toDownload)-1; i < j; i, j = i+1, j-1 {
			toDownload[i], toDownload[j] = toDownload[j], toDownload[i]
		}
	}
	if len(toDownload) == 0 {
		return nil, nil
	}
	return toDownload, []string{fmt.Sprintf("%d episode(s) (policy: latest_%d)", len(toDownload), k)}
}

func selectAllEpisodes(sortedCatalog []backend.FeedEpisode, isDownloaded func(ep backend.FeedEpisode) bool, oldest bool) ([]backend.FeedEpisode, []string) {
	var undownloaded []backend.FeedEpisode
	for _, ep := range sortedCatalog {
		if hasDownloadableEnclosure(ep) && !isDownloaded(ep) {
			undownloaded = append(undownloaded, ep)
		}
	}
	if len(undownloaded) == 0 {
		return nil, nil
	}
	if !oldest {
		for i, j := 0, len(undownloaded)-1; i < j; i, j = i+1, j-1 {
			undownloaded[i], undownloaded[j] = undownloaded[j], undownloaded[i]
		}
	}
	return undownloaded, []string{fmt.Sprintf("%d episode(s) (policy: all)", len(undownloaded))}
}
