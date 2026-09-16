package podcast

import (
	"path/filepath"
	"sort"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
)

// LatestEpisodePlanOptions shapes the selection of recently published
// episodes.
type LatestEpisodePlanOptions struct {
	// Count is how many episodes to take across the whole library.
	Count int

	// IncludeHourly keeps rolling news bulletins in the running. They are
	// excluded by default because one of them publishes more episodes in a
	// week than the rest of the library combined, and would take every slot.
	IncludeHourly bool
}

// candidate is one publishable episode with the subscription it belongs to.
type candidate struct {
	sub     Subscription
	podDir  string
	episode backend.FeedEpisode
	pubMS   int64
	held    string
	holds   bool
}

// LatestSelection is the newest episodes across the library, split by whether
// the audio still has to be fetched.
type LatestSelection struct {
	// Plans downloads the episodes that are not on disk.
	Plans []SubscriptionPlan

	// Existing are the episodes already downloaded. They still belong to the
	// selection: an episode fetched yesterday and never cleaned is exactly
	// what a "process the latest ten" request means to catch.
	Existing []string
}

// Paths is every episode in the selection, downloaded or about to be.
func (s LatestSelection) Paths() []string {
	out := append([]string(nil), s.Existing...)
	for _, p := range s.Plans {
		for _, ep := range p.ToDownload {
			out = append(out, EpisodeDestPath(p.PodDir, ep))
		}
	}
	return out
}

// PlanLatestEpisodes selects the most recently published episodes across every
// subscription.
//
// This deliberately ignores each podcast's download policy. The policy governs
// unattended downloading — most shows here are set to download nothing — while
// this is an explicit request for the newest N episodes, whatever their shows
// are configured to do. Favourite status is likewise not consulted.
func (l *Library) PlanLatestEpisodes(subs []Subscription, opts LatestEpisodePlanOptions) LatestSelection {
	if opts.Count <= 0 {
		return LatestSelection{}
	}

	var candidates []candidate
	for _, sub := range subs {
		podDir := l.PodcastDir(sub)
		if !opts.IncludeHourly && isHourlyCadence(podDir) {
			continue
		}
		feedEps := l.cachedFeedEpisodes(sub)
		if len(feedEps) == 0 {
			continue
		}
		locate := downloadedEpisodeLocator(podDir)
		for _, ep := range feedEps {
			if ep.Title == "" || episodeEnclosureURL(ep) == "" {
				continue
			}
			pub := GetPubMS(ep)
			if pub <= 0 {
				continue
			}
			heldPath, held := locate(ep)
			candidates = append(candidates, candidate{
				sub: sub, podDir: podDir, episode: ep, pubMS: pub,
				held: heldPath, holds: held,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].pubMS > candidates[j].pubMS })
	if len(candidates) > opts.Count {
		candidates = candidates[:opts.Count]
	}

	var sel LatestSelection
	var toFetch []candidate
	for _, c := range candidates {
		if c.holds {
			sel.Existing = append(sel.Existing, c.held)
			continue
		}
		toFetch = append(toFetch, c)
	}
	sel.Plans = groupCandidatesIntoPlans(l, toFetch)
	return sel
}

// EpisodeDestPath is where an episode's audio is written.
func EpisodeDestPath(podDir string, ep backend.FeedEpisode) string {
	var pubTime time.Time
	if ms := GetPubMS(ep); ms > 0 {
		pubTime = time.UnixMilli(ms).UTC()
	}
	return filepath.Join(podDir, FormatEpisodeFilename(pubTime, ep.Episode, ep.Title))
}

// groupCandidatesIntoPlans collects the chosen episodes per subscription,
// preserving the order in which the shows were first selected so the output
// reads newest-show-first.
func groupCandidatesIntoPlans(l *Library, candidates []candidate) []SubscriptionPlan {
	index := map[string]int{}
	var plans []SubscriptionPlan
	for _, c := range candidates {
		key := c.sub.FeedURL + "\x00" + c.podDir
		idx, ok := index[key]
		if !ok {
			idx = len(plans)
			index[key] = idx
			plans = append(plans, SubscriptionPlan{
				Sub:          c.sub,
				PodDir:       c.podDir,
				FeedEpisodes: l.cachedFeedEpisodes(c.sub),
			})
		}
		plans[idx].ToDownload = append(plans[idx].ToDownload, c.episode)
	}
	return plans
}

// cachedFeedEpisodes is the publication history retained for a subscription.
func (l *Library) cachedFeedEpisodes(sub Subscription) []backend.FeedEpisode {
	if sub.FeedURL == "" {
		return nil
	}
	entry := l.FeedCache().Get(sub.FeedURL)
	if entry == nil {
		return nil
	}
	return entry.FeedEpisodes()
}

// isHourlyCadence reports a show that publishes roughly every hour.
func isHourlyCadence(podDir string) bool {
	cfg := config.LoadPodcastConfig(podDir, config.PodcastConfig{})
	return cfg.Frequency != nil && cfg.Frequency.Type == string(backend.CadenceHourly)
}
