package podcast

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/progress"
)

type DownloadOptions struct {
	Count        int
	Oldest       bool
	DryRun       bool
	NoWait       bool
	Fill         bool
	CountGiven   bool
	CheckNew     bool
	ForceNewOnly bool
	DownloadAll  bool
	Keep         *int

	// Progress receives human-readable progress. A nil Reporter is silent,
	// which is what every non-interactive caller wants.
	Progress progress.Reporter

	// Jobs caps how many feeds are read at once; zero takes the default.
	Jobs int

	// PodcastsDir is the local library root. Without it neither a podcast's own
	// download policy nor the audio already on disk can be found, and a run
	// silently falls back to the default policy.
	PodcastsDir string

	DefaultDownloadPolicy string
	DefaultDownloadK      int
}

func GetPubMS(ep backend.FeedEpisode) int64 {
	if ep.PublishedAt > 0 {
		return ep.PublishedAt
	}
	if ep.PubDate != "" {
		ms, _ := parseFeedDate(ep.PubDate)
		return ms
	}
	return 0
}

func scanPodcastDiskTitles(item backend.Podcast, podcastsDir string) map[string]bool {
	diskTitles := make(map[string]bool)
	podDir := findPodcastDirForItem(item, podcastsDir)
	if podDir == "" {
		return diskTitles
	}
	entries, err := os.ReadDir(podDir)
	if err != nil {
		return diskTitles
	}
	for _, entry := range entries {
		name := strings.ToLower(strings.TrimSpace(entry.Name()))
		if entry.IsDir() {
			diskTitles[name] = true
		} else if strings.HasSuffix(name, ".mp3") {
			diskTitles[name] = true
			stem := strings.TrimSuffix(name, ".mp3")
			diskTitles[stem] = true
			stripped := strings.ToLower(StripEpisodeFilenamePrefix(stem))
			diskTitles[stripped] = true
			diskTitles[strings.ToLower(SanitizeTitle(stripped))] = true
			diskTitles[strings.ReplaceAll(stripped, "_", " ")] = true
		}
	}
	return diskTitles
}

// buildDownloadedChecker reports which feed episodes need no download. What the
// server already holds comes from the catalog index, read once for the whole
// run; queued downloads and audio already on disk are added on top of it.
func buildDownloadedChecker(item backend.Podcast, index *PodcastEpisodeIndex, active []backend.ActiveDownload, podcastsDir string) func(backend.FeedEpisode) bool {
	queuedTitles := make(map[string]bool)
	queuedURLs := make(map[string]bool)
	queuedGUIDs := make(map[string]bool)

	for _, ad := range active {
		for _, t := range []string{ad.EpisodeDisplayTitle, ad.DisplayTitle, ad.Title, ad.Episode.Title} {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" {
				queuedTitles[t] = true
			}
		}
		if ad.URL != "" {
			queuedURLs[ad.URL] = true
		}
		if ad.Episode.EnclosureURL != "" {
			queuedURLs[ad.Episode.EnclosureURL] = true
		}
		if ad.Episode.GUID != "" {
			queuedGUIDs[ad.Episode.GUID] = true
		}
	}

	diskTitles := scanPodcastDiskTitles(item, podcastsDir)

	return func(ep backend.FeedEpisode) bool {
		if index.HasAudio(ep) {
			return true
		}
		encURL := ""
		if ep.Enclosure != nil {
			encURL = ep.Enclosure.URL
		}
		if encURL == "" {
			encURL = ep.EnclosureURL
		}
		guid := ep.GUID
		title := strings.ToLower(strings.TrimSpace(ep.Title))

		if (encURL != "" && queuedURLs[encURL]) ||
			(guid != "" && queuedGUIDs[guid]) ||
			(title != "" && queuedTitles[title]) {
			return true
		}
		if title != "" {
			if diskTitles[title] || diskTitles[strings.ToLower(SanitizeTitle(ep.Title))] {
				return true
			}
			pubMs := GetPubMS(ep)
			var pubTime time.Time
			if pubMs > 0 {
				pubTime = time.UnixMilli(pubMs).UTC()
			}
			fn := strings.ToLower(FormatEpisodeFilename(pubTime, ep.Episode, ep.Title))
			if diskTitles[fn] || diskTitles[strings.TrimSuffix(fn, ".mp3")] {
				return true
			}
		}
		return false
	}
}

