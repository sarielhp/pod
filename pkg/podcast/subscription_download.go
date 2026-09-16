package podcast

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/pipeline"
	"pod/pkg/util"
)

// SubscriptionDownloadOptions selects which subscriptions a run covers and how
// many episodes it takes from each.
type SubscriptionDownloadOptions struct {
	// Target selects a subscription by ID or title substring. Empty covers
	// every enabled subscription.
	Target string

	// Jobs caps how many feeds are fetched at once; zero takes the default.
	Jobs int

	// Count, when CountGiven, caps episodes taken per podcast and overrides
	// the podcast's own download policy.
	Count      int
	CountGiven bool

	// DownloadAll ignores the podcast's download policy and takes everything
	// not already on disk.
	DownloadAll bool

	// Defaults supply the download and ad-removal policy for podcasts that
	// carry none of their own.
	Defaults config.PolicyDefaults
}

// SubscriptionPlan is what a run intends to do for one subscription. Err
// records a feed that could not be read; the plan is still returned, so a
// caller can report the failure alongside the podcasts that did work.
type SubscriptionPlan struct {
	Sub          Subscription
	PodDir       string
	FeedEpisodes []backend.FeedEpisode
	ToDownload   []backend.FeedEpisode
	Err          error
}

// SubscriptionDownloadResult summarises a completed run.
type SubscriptionDownloadResult struct {
	Downloaded int
	Podcasts   int
	Failures   []error
}

// SubscriptionTargets returns the enabled subscriptions a query selects.
func SubscriptionTargets(subs []Subscription, target string) []Subscription {
	var targets []Subscription
	for _, sub := range subs {
		if sub.Disabled || !SubscriptionMatches(sub, target) {
			continue
		}
		targets = append(targets, sub)
	}
	return targets
}

// PlanSubscriptionDownloads reads every target's feed and works out which
// episodes are missing, fetching feeds concurrently. onProgress, if given, is
// called as each feed completes so a caller can show progress; it runs under a
// lock, so it must not block.
func (l *Library) PlanSubscriptionDownloads(targets []Subscription, opts SubscriptionDownloadOptions, onProgress func(done, total int)) []SubscriptionPlan {
	plans := make([]SubscriptionPlan, len(targets))
	workers := FeedCheckWorkers(opts.Jobs, len(targets))

	jobs := make(chan int)
	var wg util.WaitGroup
	var mu util.Mutex
	done := 0

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				plans[i] = l.planOneSubscription(targets[i], opts)
				mu.Lock()
				done++
				if onProgress != nil {
					onProgress(done, len(targets))
				}
				mu.Unlock()
			}
		}()
	}
	for i := range targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return plans
}

func (l *Library) planOneSubscription(sub Subscription, opts SubscriptionDownloadOptions) SubscriptionPlan {
	podDir := filepath.Join(l.cfg.PodcastsDir, sub.Folder)
	plan := SubscriptionPlan{Sub: sub, PodDir: podDir}

	if strings.TrimSpace(sub.FeedURL) == "" {
		plan.Err = fmt.Errorf("no RSS feed URL configured")
		return plan
	}
	feedEps, _, _, _, err := FetchFeedDirect(sub.FeedURL, "", "")
	if err != nil {
		plan.Err = fmt.Errorf("fetch feed %s: %w", sub.FeedURL, err)
		return plan
	}
	plan.FeedEpisodes = feedEps
	if len(feedEps) > 0 {
		plan.ToDownload = selectSubscriptionEpisodes(podDir, feedEps, sub, opts)
	}
	return plan
}

