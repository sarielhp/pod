package podcastsearch

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PodcastIndexBaseURL is the API root. The version segment is part of it
// because Podcast Index versions its whole surface at once.
const PodcastIndexBaseURL = "https://api.podcastindex.org/api/1.0"

// searchUserAgent identifies pod to the directories. Podcast Index asks callers
// to send something recognisable, and Apple's API is happier with a UA than
// without one.
const searchUserAgent = "pod/1.0 (podcast search)"

// maxResponseBytes caps a directory response. Search payloads run to tens of
// kilobytes; the cap exists so a hostile or broken endpoint cannot stream until
// memory runs out.
const maxResponseBytes = 8 << 20

// podcastIndex is the Podcast Index client. Its fields are settable so a test
// can point it at a local server and a fixed clock without touching the network.
type podcastIndex struct {
	apiKey    string
	apiSecret string
	baseURL   string
	client    *http.Client
	now       func() time.Time
}

// NewPodcastIndex returns a Podcast Index searcher signed with the given
// credentials.
func NewPodcastIndex(apiKey, apiSecret string) *podcastIndex {
	return &podcastIndex{
		apiKey:    strings.TrimSpace(apiKey),
		apiSecret: strings.TrimSpace(apiSecret),
		baseURL:   PodcastIndexBaseURL,
		client:    &http.Client{Timeout: 20 * time.Second},
		now:       time.Now,
	}
}

func (p *podcastIndex) Name() string { return "Podcast Index" }

func (p *podcastIndex) Search(ctx context.Context, query string, max int) ([]Result, error) {
	var out struct {
		Status      string   `json:"status"`
		Feeds       []piFeed `json:"feeds"`
		Description string   `json:"description"`
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("max", strconv.Itoa(ClampMax(max)))
	if err := p.get(ctx, "/search/byterm", q, &out); err != nil {
		return nil, err
	}
	if err := piStatus(out.Status, out.Description); err != nil {
		return nil, err
	}
	return piResults(out.Feeds), nil
}

func (p *podcastIndex) ByID(ctx context.Context, id string) (Result, error) {
	feedID, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil {
		return Result{}, fmt.Errorf("podcast index ids are numbers, got %q", id)
	}
	var out piFeedResponse
	q := url.Values{}
	q.Set("id", strconv.Itoa(feedID))
	if err := p.get(ctx, "/podcasts/byfeedid", q, &out); err != nil {
		return Result{}, err
	}
	if err := piStatus(out.Status, out.Description); err != nil {
		return Result{}, err
	}
	return out.Feed.result(), nil
}

func (p *podcastIndex) ByURL(ctx context.Context, feedURL string) (Result, error) {
	var out piFeedResponse
	q := url.Values{}
	q.Set("url", feedURL)
	if err := p.get(ctx, "/podcasts/byfeedurl", q, &out); err != nil {
		return Result{}, err
	}
	if err := piStatus(out.Status, out.Description); err != nil {
		return Result{}, err
	}
	return out.Feed.result(), nil
}

// get signs and performs one request, decoding the body into out.
func (p *podcastIndex) get(ctx context.Context, path string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	authDate := strconv.FormatInt(p.now().Unix(), 10)
	sum := sha1.Sum([]byte(p.apiKey + p.apiSecret + authDate))
	req.Header.Set("User-Agent", searchUserAgent)
	req.Header.Set("X-Auth-Key", p.apiKey)
	req.Header.Set("X-Auth-Date", authDate)
	req.Header.Set("Authorization", hex.EncodeToString(sum[:]))

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("podcast index request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read podcast index response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("podcast index returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse podcast index response: %w", err)
	}
	return nil
}

// piFeedResponse is the envelope a single-feed lookup returns.
type piFeedResponse struct {
	Status      string `json:"status"`
	Feed        piFeed `json:"feed"`
	Description string `json:"description"`
}

// piStatus turns Podcast Index's own success flag into an error. It reports
// failures with HTTP 200 and status "false", carrying the reason in
// description, so the status code alone is not enough.
func piStatus(status, description string) error {
	if strings.EqualFold(strings.TrimSpace(status), "true") {
		return nil
	}
	if d := strings.TrimSpace(description); d != "" {
		return fmt.Errorf("podcast index: %s", d)
	}
	return fmt.Errorf("podcast index rejected the request")
}

func piResults(feeds []piFeed) []Result {
	results := make([]Result, 0, len(feeds))
	for _, f := range feeds {
		results = append(results, f.result())
	}
	return results
}

// piFeed is the Podcast Index feed object, trimmed to the fields a listing and
// a subscription use.
type piFeed struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	OriginalURL  string `json:"originalUrl"`
	Link         string `json:"link"`
	Image        string `json:"image"`
	Artwork      string `json:"artwork"`
	Author       string `json:"author"`
	OwnerName    string `json:"ownerName"`
	EpisodeCount int    `json:"episodeCount"`
}

func (f piFeed) result() Result {
	feed := firstNonBlank(f.OriginalURL, f.URL)
	return Result{
		Source:       "Podcast Index",
		ID:           strconv.Itoa(f.ID),
		Title:        strings.TrimSpace(f.Title),
		Author:       firstNonBlank(f.Author, f.OwnerName),
		FeedURL:      feed,
		ImageURL:     firstNonBlank(f.Artwork, f.Image),
		Link:         strings.TrimSpace(f.Link),
		EpisodeCount: f.EpisodeCount,
	}
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