func resolveEpisodesToDownload(item backend.Podcast, sortedCatalog []backend.FeedEpisode, downloadedIndices []int, isDownloaded func(backend.FeedEpisode) bool, opts DownloadOptions) ([]backend.FeedEpisode, []string) {
	podDir := findPodcastDirForItem(item, opts.PodcastsDir)
	podCfg := config.DefaultPodcastConfigFrom(config.PolicyDefaults{
		DownloadPolicy: opts.DefaultDownloadPolicy,
		DownloadK:      opts.DefaultDownloadK,
	})
	if podDir != "" {
		podCfg = config.LoadPodcastConfig(podDir, podCfg)
	}

	if opts.DownloadAll {
		eps, reasons := selectEpisodesByDownloadPolicy(sortedCatalog, isDownloaded, config.DownloadPolicyAll, 0, opts.Oldest)
		if opts.CountGiven && opts.Count > 0 && len(eps) > opts.Count {
			eps = eps[:opts.Count]
		}
		return eps, reasons
	}
	if !opts.Fill && !opts.CountGiven {
		if podCfg.Favorite || podCfg.DownloadPolicy == config.DownloadPolicyNew {
			return selectNewEpisodes(sortedCatalog, downloadedIndices, isDownloaded, podCfg.FavoriteSince)
		}
		return selectEpisodesByDownloadPolicy(sortedCatalog, isDownloaded, podCfg.DownloadPolicy, podCfg.DownloadK, opts.Oldest)
	}
	if opts.ForceNewOnly {
		return selectForceNewEpisodes(sortedCatalog, downloadedIndices, isDownloaded, opts.Count, opts.CountGiven, opts.Oldest)
	}
	if opts.Fill {
		eps, reasons := selectFillEpisodes(sortedCatalog, downloadedIndices, isDownloaded, opts.CheckNew, opts.Oldest, progress.Or(opts.Progress), item.Media.Metadata.Title)
		if opts.CountGiven && len(eps) > opts.Count {
			eps = eps[:opts.Count]
		}
		return eps, reasons
	}
	return selectDefaultUndownloadedEpisodes(sortedCatalog, downloadedIndices, isDownloaded, opts.Count, opts.CountGiven, opts.CheckNew, opts.Oldest)
}

func selectForceNewEpisodes(sortedCatalog []backend.FeedEpisode, downloadedIndices []int, isDownloaded func(backend.FeedEpisode) bool, count int, countGiven, oldest bool) ([]backend.FeedEpisode, []string) {
	var episodesToDownload []backend.FeedEpisode
	var reasons []string
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
				hasEnc := (ep.Enclosure != nil && ep.Enclosure.URL != "") || ep.EnclosureURL != ""
				if hasEnc && !isDownloaded(ep) {
					newEpisodes = append(newEpisodes, ep)
				}
			}
		}
		if len(newEpisodes) > 0 {
			reasons = append(reasons, fmt.Sprintf("%d new episode(s)", len(newEpisodes)))
			episodesToDownload = newEpisodes
		}
	} else {
		var undownloaded []backend.FeedEpisode
		for _, ep := range sortedCatalog {
			hasEnc := (ep.Enclosure != nil && ep.Enclosure.URL != "") || ep.EnclosureURL != ""
			if hasEnc && !isDownloaded(ep) {
				undownloaded = append(undownloaded, ep)
			}
		}
		if len(undownloaded) > 0 {
			latestEp := undownloaded[len(undownloaded)-1]
			if oldest {
				latestEp = undownloaded[0]
			}
			reasons = append(reasons, "1 latest episode")
			episodesToDownload = []backend.FeedEpisode{latestEp}
		}
	}
	if countGiven && len(episodesToDownload) > count {
		episodesToDownload = episodesToDownload[:count]
	}
	return episodesToDownload, reasons
}