// selectSubscriptionEpisodes picks the episodes of a feed that are not already
// on disk, honouring the explicit count, then the subscription's own policy,
// then the podcast's, then the supplied defaults.
func selectSubscriptionEpisodes(podDir string, feedEps []backend.FeedEpisode, sub Subscription, opts SubscriptionDownloadOptions) []backend.FeedEpisode {
	isDownloaded := downloadedEpisodeChecker(podDir)
	podCfg := subscriptionPodcastConfig(podDir, sub, opts.Defaults)

	sortedCatalog := make([]backend.FeedEpisode, len(feedEps))
	copy(sortedCatalog, feedEps)
	sort.Slice(sortedCatalog, func(i, j int) bool {
		return GetPubMS(sortedCatalog[i]) < GetPubMS(sortedCatalog[j])
	})

	if opts.DownloadAll || (opts.CountGiven && opts.Count > 0) {
		eps, _ := selectEpisodesByDownloadPolicy(sortedCatalog, isDownloaded, config.DownloadPolicyAll, 0, false)
		if opts.CountGiven && opts.Count > 0 && len(eps) > opts.Count {
			eps = eps[:opts.Count]
		}
		return eps
	}

	if podCfg.Favorite || config.NormalizeDownloadPolicy(podCfg.DownloadPolicy) == config.DownloadPolicyNew {
		eps, _ := selectNewEpisodes(sortedCatalog, nil, isDownloaded, podCfg.FavoriteSince)
		return eps
	}
	if !podCfg.IsAutoDownloadEnabled() {
		return nil
	}

	k := podCfg.DownloadK
	if k <= 0 {
		k = opts.Defaults.DownloadK
	}
	eps, _ := selectEpisodesByDownloadPolicy(sortedCatalog, isDownloaded,
		config.NormalizeDownloadPolicy(podCfg.DownloadPolicy), k, false)
	return eps
}

func subscriptionPodcastConfig(podDir string, sub Subscription, defaults config.PolicyDefaults) config.PodcastConfig {
	podCfg := config.LoadPodcastConfig(podDir, config.DefaultPodcastConfigFrom(defaults))
	if sub.DownloadPolicy != "" {
		podCfg.DownloadPolicy = sub.DownloadPolicy
		autoDl := config.NormalizeDownloadPolicy(sub.DownloadPolicy) != config.DownloadPolicyNone
		podCfg.AutoDownload = &autoDl
	}
	if sub.DownloadK > 0 {
		podCfg.DownloadK = sub.DownloadK
	}
	return podCfg
}

// downloadedEpisodeChecker indexes the audio already in podDir, matching a feed
// entry by its formatted filename and by its title both sanitised and raw.
func downloadedEpisodeChecker(podDir string) func(backend.FeedEpisode) bool {
	existing := make(map[string]bool)
	for _, f := range util.FindMP3Files(podDir) {
		name := strings.ToLower(strings.TrimSuffix(filepath.Base(f), ".mp3"))
		existing[name] = true
		existing[strings.ToLower(StripEpisodeFilenamePrefix(name))] = true
	}
	return func(ep backend.FeedEpisode) bool {
		pubMs := GetPubMS(ep)
		var pubTime time.Time
		if pubMs > 0 {
			pubTime = time.UnixMilli(pubMs).UTC()
		}
		formatted := strings.ToLower(strings.TrimSuffix(FormatEpisodeFilename(pubTime, ep.Episode, ep.Title), ".mp3"))
		return existing[formatted] ||
			existing[strings.ToLower(SanitizeTitle(ep.Title))] ||
			existing[strings.ToLower(strings.TrimSpace(ep.Title))]
	}
}

// shouldQueueForAdRemoval reports whether episodes downloaded for this
// subscription should be queued for ad removal.
func shouldQueueForAdRemoval(podDir string, sub Subscription, defaults config.PolicyDefaults) bool {
	podCfg := config.LoadPodcastConfig(podDir, config.DefaultPodcastConfigFrom(defaults))
	if podCfg.Favorite {
		return true
	}
	adPolicy := sub.AdRemoval
	if adPolicy == "" {
		adPolicy = podCfg.AdRemoval
	}
	if adPolicy == "" {
		adPolicy = defaults.AdRemoval
	}
	return config.NormalizeAdRemovalMode(adPolicy) != config.AdRemovalNone
}

