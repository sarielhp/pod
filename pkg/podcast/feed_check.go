package podcast

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/util"
)

// DefaultFeedCheckConcurrency is how many feeds are checked at once. Each feed
// belongs to a different publisher, so a sweep is bound by round trips to
// unrelated origins rather than by local resources or by politeness to any one
// host. Raise it with --jobs on a library large enough to notice.
const DefaultFeedCheckConcurrency = 24

// FeedStatus classifies what checking a feed established.
type FeedStatus int

const (
	// FeedUnchanged means the origin confirmed the feed is identical to the
	// previous check, so it cannot contain anything new.
	FeedUnchanged FeedStatus = iota
	// FeedChanged means the feed differs from the previous check.
	FeedChanged
	// FeedUnknown means the feed could not be read.
	FeedUnknown
)

func (s FeedStatus) String() string {
	switch s {
	case FeedUnchanged:
		return "unchanged"
	case FeedChanged:
		return "changed"
	default:
		return "unknown"
	}
}

// FeedCheckOptions configures a sweep across many feeds.
type FeedCheckOptions struct {
	Concurrency int
	Force       bool
	Timeout     time.Duration
	MaxAttempts int
	Cache       *FeedCacheManager
}

// FeedCheckResult is what the sweep established about one podcast.
type FeedCheckResult struct {
	Podcast      backend.Podcast
	Title        string
	FeedURL      string
	Status       FeedStatus
	Reason       string
	EpisodeCount int
	Episodes     []backend.FeedEpisode
	New          []backend.FeedEpisode
	Undownloaded int
	Err          error
}

// NeedsServer reports whether the server has to be asked to update this
// podcast. A feed the origin confirmed unchanged never does, which is the whole
// point of checking feeds directly first.
func (r *FeedCheckResult) NeedsServer() bool {
	switch r.Status {
	case FeedUnknown:
		return true
	default:
		return len(r.New) > 0
	}
}

// CheckFeedsForUpdates checks every podcast's feed directly, concurrently and
// conditionally, and reports which ones actually changed. Validators and
// content markers are written back to the feed cache so the next sweep can be
// answered by the origin with a bodyless 304.
func CheckFeedsForUpdates(podcasts []backend.Podcast, index EpisodeIndex, opts FeedCheckOptions) []FeedCheckResult {
	cache := opts.Cache
	if cache == nil {
		cache = defaultFeedCache()
	}
	results := make([]FeedCheckResult, len(podcasts))
	if len(podcasts) == 0 {
		return results
	}

	client := feedHTTPClient(opts.Timeout)
	jobs := make(chan int)
	var wg util.WaitGroup

	for w := 0; w < feedCheckWorkers(opts.Concurrency, len(podcasts)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = checkOneFeed(podcasts[i], index, cache, client, opts)
			}
		}()
	}
	for i := range podcasts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	_ = cache.Save()
	return results
}

func FeedCheckWorkers(requested, total int) int {
	if requested <= 0 {
		requested = DefaultFeedCheckConcurrency
	}
	if requested > total {
		requested = total
	}
	return requested
}

func feedCheckWorkers(requested, total int) int {
	return FeedCheckWorkers(requested, total)
}

func checkOneFeed(item backend.Podcast, index EpisodeIndex, cache *FeedCacheManager, client *http.Client, opts FeedCheckOptions) FeedCheckResult {
	res := FeedCheckResult{
		Podcast: item,
		Title:   podcastDisplayTitle(item),
		FeedURL: strings.TrimSpace(item.Media.Metadata.FeedURL),
	}
	if res.FeedURL == "" {
		res.Status = FeedUnknown
		res.Err = fmt.Errorf("no feed URL configured")
		res.Reason = res.Err.Error()
		return res
	}

	entry := cache.Get(res.FeedURL)
	fetchOpts := FeedFetchOptions{
		Timeout:     opts.Timeout,
		MaxAttempts: opts.MaxAttempts,
		Client:      client,
	}
	if entry != nil && !opts.Force && serverKnowsLatest(entry, index[item.ID]) {
		fetchOpts.ETag = entry.ETag
		fetchOpts.LastModified = entry.LastModified
	}

	fetched, err := fetchFeedConditional(res.FeedURL, fetchOpts)
	if err != nil {
		res.Status = FeedUnknown
		res.Err = err
		res.Reason = err.Error()
		if entry != nil {
			res.EpisodeCount = entry.EpisodeCount
		}
		return res
	}

	if fetched.NotModified {
		return unchangedResult(res, entry, index, cache, "HTTP 304 not modified")
	}
	return classifyFetchedFeed(res, fetched, entry, index, cache, opts)
}

