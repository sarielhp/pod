// Package podcastsearch finds podcasts by name and resolves the result a user
// picked to the upstream RSS feed URL a subscription needs.
//
// Two directories can answer that question. Podcast Index is the richer one and
// requires a per-user API key and secret; Apple's public search API requires
// nothing at all. New chooses between them from whether credentials are
// present, so a user who has not registered still gets a working search while a
// user who has gets the full catalogue.
//
// Like every library package here it writes nothing to the terminal: callers
// render results, and a search returns values or an error.
package podcastsearch

import (
	"context"
	"strings"
)

// Result is one podcast a search matched. FeedURL is what a caller subscribes
// to; ID identifies the result within the directory named by Source.
type Result struct {
	Source       string `json:"source"`
	ID           string `json:"id"`
	Title        string `json:"title"`
	Author       string `json:"author,omitempty"`
	FeedURL      string `json:"feed_url"`
	ImageURL     string `json:"image_url,omitempty"`
	Link         string `json:"link,omitempty"`
	EpisodeCount int    `json:"episode_count,omitempty"`
}

// Searcher answers a text query with podcasts and resolves a single result by
// the identifier a previous search printed.
type Searcher interface {
	Search(ctx context.Context, query string, max int) ([]Result, error)
	ByID(ctx context.Context, id string) (Result, error)
	Name() string
}

// Credentials are the Podcast Index API key and secret. An empty pair means the
// user has not registered, which selects the keyless directory instead.
type Credentials struct {
	APIKey    string
	APISecret string
}

// Configured reports whether both halves of the credential pair are present.
// Podcast Index rejects a request signed with only one of them, so a lone half
// is treated as no credential at all rather than as a request that will fail.
func (c Credentials) Configured() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.APISecret) != ""
}

// DefaultMax is how many results a search returns when the caller does not say.
const DefaultMax = 20

// maxResultsCeiling bounds any caller's larger request to something both
// directories accept without argument.
const maxResultsCeiling = 100

// ClampMax bounds a requested result count. Zero or negative selects
// DefaultMax, so a command that forgot to set a limit still gets a sane one.
func ClampMax(max int) int {
	if max <= 0 {
		return DefaultMax
	}
	if max > maxResultsCeiling {
		return maxResultsCeiling
	}
	return max
}

// New returns the directory to search: Podcast Index when credentials are
// configured, Apple's public search otherwise.
func New(creds Credentials) Searcher {
	if creds.Configured() {
		return NewPodcastIndex(creds.APIKey, creds.APISecret)
	}
	return NewITunes()
}

// Resolve turns the identifier a search printed — a directory id or a feed URL
// — into a subscribable result, using the directory the credentials select.
//
// A feed URL is already everything a subscription needs, so it is accepted
// as-is; Podcast Index is asked to enrich it with a title and artwork when a key
// is present, but its failure is not the user's problem and the bare URL is
// returned instead.
func Resolve(ctx context.Context, creds Credentials, idOrURL string) (Result, error) {
	idOrURL = strings.TrimSpace(idOrURL)
	if isFeedURL(idOrURL) {
		if creds.Configured() {
			if r, err := NewPodcastIndex(creds.APIKey, creds.APISecret).ByURL(ctx, idOrURL); err == nil {
				return r, nil
			}
		}
		return Result{Source: "feed", FeedURL: idOrURL}, nil
	}
	if creds.Configured() {
		return NewPodcastIndex(creds.APIKey, creds.APISecret).ByID(ctx, idOrURL)
	}
	return NewITunes().ByID(ctx, idOrURL)
}

func isFeedURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
