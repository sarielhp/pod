package adremoval

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/types"
	"pod/pkg/util"
)

// ProcessPodcast removes ads from one podcast the caller has already resolved,
// queueing or processing episodes according to that podcast's own policy.
func ProcessPodcast(pod *podcast.ResolvedPodcast, opts types.ProcOptions, cfg types.Config, action string) error {
	opts.Normalize()

	targetAudioPath, err := resolveTargetEpisodeForRmAds(pod, opts, cfg)
	if err != nil {
		return err
	}
	if targetAudioPath == "" {
		return nil
	}

	epFilename := queueItemFilename(pod.Dir, targetAudioPath)
	displayEp := filepath.Base(targetAudioPath)
	if strings.EqualFold(util.StripExt(displayEp), "podcast") {
		if t := podcast.EpisodeTitleFromPath(targetAudioPath); t != "" {
			displayEp = t
		}
	}
	if opts.DryRun {
		if !opts.Quiet {
			fmt.Printf("[Dry run] Would queue and process %s for ad removal.\n", util.DisplayName(displayEp))
		}
		return nil
	}

	added := pipeline.AddToQueue(pod.Dir, epFilename)
	epID := podcast.GetOrSetEpisodeShortID(pod.Dir, pod.ShortID, targetAudioPath)
	title := podcast.EpisodeTitleFromPath(targetAudioPath)

	if !opts.Quiet {
		if added {
			fmt.Printf("Added to AdR queue: [%s] %s\n", util.BoldCyan(epID), util.DisplayName(title))
		} else {
			fmt.Printf("Already in AdR queue: [%s] %s\n", util.BoldCyan(epID), util.DisplayName(title))
		}
	}

	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "."
	}
	totalQueued := countAllQueuedEpisodes(podcastsDir)
	if totalQueued > 1 {
		if !opts.Quiet {
			fmt.Printf("Queued for ad removal (%d items in queue).\n", totalQueued)
		}
		return nil
	}

	return ProcessQueuedTarget(pod.Dir, targetAudioPath, action, opts, cfg)
}

func countAllQueuedEpisodes(podcastsDir string) int {
	if podcastsDir == "" {
		podcastsDir = "."
	}
	total := 0
	for _, p := range podcast.ScanPodcastDirs(podcastsDir) {
		total += len(pipeline.QueuedEpisodes(p.Dir))
	}
	return total
}

func resolveTargetEpisodeForRmAds(pod *podcast.ResolvedPodcast, opts types.ProcOptions, cfg types.Config) (string, error) {
	b := getActiveBackendForPodcast(cfg, opts.Quiet)
	if b != nil {
		if targetPath, handled := findTargetEpisodeFromBackend(b, pod, cfg, opts.Quiet); handled {
			return targetPath, nil
		}
	}

	targetPath, ok := findLatestUncleanedLocalEpisode(pod.Dir, pod.Title, opts.Quiet)
	if !ok {
		return "", nil
	}
	return targetPath, nil
}

func getActiveBackendForPodcast(cfg types.Config, quiet bool) backend.Backend {
	b, err := backend.FromAppConfig(&cfg, stdoutReporter(quiet))
	if err == nil {
		return b
	}
	return nil
}

