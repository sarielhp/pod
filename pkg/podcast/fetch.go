package podcast

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/util"
)

// FetchOptions controls which episodes pod fetch targets.
type FetchOptions struct {
	Target        string
	Count         int
	IncludeHourly bool
	Jobs          int
}

// PlanFetch reads feeds of active subscriptions and selects undownloaded latest episodes.
func (l *Library) PlanFetch(subs []Subscription, opts FetchOptions, onProgress func(done, total int)) []SubscriptionPlan {
	targets := FetchTargets(subs, opts, l)
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
				plans[i] = l.planOneFetch(targets[i], opts)
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

// FetchTargets filters subscriptions for fetch: enabled only, matching target if specified,
// and non-hourly unless IncludeHourly is true.
func FetchTargets(subs []Subscription, opts FetchOptions, l *Library) []Subscription {
	var targets []Subscription
	for _, sub := range subs {
		if sub.Disabled {
			continue
		}
		if opts.Target != "" && !SubscriptionMatches(sub, opts.Target) {
			continue
		}
		podDir := filepath.Join(l.cfg.PodcastsDir, sub.Folder)
		if !opts.IncludeHourly && isHourlyCadence(podDir) {
			continue
		}
		targets = append(targets, sub)
	}
	return targets
}

func (l *Library) planOneFetch(sub Subscription, opts FetchOptions) SubscriptionPlan {
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
	if len(feedEps) == 0 {
		return plan
	}

	sortedCatalog := make([]backend.FeedEpisode, len(feedEps))
	copy(sortedCatalog, feedEps)
	sort.Slice(sortedCatalog, func(i, j int) bool {
		return GetPubMS(sortedCatalog[i]) < GetPubMS(sortedCatalog[j])
	})

	isDownloaded := downloadedEpisodeChecker(podDir)
	count := opts.Count
	if count <= 0 {
		count = 1
	}
	if count == 1 {
		plan.ToDownload, _ = selectLatestEpisode(sortedCatalog, isDownloaded, false)
	} else {
		plan.ToDownload, _ = selectLatestKEpisodes(sortedCatalog, isDownloaded, count, false)
	}
	return plan
}
