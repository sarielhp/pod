package podcast

import (
	"fmt"
	"sort"

	"pod/pkg/backend"
	"pod/pkg/util"
)

// Planning a download run means asking every podcast's feed what it currently
// offers and comparing that against what the server already holds. Those
// questions are independent of one another, so they are asked concurrently:
// asked in sequence, a library of eighty podcasts costs eighty round trips
// before the first episode is queued.

// DownloadPlan is what one podcast turned out to need: the episodes selected
// for download and the reasons they were selected, or the error that stopped
// the feed from being read at all. Unknown holds selected episodes the server
// cannot be asked for because it has never ingested them.
type DownloadPlan struct {
	Item     backend.Podcast
	Episodes []backend.FeedEpisode
	Unknown  []backend.FeedEpisode
	Reasons  []string
	Err      error
}

// Title names the podcast a plan belongs to, for reporting.
func (p *DownloadPlan) Title() string {
	if t := p.Item.Media.Metadata.Title; t != "" {
		return t
	}
	return "Untitled Podcast"
}

// planPodcastDownloads reads one podcast's feed and decides which episodes to
// download. It performs no output and queues nothing, so it is safe to run for
// many podcasts at once; index carries what the server already holds, read once
// for the whole run rather than per podcast.
func planPodcastDownloads(client backend.Backend, item backend.Podcast, index *PodcastEpisodeIndex, opts DownloadOptions) DownloadPlan {
	plan := DownloadPlan{Item: item}

	feedURL := item.Media.Metadata.FeedURL
	if feedURL == "" {
		plan.Err = fmt.Errorf("no RSS feed URL configured")
		return plan
	}

	feedEpisodes, err := client.PodcastFeedEpisodes(feedURL)
	if err != nil {
		plan.Err = fmt.Errorf("fetch episode catalog: %w", err)
		return plan
	}

	var active []backend.ActiveDownload
	if client != nil {
		active, _ = client.ActiveDownloads(item.ID)
	}
	isDownloaded := buildDownloadedChecker(item, index, active, opts.PodcastsDir)

	sortedCatalog := make([]backend.FeedEpisode, len(feedEpisodes))
	copy(sortedCatalog, feedEpisodes)
	sort.Slice(sortedCatalog, func(i, j int) bool {
		return GetPubMS(sortedCatalog[i]) < GetPubMS(sortedCatalog[j])
	})

	var downloadedIndices []int
	for idx, ep := range sortedCatalog {
		if isDownloaded(ep) {
			downloadedIndices = append(downloadedIndices, idx)
		}
	}

	plan.Episodes, plan.Reasons = resolveEpisodesToDownload(item, sortedCatalog, downloadedIndices, isDownloaded, opts)
	if catalogBound(client) {
		plan.Episodes, plan.Unknown = splitByCatalog(plan.Episodes, index)
	}
	return plan
}

func catalogBound(client backend.Backend) bool {
	return false
}

// splitByCatalog separates the selected episodes the server already knows from
// the ones it does not, so a run can say which episodes it is not going to be
// able to fetch and why instead of silently failing to fetch them.
func splitByCatalog(selected []backend.FeedEpisode, index *PodcastEpisodeIndex) (known, unknown []backend.FeedEpisode) {
	for _, ep := range selected {
		if index.Knows(ep) {
			known = append(known, ep)
		} else {
			unknown = append(unknown, ep)
		}
	}
	return known, unknown
}

// PlanDownloads plans every target podcast concurrently, reporting progress as
// each one finishes. The returned plans are in the order the podcasts were
// given, so the run reads the same way whatever order the feeds answered in.
func PlanDownloads(client backend.Backend, items []backend.Podcast, index EpisodeIndex, opts DownloadOptions, progress func(done, total int)) []DownloadPlan {
	plans := make([]DownloadPlan, len(items))
	if len(items) == 0 {
		return plans
	}

	jobs := make(chan int)
	var wg util.WaitGroup
	var mu util.Mutex
	done := 0

	workers := feedCheckWorkers(opts.Jobs, len(items))
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				plans[i] = planPodcastDownloads(client, items[i], index[items[i].ID], opts)
				mu.Lock()
				done++
				if progress != nil {
					progress(done, len(items))
				}
				mu.Unlock()
			}
		}()
	}
	for i := range items {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return plans
}
