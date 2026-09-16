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
		downloadedIndices = downloadedCatalogIndices(sortedCatalog, isDownloaded)
	}

	// With something already downloaded, "new" means everything published
	// after the newest copy held. With nothing downloaded there is no such
	// mark, so a favourite falls back to everything since it was favourited
	// and anything else takes nothing.
	var candidates []backend.FeedEpisode
	if len(downloadedIndices) > 0 {
		candidates = fetchableAfter(sortedCatalog, maxIndex(downloadedIndices)+1, isDownloaded)
	} else if favoriteSince != nil {
		candidates = fetchableSince(sortedCatalog, favoriteSince.UnixMilli(), isDownloaded)
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	reverseEpisodes(candidates)
	return candidates, []string{fmt.Sprintf("%d new episode(s) (policy: new)", len(candidates))}
}

// downloadedCatalogIndices reports which catalogue positions are held.
func downloadedCatalogIndices(catalog []backend.FeedEpisode, isDownloaded func(backend.FeedEpisode) bool) []int {
	var out []int
	for idx, ep := range catalog {
		if isDownloaded(ep) {
			out = append(out, idx)
		}
	}
	return out
}

func maxIndex(indices []int) int {
	max := -1
	for _, i := range indices {
		if i > max {
			max = i
		}
	}
	return max
}

// fetchableAfter is every downloadable episode from a position onwards that
// is not already held.
func fetchableAfter(catalog []backend.FeedEpisode, from int, isDownloaded func(backend.FeedEpisode) bool) []backend.FeedEpisode {
	if from >= len(catalog) {
		return nil
	}
	var out []backend.FeedEpisode
	for _, ep := range catalog[from:] {
		if hasDownloadableEnclosure(ep) && !isDownloaded(ep) {
			out = append(out, ep)
		}
	}
	return out
}

// fetchableSince is every downloadable episode published at or after a time
// that is not already held.
func fetchableSince(catalog []backend.FeedEpisode, cutoffMS int64, isDownloaded func(backend.FeedEpisode) bool) []backend.FeedEpisode {
	var out []backend.FeedEpisode
	for _, ep := range catalog {
		if GetPubMS(ep) >= cutoffMS && hasDownloadableEnclosure(ep) && !isDownloaded(ep) {
			out = append(out, ep)
		}
	}
	return out
}

// reverseEpisodes puts the newest first, which is the order downloads are
// reported and executed in.
func reverseEpisodes(eps []backend.FeedEpisode) {
	for i, j := 0, len(eps)-1; i < j; i, j = i+1, j-1 {
		eps[i], eps[j] = eps[j], eps[i]
	}
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
