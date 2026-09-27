package podcast

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"pod/pkg/backend"
	"pod/pkg/util"
)

var (
	testFeedRetryMu    util.Mutex
	testFeedRetryDelay *time.Duration
	testFeedTransport  http.RoundTripper
)

// SetFeedRetryDelay overrides the wait between feed fetch attempts. Pass a
// pointer to zero to remove the wait entirely; pass nil to restore the default
// backoff. A plain duration cannot express "no wait", which is why this takes
// a pointer — the previous hook treated zero as "unset" and so could only ever
// make the wait longer.
func SetFeedRetryDelay(d *time.Duration) {
	testFeedRetryMu.Lock()
	defer testFeedRetryMu.Unlock()
	testFeedRetryDelay = d
}

// SetFeedTransport substitutes the HTTP transport used for every feed fetch.
// Tests install one that refuses to leave the machine, so a suite cannot
// silently depend on the network, on a third party's uptime, or on DNS.
func SetFeedTransport(rt http.RoundTripper) {
	testFeedRetryMu.Lock()
	defer testFeedRetryMu.Unlock()
	testFeedTransport = rt
}

func feedRetryDelay() (time.Duration, bool) {
	testFeedRetryMu.Lock()
	defer testFeedRetryMu.Unlock()
	if testFeedRetryDelay == nil {
		return 0, false
	}
	return *testFeedRetryDelay, true
}

func activeFeedTransport() http.RoundTripper {
	testFeedRetryMu.Lock()
	defer testFeedRetryMu.Unlock()
	if testFeedTransport != nil {
		return testFeedTransport
	}
	return feedTransport
}

func isTransientHTTPStatus(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusRequestTimeout || code >= 500
}

func feedSleepBackoff(attempt int) {
	if d, ok := feedRetryDelay(); ok {
		if d > 0 {
			time.Sleep(d)
		}
		return
	}
	jitter := time.Duration(rand.Intn(500)) * time.Millisecond
	time.Sleep(time.Duration(attempt)*time.Second + jitter)
}

const (
	feedUserAgent            = "Mozilla/5.0 (compatible; ABSPodcastManager/1.0)"
	defaultFeedFetchTimeout  = 15 * time.Second
	defaultFeedFetchAttempts = 2
	maxFeedSize              = 32 * 1024 * 1024
)

// feedTransport is shared by every feed fetch. A feed sweep opens connections
// to dozens of hosts at once and repeats the sweep on later runs, so pooling
// connections and TLS sessions across calls is worth far more than the
// isolation a per-call transport would buy.
var feedTransport = &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	MaxIdleConns:        128,
	MaxIdleConnsPerHost: 4,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

func feedHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultFeedFetchTimeout
	}
	return &http.Client{Transport: activeFeedTransport(), Timeout: timeout}
}

// FeedFetchOptions configures a single conditional feed fetch.
type FeedFetchOptions struct {
	ETag         string
	LastModified string
	Timeout      time.Duration
	MaxAttempts  int
	Client       *http.Client
}

// FeedFetchResult is the outcome of a conditional feed fetch. When NotModified
// is set the origin answered 304, no body was transferred, and Doc is nil.
type FeedFetchResult struct {
	Doc          *FeedDocument
	ETag         string
	LastModified string
	NotModified  bool
}

// fetchFeedConditional fetches a feed, sending If-None-Match/If-Modified-Since
// when the caller has cached validators. Most podcast hosts honour them and
// answer 304, which is the cheapest possible way to learn that a feed has
// nothing new.
func fetchFeedConditional(feedURL string, opts FeedFetchOptions) (FeedFetchResult, error) {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultFeedFetchAttempts
	}
	client := opts.Client
	if client == nil {
		client = feedHTTPClient(opts.Timeout)
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		res, retry, err := fetchFeedAttempt(client, feedURL, opts, attempt)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !retry || attempt == maxAttempts {
			break
		}
		feedSleepBackoff(attempt)
	}
	return FeedFetchResult{}, fmt.Errorf("fetch feed %s: %w", feedURL, lastErr)
}

func fetchFeedAttempt(client *http.Client, feedURL string, opts FeedFetchOptions, attempt int) (FeedFetchResult, bool, error) {
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return FeedFetchResult{}, false, err
	}
	req.Header.Set("User-Agent", feedUserAgent)
	if opts.ETag != "" {
		req.Header.Set("If-None-Match", opts.ETag)
	}
	if opts.LastModified != "" {
		req.Header.Set("If-Modified-Since", opts.LastModified)
	}

	resp, err := client.Do(req)
	if err != nil {
		return FeedFetchResult{}, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return FeedFetchResult{
			ETag:         opts.ETag,
			LastModified: opts.LastModified,
			NotModified:  true,
		}, false, nil
	}

	if isTransientHTTPStatus(resp.StatusCode) {
		return FeedFetchResult{}, true, fmt.Errorf("transient HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return FeedFetchResult{}, false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return readFeedResponse(resp, opts)
}

func readFeedResponse(resp *http.Response, opts FeedFetchOptions) (FeedFetchResult, bool, error) {
	res := FeedFetchResult{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	if res.ETag == "" {
		res.ETag = opts.ETag
	}
	if res.LastModified == "" {
		res.LastModified = opts.LastModified
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedSize))
	if err != nil {
		return FeedFetchResult{}, true, err
	}
	doc, err := parseRSSFeed(data)
	if err != nil {
		return FeedFetchResult{}, false, err
	}
	res.Doc = doc
	return res, false, nil
}

func FetchFeedDirect(feedURL string, cachedETag, cachedLastMod string) ([]backend.FeedEpisode, string, string, bool, error) {
	res, err := fetchFeedConditional(feedURL, FeedFetchOptions{
		ETag:         cachedETag,
		LastModified: cachedLastMod,
		Timeout:      60 * time.Second,
		MaxAttempts:  3,
	})
	if err != nil {
		return nil, "", "", false, err
	}
	if res.NotModified {
		return nil, res.ETag, res.LastModified, true, nil
	}
	if res.Doc != nil && res.Doc.ImageURL != "" {
		cacheDocImage(feedURL, res.Doc.ImageURL)
	}
	return res.Doc.Episodes, res.ETag, res.LastModified, false, nil
}

func cacheDocImage(feedURL, imageURL string) {
	entry := defaultFeedCache().Get(feedURL)
	if entry != nil {
		if entry.ImageURL == "" {
			entry.ImageURL = imageURL
			defaultFeedCache().Put(feedURL, entry)
		}
		return
	}
	defaultFeedCache().Put(feedURL, &FeedCacheEntry{
		FeedURL:  feedURL,
		ImageURL: imageURL,
	})
}

func fetchFeedDoc(feedURL string) (*FeedDocument, error) {
	res, err := fetchFeedConditional(feedURL, FeedFetchOptions{
		Timeout:     60 * time.Second,
		MaxAttempts: 3,
	})
	if err != nil {
		return nil, err
	}
	return res.Doc, nil
}