func serverKnowsLatest(entry *FeedCacheEntry, pIndex *PodcastEpisodeIndex) bool {
	if entry == nil || pIndex == nil {
		return false
	}
	if entry.LatestGUID != "" && !pIndex.Knows(backend.FeedEpisode{GUID: entry.LatestGUID}) {
		return false
	}
	if entry.EpisodeCount > 0 && pIndex.Total() < entry.EpisodeCount {
		return false
	}
	return true
}

func unchangedResult(res FeedCheckResult, entry *FeedCacheEntry, index EpisodeIndex, cache *FeedCacheManager, reason string) FeedCheckResult {
	res.Status = FeedUnchanged
	res.Reason = reason
	res.Undownloaded = index[res.Podcast.ID].Pending()
	if entry != nil {
		res.EpisodeCount = entry.EpisodeCount
		entry.LastChecked = time.Now()
		cache.Put(res.FeedURL, entry)
		if !serverKnowsLatest(entry, index[res.Podcast.ID]) {
			res.Reason = "server catalog missing latest episode"
			if entry.LatestGUID != "" {
				res.New = []backend.FeedEpisode{{GUID: entry.LatestGUID}}
			}
		}
	}
	if res.EpisodeCount == 0 {
		res.EpisodeCount = index[res.Podcast.ID].Total()
	}
	return res
}

func classifyFetchedFeed(res FeedCheckResult, fetched FeedFetchResult, entry *FeedCacheEntry, index EpisodeIndex, cache *FeedCacheManager, opts FeedCheckOptions) FeedCheckResult {
	doc := fetched.Doc
	latest := doc.LatestGUID()
	res.EpisodeCount = len(doc.Episodes)

	updated := &FeedCacheEntry{
		FeedURL:        res.FeedURL,
		ETag:           fetched.ETag,
		LastModified:   fetched.LastModified,
		LastChecked:    time.Now(),
		LatestGUID:     latest,
		ImageURL:       doc.ImageURL,
		LastBuildDate:  doc.LastBuildDate,
		ChannelPubDate: doc.ChannelPubDate,
		EpisodeCount:   len(doc.Episodes),
	}
	if entry != nil {
		if updated.ImageURL == "" {
			updated.ImageURL = entry.ImageURL
		}
		// The sweep replaces the entry outright, so carry the publication
		// history over rather than discarding it — and fold in what this fetch
		// just read, which nothing did before, leaving the catalogue frozen at
		// whenever it was first written.
		updated.PubDates = entry.PubDates
	}
	updated.PubDates = mergePubDates(updated.PubDates, doc.Episodes, FeedCachePubDateLimit)
	cache.Put(res.FeedURL, updated)

	res.Episodes = doc.Episodes
	res.New = index.Unknown(res.Podcast.ID, doc.Episodes)
	res.Undownloaded = len(index.Undownloaded(res.Podcast.ID, doc.Episodes))

	if !opts.Force && feedMarkersUnchanged(entry, doc, latest) {
		res.Status = FeedUnchanged
		res.Reason = "content markers unchanged"
		return res
	}

	res.Status = FeedChanged
	res.Reason = "feed body changed"
	return res
}

// feedMarkersUnchanged reports whether a freshly downloaded feed carries the
// same content markers as the previous check. About one feed in six serves no
// usable ETag or Last-Modified, and their markup often differs byte-for-byte on
// every request, so the channel's build date, the episode count and the newest
// episode's identity are what distinguish a genuinely new episode from noise.
func feedMarkersUnchanged(entry *FeedCacheEntry, doc *FeedDocument, latest string) bool {
	if entry == nil || entry.LastChecked.IsZero() {
		return false
	}
	if entry.EpisodeCount != len(doc.Episodes) {
		return false
	}
	if doc.LastBuildDate != "" && entry.LastBuildDate != "" {
		return doc.LastBuildDate == entry.LastBuildDate && latest == entry.LatestGUID
	}
	if latest == "" || entry.LatestGUID == "" {
		return false
	}
	return latest == entry.LatestGUID
}

// podcastDisplayTitle is the podcast's title, or a placeholder when the server
// records none.
func podcastDisplayTitle(item backend.Podcast) string {
	if title := strings.TrimSpace(item.Media.Metadata.Title); title != "" {
		return title
	}
	return "Untitled Podcast"
}