func findTargetEpisodeFromBackend(b backend.Backend, pod *podcast.ResolvedPodcast, cfg types.Config, quiet bool) (string, bool) {
	feedURL, targetItem := resolveBackendPodcastAndFeed(b, pod, cfg)
	if feedURL == "" {
		return "", false
	}

	feedEpisodes, err := b.PodcastFeedEpisodes(feedURL)
	if err != nil || len(feedEpisodes) == 0 {
		feedEpisodes, _, _, _, _ = podcast.FetchFeedDirect(feedURL, "", "")
	}
	if len(feedEpisodes) == 0 {
		return "", false
	}

	sort.Slice(feedEpisodes, func(i, j int) bool {
		return podcast.GetPubMS(feedEpisodes[i]) > podcast.GetPubMS(feedEpisodes[j])
	})

	for i, fe := range feedEpisodes {
		localPath, isDownloaded := findLocalPathForFeedEpisode(pod.Dir, fe, targetItem)
		if isDownloaded {
			if pipeline.IsEpisodeClean(localPath) {
				continue
			}
			return localPath, true
		}

		if i == 0 {
			dlPath, dlErr := downloadSingleFeedEpisode(b, targetItem, pod.Dir, fe, quiet)
			if dlErr == nil && dlPath != "" {
				return dlPath, true
			}
		}
	}

	if uncleanedPath, ok := findLatestUncleanedLocalEpisode(pod.Dir, pod.Title, true); ok && uncleanedPath != "" {
		return uncleanedPath, true
	}

	if !quiet {
		fmt.Printf("All episodes for podcast %s already have ads removed.\n", util.DisplayName(pod.Title))
	}
	return "", true
}

func resolveBackendPodcastAndFeed(b backend.Backend, pod *podcast.ResolvedPodcast, cfg types.Config) (string, *backend.Podcast) {
	feedURL := ""
	itemID := pod.UUID
	if cached, _ := podcast.LoadPodcastCache(pod.Dir); cached != nil {
		if cached.FeedURL != "" {
			feedURL = cached.FeedURL
		}
		if cached.ABSItemID != "" {
			itemID = cached.ABSItemID
		}
	}

	if itemID != "" {
		if targetItem, err := b.GetPodcast(itemID); err == nil && targetItem != nil {
			if targetItem.Media.Metadata.FeedURL != "" {
				feedURL = targetItem.Media.Metadata.FeedURL
			}
			if pod.Title == "" && targetItem.Media.Metadata.Title != "" {
				pod.Title = targetItem.Media.Metadata.Title
			}
			return feedURL, targetItem
		}
	}

	podcasts, err := b.Podcasts()
	if err != nil {
		return feedURL, nil
	}

	var targetItem *backend.Podcast
	for i := range podcasts {
		p := &podcasts[i]
		if isMatchingBackendPodcast(p, itemID, pod, cfg.PodcastsDir) {
			targetItem = p
			break
		}
	}

	if targetItem != nil {
		if targetItem.Media.Metadata.FeedURL != "" {
			feedURL = targetItem.Media.Metadata.FeedURL
		}
		if pod.Title == "" && targetItem.Media.Metadata.Title != "" {
			pod.Title = targetItem.Media.Metadata.Title
		}
	}

	return feedURL, targetItem
}

func isMatchingBackendPodcast(p *backend.Podcast, itemID string, pod *podcast.ResolvedPodcast, podcastsDir string) bool {
	if pod.Dir != "" && p.Path != "" && filepath.Clean(p.Path) == filepath.Clean(pod.Dir) {
		return true
	}
	if podcastsDir != "" && p.Path != "" {
		if !strings.HasPrefix(filepath.Clean(p.Path), filepath.Clean(podcastsDir)) {
			return false
		}
	}
	if strings.EqualFold(p.Media.Metadata.Title, pod.Title) {
		return true
	}
	if itemID != "" && p.ID == itemID && strings.EqualFold(filepath.Base(p.Path), filepath.Base(pod.Dir)) {
		return true
	}
	if pod.UUID != "" && p.ID == pod.UUID && strings.EqualFold(filepath.Base(p.Path), filepath.Base(pod.Dir)) {
		return true
	}
	return false
}

