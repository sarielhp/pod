// Package podsite renders a podcast library as a static site: an
// iTunes-compatible RSS feed and a self-contained HTML player, per show, plus
// a catalog index across shows.
//
// Every function here is pure — it takes data and returns bytes. It performs
// no filesystem access and no network access, and it imports nothing from the
// podcast library. Callers resolve the filesystem facts (which cover file
// exists, how many episodes are on disk, whether an ad-removal report sits
// next to an episode) and pass them in. Publishing the returned bytes is the
// caller's job; see podcast.PublishPodcast.
package podsite

import _ "embed"

//go:embed assets/site.css
var siteCSS string

// Show is one podcast, as the site needs to render it.
//
// CoverFile is the base name of a cover image sitting in the show's own
// directory ("cover.jpg"), or empty when there is none. ImageURL is the remote
// artwork to fall back on. CoverSrc is what the HTML <img> should point at,
// already resolved relative to the page being rendered.
type Show struct {
	Title     string
	FeedURL   string
	Folder    string
	ImageURL  string
	CoverFile string
	CoverSrc  string
}

// Episode is one episode of a show. Filename is relative to the show
// directory. ReportHref points at an ad-removal report next to the audio file,
// or is empty when there is none.
type Episode struct {
	Filename    string
	Title       string
	GUID        string
	PubDate     string
	Description string
	DurationSec float64
	SizeBytes   int64
	ReportHref  string

	// PublishedAt is the publication time in milliseconds, used to order the
	// feed. PubDate carries the same instant formatted for RSS, but ordering
	// on that string sorts "Wed, 29 Apr" above "Wed, 16 Sep" — the month is
	// text, so the comparison is alphabetical rather than chronological.
	PublishedAt int64

	// RemoteURL is where the audio lives when it is not held locally. A
	// listener subscribing to this feed should get the show's whole run, not
	// only the part that happens to have been downloaded, so an episode that
	// is not on disk is published pointing at its original enclosure. Empty
	// means the audio is local and the URL is built from Filename.
	RemoteURL string
}

// Local reports whether the audio is held here rather than upstream.
func (e Episode) Local() bool { return e.RemoteURL == "" }

// CatalogEntry is one show as it appears on the library index page.
type CatalogEntry struct {
	Title        string
	Folder       string
	CoverSrc     string
	EpisodeCount int
}

// CatalogOptions carries the facts the catalog page needs that are not per
// show. HasOPML controls whether the AntennaPod export link is rendered.
type CatalogOptions struct {
	HasOPML bool
}