// searchOrder is the catalogue in the order to walk it: newest first unless
// the caller asked to work up from the oldest.
func searchOrder(catalog []backend.FeedEpisode, oldest bool) []backend.FeedEpisode {
	out := make([]backend.FeedEpisode, len(catalog))
	copy(out, catalog)
	if !oldest {
		reverseEpisodes(out)
	}
	return out
}

// missingAfter is every episode from a position onwards that is not held.
// Unlike fetchableAfter it does not require an enclosure, because the gap
// filler reports what is missing rather than only what it could fetch.
func missingAfter(catalog []backend.FeedEpisode, from int, isDownloaded func(backend.FeedEpisode) bool) []backend.FeedEpisode {
	if from >= len(catalog) {
		return nil
	}
	var out []backend.FeedEpisode
	for _, ep := range catalog[from:] {
		if !isDownloaded(ep) {
			out = append(out, ep)
		}
	}
	return out
}

func selectFillEpisodes(sortedCatalog []backend.FeedEpisode, downloadedIndices []int, isDownloaded func(backend.FeedEpisode) bool, checkNew, oldest bool, rep progress.Reporter, podcastTitle string) ([]backend.FeedEpisode, []string) {
	var episodesToDownload []backend.FeedEpisode
	var reasons []string

	searchCatalog := searchOrder(sortedCatalog, oldest)

	if checkNew && len(downloadedIndices) > 0 {
		newEpisodes := missingAfter(sortedCatalog, maxIndex(downloadedIndices)+1, isDownloaded)
		if len(newEpisodes) > 0 {
			reasons = append(reasons, fmt.Sprintf("%d new episode(s)", len(newEpisodes)))
			episodesToDownload = append(episodesToDownload, newEpisodes...)
		}
	}

	consecutiveMissing := 0
	gapTerminated := false

	inToDownload := func(target backend.FeedEpisode) bool {
		for _, ep := range episodesToDownload {
			if (ep.GUID != "" && target.GUID != "" && ep.GUID == target.GUID) ||
				(ep.Enclosure != nil && target.Enclosure != nil && ep.Enclosure.URL == target.Enclosure.URL) {
				return true
			}
		}
		return false
	}

	for _, ep := range searchCatalog {
		if isDownloaded(ep) {
			consecutiveMissing = 0
		} else {
			consecutiveMissing++
			if consecutiveMissing > 10 {
				gapTerminated = true
				break
			}
			if !inToDownload(ep) {
				episodesToDownload = append(episodesToDownload, ep)
			}
		}
	}

	if len(episodesToDownload) > 0 && len(reasons) == 0 {
		reasons = append(reasons, fmt.Sprintf("%d gap/fill episode(s)", len(episodesToDownload)))
	}
	if gapTerminated {
		rep.Infof("Search for %s terminated: gap larger than 10 undownloaded episodes encountered.", podcastTitle)
	}
	return episodesToDownload, reasons
}

// trimToCount keeps at most count episodes, taking them from the end the
// caller cares about: working up from the oldest keeps the first, otherwise
// the newest are the ones worth having.
func trimToCount(eps []backend.FeedEpisode, count int, oldest bool) []backend.FeedEpisode {
	if len(eps) <= count {
		return eps
	}
	if oldest {
		return eps[:count]
	}
	return eps[len(eps)-count:]
}

