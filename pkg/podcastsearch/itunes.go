package podcastsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Apple's public podcast directory. Unlike Podcast Index it needs no key and no
// signing, which is what makes it usable the moment pod is installed.
const (
	itunesSearchURL = "https://itunes.apple.com/search"
	itunesLookupURL = "https://itunes.apple.com/lookup"
)

// itunes is the keyless fallback searcher. Fields are settable for tests.
type itunes struct {
	searchURL string
	lookupURL string
	client    *http.Client
}

// NewITunes returns a searcher backed by Apple's public podcast search.
func NewITunes() *itunes {
	return &itunes{
		searchURL: itunesSearchURL,
		lookupURL: itunesLookupURL,
		client:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (i *itunes) Name() string { return "Apple Podcasts" }

func (i *itunes) Search(ctx context.Context, query string, max int) ([]Result, error) {
	q := url.Values{}
	q.Set("media", "podcast")
	q.Set("entity", "podcast")
	q.Set("term", query)
	q.Set("limit", strconv.Itoa(ClampMax(max)))
	items, err := i.fetch(ctx, i.searchURL, q)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(items))
	for _, item := range items {
		if r, ok := item.result(); ok {
			results = append(results, r)
		}
	}
	return results, nil
}

func (i *itunes) ByID(ctx context.Context, id string) (Result, error) {
	id = strings.TrimSpace(id)
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return Result{}, fmt.Errorf("apple podcasts ids are numbers, got %q", id)
	}
	q := url.Values{}
	q.Set("id", id)
	q.Set("entity", "podcast")
	items, err := i.fetch(ctx, i.lookupURL, q)
	if err != nil {
		return Result{}, err
	}
	for _, item := range items {
		if r, ok := item.result(); ok {
			return r, nil
		}
	}
	return Result{}, fmt.Errorf("no Apple Podcasts podcast for id %s", id)
}

func (i *itunes) fetch(ctx context.Context, endpoint string, params url.Values) ([]itunesItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", searchUserAgent)

	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apple podcasts request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read apple podcasts response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apple podcasts returned HTTP %d", resp.StatusCode)
	}
	var out struct {
		Results []itunesItem `json:"results"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse apple podcasts response: %w", err)
	}
	return out.Results, nil
}

// itunesItem is one entry of Apple's search or lookup response.
type itunesItem struct {
	CollectionID      int64  `json:"collectionId"`
	CollectionName    string `json:"collectionName"`
	ArtistName        string `json:"artistName"`
	FeedURL           string `json:"feedUrl"`
	ArtworkURL        string `json:"artworkUrl600"`
	TrackCount        int    `json:"trackCount"`
	CollectionViewURL string `json:"collectionViewUrl"`
}

// result converts an Apple entry, reporting false for anything without an RSS
// feed URL. A music album, or a podcast Apple exposes no feed for, cannot be
// subscribed to; showing it would only invite a dead end.
func (it itunesItem) result() (Result, bool) {
	feed := strings.TrimSpace(it.FeedURL)
	if feed == "" || it.CollectionID == 0 {
		return Result{}, false
	}
	return Result{
		Source:       "Apple Podcasts",
		ID:           strconv.FormatInt(it.CollectionID, 10),
		Title:        firstNonBlank(it.CollectionName, it.ArtistName),
		Author:       strings.TrimSpace(it.ArtistName),
		FeedURL:      feed,
		ImageURL:     strings.TrimSpace(it.ArtworkURL),
		Link:         strings.TrimSpace(it.CollectionViewURL),
		EpisodeCount: it.TrackCount,
	}, true
}