// ExecuteSubscriptionDownloads carries out a set of plans: downloads the
// selected episodes, republishes each podcast's feed and page, refreshes any
// missing cover from the feed cache, and republishes the catalog.
func (l *Library) ExecuteSubscriptionDownloads(plans []SubscriptionPlan, store *SubscriptionStore, opts SubscriptionDownloadOptions) SubscriptionDownloadResult {
	downloader := NewDownloader()
	var res SubscriptionDownloadResult

	for _, plan := range plans {
		if plan.Err != nil {
			continue
		}
		if len(plan.ToDownload) > 0 {
			n, err := l.downloadPlan(downloader, plan, opts)
			if err != nil {
				res.Failures = append(res.Failures, fmt.Errorf("%s: %w", plan.Sub.Title, err))
			} else {
				res.Downloaded += n
				res.Podcasts++
			}
		} else {
			_ = PublishPodcast(plan.PodDir, plan.Sub, l.cfg.ServerBaseURL, plan.FeedEpisodes)
		}
		l.refreshSubscriptionCover(&plan, store)
	}

	if l.cfg.PodcastsDir != "" && store != nil {
		_ = PublishCatalog(l.cfg.PodcastsDir, store.List())
	}
	return res
}

func (l *Library) downloadPlan(d *Downloader, plan SubscriptionPlan, opts SubscriptionDownloadOptions) (int, error) {
	if err := os.MkdirAll(plan.PodDir, 0755); err != nil {
		return 0, err
	}
	l.progress.Infof("\n=== Podcast: %s ===", util.BoldCyan(plan.Sub.Title))
	l.progress.Infof("Found %d episode(s) to download:", len(plan.ToDownload))
	for idx, ep := range plan.ToDownload {
		l.progress.Infof("  %d. %s", idx+1, ep.Title)
	}

	shouldQueue := shouldQueueForAdRemoval(plan.PodDir, plan.Sub, opts.Defaults)
	downloaded := 0
	for _, ep := range plan.ToDownload {
		if err := l.downloadEpisode(d, plan.PodDir, ep, shouldQueue); err != nil {
			l.progress.Warnf("    Download error for %q: %v", ep.Title, err)
			continue
		}
		downloaded++
	}
	_ = PublishPodcast(plan.PodDir, plan.Sub, l.cfg.ServerBaseURL, plan.FeedEpisodes)
	return downloaded, nil
}

func (l *Library) downloadEpisode(d *Downloader, podDir string, ep backend.FeedEpisode, shouldQueue bool) error {
	encURL := ep.EnclosureURL
	if ep.Enclosure != nil && ep.Enclosure.URL != "" {
		encURL = ep.Enclosure.URL
	}
	if encURL == "" {
		return fmt.Errorf("no enclosure URL found")
	}

	pubMs := GetPubMS(ep)
	var pubTime time.Time
	if pubMs > 0 {
		pubTime = time.UnixMilli(pubMs).UTC()
	}
	fn := FormatEpisodeFilename(pubTime, ep.Episode, ep.Title)
	destPath := filepath.Join(podDir, fn)

	l.progress.Infof("  Downloading: %s", ep.Title)
	if err := d.DownloadEpisode(context.Background(), encURL, destPath, l.progress); err != nil {
		return err
	}

	if pubMs > 0 {
		st := pipeline.GetOrCreateEpisodeStatus(destPath)
		st.PublishedAt = time.UnixMilli(pubMs).UTC().Format(time.RFC3339)
		st.PublicationSource = "feed"
		_ = pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(destPath), st)
	}
	if shouldQueue {
		pipeline.AddToQueue(podDir, fn)
	}
	return nil
}

func (l *Library) refreshSubscriptionCover(plan *SubscriptionPlan, store *SubscriptionStore) {
	if plan.Sub.ImageURL != "" || store == nil {
		return
	}
	entry := l.feedCache.Get(plan.Sub.FeedURL)
	if entry == nil || entry.ImageURL == "" {
		return
	}
	plan.Sub.ImageURL = entry.ImageURL
	_ = store.Add(plan.Sub)
	_ = store.Save()
}