func selectDefaultUndownloadedEpisodes(sortedCatalog []backend.FeedEpisode, downloadedIndices []int, isDownloaded func(backend.FeedEpisode) bool, count int, countGiven, checkNew, oldest bool) ([]backend.FeedEpisode, []string) {
	var episodesToDownload []backend.FeedEpisode
	var reasons []string

	if checkNew && len(downloadedIndices) > 0 {
		newEpisodes := missingAfter(sortedCatalog, maxIndex(downloadedIndices)+1, isDownloaded)
		if len(newEpisodes) > 0 {
			reasons = append(reasons, fmt.Sprintf("%d new episode(s)", len(newEpisodes)))
			episodesToDownload = append(episodesToDownload, newEpisodes...)
		}
	}

	if len(episodesToDownload) > 0 {
		if countGiven && len(episodesToDownload) > count {
			episodesToDownload = trimToCount(episodesToDownload, count, oldest)
		}
		return episodesToDownload, reasons
	}

	undownloaded := missingAfter(sortedCatalog, 0, isDownloaded)
	if len(undownloaded) == 0 {
		return nil, reasons
	}
	if !oldest {
		reverseEpisodes(undownloaded)
	}
	if len(undownloaded) > count {
		undownloaded = undownloaded[:count]
	}
	return undownloaded, append(reasons, fmt.Sprintf("%d undownloaded episode(s)", len(undownloaded)))
}

func ExecuteEpisodeDownloads(client backend.Backend, item backend.Podcast, episodesToDownload []backend.FeedEpisode, reasons []string, opts DownloadOptions) (int, error) {
	podcastTitle := item.Media.Metadata.Title
	if podcastTitle == "" {
		podcastTitle = "Untitled Podcast"
	}

	if len(episodesToDownload) == 0 {
		return 0, nil
	}
	rep := progress.Or(opts.Progress)
	sortAndReportSelectedEpisodes(episodesToDownload, podcastTitle, reasons, opts.Oldest, rep)
	if err := queueAndTrackDownloads(client, item, episodesToDownload, opts.NoWait, opts.DryRun, rep); err != nil {
		return 0, err
	}

	return len(episodesToDownload), nil
}

func sortAndReportSelectedEpisodes(episodesToDownload []backend.FeedEpisode, podcastTitle string, reasons []string, oldest bool, rep progress.Reporter) {
	rep.Infof("\n=== Podcast: %s ===", podcastTitle)
	sort.Slice(episodesToDownload, func(i, j int) bool {
		return GetPubMS(episodesToDownload[i]) < GetPubMS(episodesToDownload[j])
	})
	if !oldest {
		for i, j := 0, len(episodesToDownload)-1; i < j; i, j = i+1, j-1 {
			episodesToDownload[i], episodesToDownload[j] = episodesToDownload[j], episodesToDownload[i]
		}
	}

	directionStr := "latest -> oldest"
	if oldest {
		directionStr = "oldest -> newest"
	}
	rep.Infof("Found %s (%s).", strings.Join(reasons, " and "), directionStr)
	rep.Infof("\n=== Selected %d Episode(s) for Download ===", len(episodesToDownload))
	for idx, ep := range episodesToDownload {
		pub := ep.PubDate
		if pub == "" {
			pub = fmt.Sprintf("%d", ep.PublishedAt)
		}
		rep.Infof("  %d. %s", idx+1, ep.Title)
		rep.Infof("     Published: %s", pub)
		if ep.Enclosure != nil {
			rep.Detailf("     URL: %s", ep.Enclosure.URL)
		} else {
			rep.Detailf("     URL: ")
		}
	}
}

func queueAndTrackDownloads(client backend.Backend, item backend.Podcast, episodesToDownload []backend.FeedEpisode, noWait, dryRun bool, rep progress.Reporter) error {
	if dryRun {
		rep.Infof("Dry run mode enabled. Skipping actual download request.")
		return nil
	}

	rep.Infof("Queueing download request for %d episode(s)...", len(episodesToDownload))
	if err := client.DownloadEpisodes(item.ID, episodesToDownload); err != nil {
		return fmt.Errorf("queue episode download: %w", err)
	}

	rep.Infof("Download request successfully sent!")
	if !noWait {
		return client.WaitForActiveDownloads([]backend.Podcast{item}, 5*time.Minute)
	}
	return nil
}