func normalizeForFuzzyMatch(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

func stripShowPrefix(title, showName string) string {
	if showName == "" || title == "" {
		return title
	}
	cleanShow := strings.ToLower(strings.TrimSpace(showName))
	cleanTitle := strings.TrimSpace(title)
	if strings.HasPrefix(strings.ToLower(cleanTitle), cleanShow) {
		trimmed := strings.TrimSpace(cleanTitle[len(cleanShow):])
		trimmed = strings.TrimLeft(trimmed, "-:–—_ ")
		if trimmed != "" {
			return trimmed
		}
	}
	return cleanTitle
}

func isFuzzyEpisodeMatch(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	na := normalizeForFuzzyMatch(a)
	nb := normalizeForFuzzyMatch(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	shorter, longer := na, nb
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len([]rune(shorter)) >= 6 && strings.Contains(longer, shorter) {
		return true
	}
	return false
}

func resolveMatchingEpisodeAudioFile(podDir string, ep backend.Episode) (string, bool) {
	if ep.AudioFile == nil || ep.AudioFile.Metadata == nil {
		return "", false
	}
	meta := ep.AudioFile.Metadata
	for _, raw := range []string{meta.Path, meta.RelPath} {
		if p := locateEpisodeAudio(podDir, raw); p != "" {
			return p, true
		}
	}
	if meta.Filename != "" {
		if p := filepath.Join(podDir, meta.Filename); util.FileExists(p) {
			return p, true
		}
	}
	return "", false
}

// locateEpisodeAudio finds the local file a backend path refers to.
//
// The path may be absolute, relative to the podcast, relative to the library,
// or carry a "/podcasts/" prefix from the server that recorded it, so each
// interpretation is tried in turn and the first that exists wins.
func locateEpisodeAudio(podDir, raw string) string {
	if raw == "" {
		return ""
	}
	for _, candidate := range episodeAudioCandidates(podDir, raw) {
		if util.FileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// episodeAudioCandidates lists every place a recorded path might mean, in
// order of preference.
func episodeAudioCandidates(podDir, raw string) []string {
	podBase := filepath.Base(podDir)
	podRoot := filepath.Dir(podDir)

	candidates := []string{
		raw,
		filepath.Join(podDir, raw),
		filepath.Join(podDir, filepath.Base(raw)),
	}

	// A path recorded as "<episode dir>/<file>" keeps its own directory, but
	// only when that directory is not the podcast itself.
	if epDir := filepath.Base(filepath.Dir(raw)); epDir != "." && epDir != "/" && epDir != "" && epDir != podBase {
		candidates = append(candidates, filepath.Join(podDir, epDir, filepath.Base(raw)))
	}

	candidates = append(candidates, filepath.Join(podRoot, raw))

	trimmed := strings.TrimPrefix(raw, "/podcasts/")
	trimmed = strings.TrimPrefix(trimmed, "podcasts/")
	trimmed = strings.TrimPrefix(trimmed, "/")
	candidates = append(candidates,
		filepath.Join(podRoot, trimmed),
		filepath.Join(podDir, trimmed))

	if strings.HasPrefix(trimmed, podBase+"/") {
		candidates = append(candidates, filepath.Join(podDir, strings.TrimPrefix(trimmed, podBase+"/")))
	}
	return candidates
}

func findLocalPathForFeedEpisode(podDir string, fe backend.FeedEpisode, item *backend.Podcast) (string, bool) {
	if path, ok := findMatchingEpisodeInItem(podDir, fe, item); ok {
		return path, true
	}

	podBase := filepath.Base(podDir)
	safeTitle := podcast.SanitizeTitle(fe.Title)
	feStripped := stripShowPrefix(fe.Title, podBase)
	pubMs := podcast.GetPubMS(fe)
	var pubTime time.Time
	if pubMs > 0 {
		pubTime = time.UnixMilli(pubMs).UTC()
	}
	targetStem := strings.ToLower(util.StripExt(podcast.FormatEpisodeFilename(pubTime, fe.Episode, fe.Title)))
	for _, mp3 := range util.FindMP3Files(podDir) {
		base := util.StripExt(filepath.Base(mp3))
		strippedBase := podcast.StripEpisodeFilenamePrefix(base)
		title := podcast.EpisodeTitleFromPath(mp3)
		baseStripped := stripShowPrefix(base, podBase)
		titleStripped := stripShowPrefix(title, podBase)
		if strings.EqualFold(base, targetStem) ||
			strings.EqualFold(base, safeTitle) || strings.EqualFold(base, fe.Title) ||
			strings.EqualFold(strippedBase, safeTitle) ||
			strings.EqualFold(title, safeTitle) || strings.EqualFold(title, fe.Title) ||
			strings.EqualFold(podcast.SanitizeTitle(title), safeTitle) ||
			strings.EqualFold(baseStripped, feStripped) ||
			strings.EqualFold(titleStripped, feStripped) ||
			isFuzzyEpisodeMatch(title, fe.Title) ||
			isFuzzyEpisodeMatch(strippedBase, fe.Title) ||
			isFuzzyEpisodeMatch(titleStripped, feStripped) {
			return mp3, true
		}
		detailKey := filepath.Base(mp3)
		if strings.EqualFold(base, "podcast") && title != "" {
			detailKey = title + ".mp3"
		}
		if dt, _ := podcast.LoadEpisodeDetails(podDir, detailKey); dt != nil {
			if (fe.GUID != "" && dt.Subtitle == fe.GUID) ||
				(fe.Title != "" && strings.EqualFold(dt.Title, fe.Title)) {
				return mp3, true
			}
		}
	}

	return "", false
}

func findMatchingEpisodeInItem(podDir string, fe backend.FeedEpisode, item *backend.Podcast) (string, bool) {
	if item == nil {
		return "", false
	}
	feURL := fe.EnclosureURL
	if feURL == "" && fe.Enclosure != nil {
		feURL = fe.Enclosure.URL
	}
	for _, ep := range item.Media.Episodes {
		matched := (feURL != "" && ep.EnclosureURL != "" && feURL == ep.EnclosureURL) ||
			(fe.GUID != "" && ep.GUID != "" && fe.GUID == ep.GUID) ||
			(fe.Title != "" && ep.Title != "" && strings.EqualFold(strings.TrimSpace(fe.Title), strings.TrimSpace(ep.Title))) ||
			(fe.Title != "" && ep.Title != "" && isFuzzyEpisodeMatch(ep.Title, fe.Title))
		if matched && ep.AudioFile != nil && ep.AudioFile.Metadata != nil {
			if p, ok := resolveMatchingEpisodeAudioFile(podDir, ep); ok {
				return p, true
			}
		}
	}
	return "", false
}

func tryDirectDownloadEpisode(podDir string, fe backend.FeedEpisode, quiet bool) (string, bool) {
	encURL := fe.EnclosureURL
	if fe.Enclosure != nil && fe.Enclosure.URL != "" {
		encURL = fe.Enclosure.URL
	}
	if encURL == "" {
		return "", false
	}
	safeTitle := podcast.SanitizeTitle(fe.Title)
	destPath := filepath.Join(podDir, safeTitle+".mp3")
	d := podcast.NewDownloader()
	if err := d.DownloadEpisode(context.Background(), encURL, destPath, stdoutReporter(quiet)); err == nil {
		return destPath, true
	}
	return "", false
}

func downloadSingleFeedEpisode(b backend.Backend, item *backend.Podcast, podDir string, fe backend.FeedEpisode, quiet bool) (string, error) {
	if !quiet {
		fmt.Printf("Downloading latest episode: %s\n", fe.Title)
	}

	if destPath, ok := tryDirectDownloadEpisode(podDir, fe, quiet); ok {
		return destPath, nil
	}

	if b == nil {
		return "", fmt.Errorf("direct download failed and no backend available")
	}

	itemID := resolveDownloadPodcastID(item, podDir)
	if itemID == "" {
		return "", fmt.Errorf("could not determine podcast ID for download")
	}

	existingFiles := make(map[string]bool)
	for _, f := range util.FindMP3Files(podDir) {
		existingFiles[f] = true
	}

	if err := b.DownloadEpisodes(itemID, []backend.FeedEpisode{fe}); err != nil {
		return "", fmt.Errorf("failed to trigger episode download: %w", err)
	}

	waitForBackendDownloads(b, itemID)

	for _, f := range util.FindMP3Files(podDir) {
		if !existingFiles[f] {
			return f, nil
		}
	}

	safeTitle := podcast.SanitizeTitle(fe.Title)
	for _, f := range util.FindMP3Files(podDir) {
		base := util.StripExt(filepath.Base(f))
		title := podcast.EpisodeTitleFromPath(f)
		if strings.EqualFold(base, safeTitle) || strings.EqualFold(base, fe.Title) ||
			strings.EqualFold(title, safeTitle) || strings.EqualFold(title, fe.Title) ||
			strings.EqualFold(podcast.SanitizeTitle(title), safeTitle) ||
			isFuzzyEpisodeMatch(title, fe.Title) {
			return f, nil
		}
	}

	return "", fmt.Errorf("downloaded file not found in %s", podDir)
}

func resolveDownloadPodcastID(item *backend.Podcast, podDir string) string {
	if item != nil && item.ID != "" {
		return item.ID
	}
	if cached, _ := podcast.LoadPodcastCache(podDir); cached != nil {
		return cached.ABSItemID
	}
	return ""
}

func waitForBackendDownloads(b backend.Backend, itemID string) {
	startTime := time.Now()
	for {
		activeDls, err := b.ActiveDownloads(itemID)
		if err == nil && len(activeDls) == 0 {
			break
		}
		time.Sleep(2 * time.Second)
		if time.Since(startTime) > 300*time.Second {
			break
		}
	}
}

func findLatestUncleanedLocalEpisode(podDir, podTitle string, quiet bool) (string, bool) {
	mp3s := util.FindMP3Files(podDir)
	if len(mp3s) == 0 {
		if !quiet {
			fmt.Printf("All episodes for podcast %s already have ads removed.\n", util.DisplayName(podTitle))
		}
		return "", false
	}

	sort.Slice(mp3s, func(i, j int) bool {
		ti := podcast.GetEpisodePublicationTime(mp3s[i])
		tj := podcast.GetEpisodePublicationTime(mp3s[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return mp3s[i] > mp3s[j]
	})

	for _, mp3 := range mp3s {
		if !pipeline.IsEpisodeClean(mp3) {
			return mp3, true
		}
	}

	if !quiet {
		fmt.Printf("All episodes for podcast %s already have ads removed.\n", util.DisplayName(podTitle))
	}
	return "", false
}

func ProcessQueuedTarget(podDir, targetAudioPath, action string, opts types.ProcOptions, cfg types.Config) error {
	opts.Normalize()

	if err := executeLocalBatchProcessing([]string{targetAudioPath}, opts, cfg, action); err != nil {
		return err
	}

	if !pipeline.IsEpisodeClean(targetAudioPath) {
		return fmt.Errorf("episode did not complete ad removal; retained in queue: %s", targetAudioPath)
	}
	_, err := pipeline.RemoveQueuedAudio(podDir, targetAudioPath)
	refreshPodcastFeedXML(podDir, cfg)
	return err
}

func refreshPodcastFeedXML(podDir string, cfg types.Config) {
	store, _ := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	var sub podcast.Subscription
	if store != nil {
		if s := store.Get(filepath.Base(podDir)); s != nil {
			sub = *s
		}
	}
	if sub.Title == "" {
		sub.Title = filepath.Base(podDir)
		sub.Folder = filepath.Base(podDir)
	}
	_ = podcast.PublishPodcast(podDir, sub, cfg.ServerBaseURL, nil)
}

func queueItemFilename(podDir, audioPath string) string {
	if rel, err := filepath.Rel(podDir, audioPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return filepath.Base(audioPath)
}
