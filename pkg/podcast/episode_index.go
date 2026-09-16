package podcast

import (
	"strings"

	"pod/pkg/backend"
)

// PodcastEpisodeIndex records the episode identities a server already holds for
// one podcast, so feed episodes can be classified as known or new without
// asking the server about them one at a time.
type PodcastEpisodeIndex struct {
	known      map[string]bool
	downloaded map[string]bool
	titles     map[string]bool
	dlTitles   map[string]bool
	fallback   *PodcastEpisodeIndex
	total      int
	withAudio  int
}

func newPodcastEpisodeIndex(size int) *PodcastEpisodeIndex {
	return &PodcastEpisodeIndex{
		known:      make(map[string]bool, size*2),
		downloaded: make(map[string]bool, size*2),
		titles:     make(map[string]bool, size),
		dlTitles:   make(map[string]bool, size),
	}
}

// normalizeEnclosureURL applies the same http-to-https upgrade the feed parser
// performs, so a catalog URL recorded before that upgrade still compares equal
// to the URL the parser reports today.
func normalizeEnclosureURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "http://") {
		return "https://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

func (idx *PodcastEpisodeIndex) add(guid, enclosureURL, title string, downloaded bool) {
	record := func(set map[string]bool, titleSet map[string]bool) {
		if g := strings.TrimSpace(guid); g != "" {
			set[g] = true
		}
		if u := normalizeEnclosureURL(enclosureURL); u != "" {
			set[u] = true
		}
		if t := strings.ToLower(strings.TrimSpace(title)); t != "" {
			titleSet[t] = true
		}
	}
	record(idx.known, idx.titles)
	idx.total++
	if downloaded {
		record(idx.downloaded, idx.dlTitles)
		idx.withAudio++
	}
}

// Pending reports how many catalog episodes have no audio file on the server.
// It is the undownloaded count for a feed the origin confirmed unchanged, where
// the feed itself was never transferred and so cannot be counted against.
func (idx *PodcastEpisodeIndex) Pending() int {
	if idx == nil {
		return 0
	}
	return idx.total - idx.withAudio
}

// Total reports how many episodes the server's catalog holds.
func (idx *PodcastEpisodeIndex) Total() int {
	if idx == nil {
		return 0
	}
	return idx.total
}

func (idx *PodcastEpisodeIndex) matches(ep backend.FeedEpisode, set, titleSet map[string]bool) bool {
	if idx == nil {
		return false
	}
	if g := strings.TrimSpace(ep.GUID); g != "" && set[g] {
		return true
	}
	if u := normalizeEnclosureURL(ep.EnclosureURL); u != "" && set[u] {
		return true
	}
	if ep.Enclosure != nil {
		if u := normalizeEnclosureURL(ep.Enclosure.URL); u != "" && set[u] {
			return true
		}
	}
	t := strings.ToLower(strings.TrimSpace(ep.Title))
	return t != "" && titleSet[t]
}

// Knows reports whether the server's catalog already contains this episode,
// downloaded or not. A feed episode the catalog does not know is what makes a
// server refresh worth issuing.
func (idx *PodcastEpisodeIndex) Knows(ep backend.FeedEpisode) bool {
	if idx == nil {
		return false
	}
	if idx.matches(ep, idx.known, idx.titles) {
		return true
	}
	if idx.fallback != nil {
		return idx.fallback.Knows(ep)
	}
	return false
}

// HasAudio reports whether the server holds a downloaded audio file for this
// episode.
func (idx *PodcastEpisodeIndex) HasAudio(ep backend.FeedEpisode) bool {
	if idx == nil {
		return false
	}
	if idx.matches(ep, idx.downloaded, idx.dlTitles) {
		return true
	}
	if idx.fallback != nil {
		return idx.fallback.HasAudio(ep)
	}
	return false
}

// EpisodeIndex maps a podcast ID to the episodes the server holds for it.
type EpisodeIndex map[string]*PodcastEpisodeIndex

// Unknown returns the feed episodes the server's catalog does not know about.
func (e EpisodeIndex) Unknown(podcastID string, episodes []backend.FeedEpisode) []backend.FeedEpisode {
	idx := e[podcastID]
	var unknown []backend.FeedEpisode
	for _, ep := range episodes {
		if !idx.Knows(ep) {
			unknown = append(unknown, ep)
		}
	}
	return unknown
}

// Undownloaded returns the feed episodes with no audio file on the server.
func (e EpisodeIndex) Undownloaded(podcastID string, episodes []backend.FeedEpisode) []backend.FeedEpisode {
	idx := e[podcastID]
	var pending []backend.FeedEpisode
	for _, ep := range episodes {
		if !idx.HasAudio(ep) {
			pending = append(pending, ep)
		}
	}
	return pending
}

// BuildEpisodeIndex indexes the catalog a backend holds. It prefers the
// backend's bulk catalog reader and otherwise falls back to indexing the
// podcast records already in hand, which costs no further I/O either way.
func BuildEpisodeIndex(b backend.Backend, podcasts []backend.Podcast) EpisodeIndex {
	if b != nil {
		if catalog, ok := backend.CatalogEpisodesFrom(b); ok {
			return buildIndexFromCatalog(catalog, podcasts)
		}
	}
	return buildEpisodeIndexFromPodcasts(b, podcasts)
}

func buildIndexFromCatalog(catalog []backend.CatalogEpisode, podcasts []backend.Podcast) EpisodeIndex {
	index := make(EpisodeIndex, len(podcasts))
	global := newPodcastEpisodeIndex(len(catalog))
	for _, p := range podcasts {
		index[p.ID] = newPodcastEpisodeIndex(0)
	}
	for _, ep := range catalog {
		idx, ok := index[ep.PodcastID]
		if !ok {
			idx = newPodcastEpisodeIndex(0)
			index[ep.PodcastID] = idx
		}
		idx.add(ep.GUID, ep.EnclosureURL, ep.Title, ep.Downloaded)
		global.add(ep.GUID, ep.EnclosureURL, ep.Title, ep.Downloaded)
	}
	for _, idx := range index {
		idx.fallback = global
	}
	return index
}

// buildEpisodeIndexFromPodcasts indexes the episode lists both backends return
// inline with each podcast record.
func buildEpisodeIndexFromPodcasts(b backend.Backend, podcasts []backend.Podcast) EpisodeIndex {
	index := make(EpisodeIndex, len(podcasts))
	global := newPodcastEpisodeIndex(len(podcasts) * 50)
	for _, p := range podcasts {
		idx := newPodcastEpisodeIndex(len(p.Media.Episodes))
		for _, ep := range p.Media.Episodes {
			dl := ep.AudioFile != nil
			idx.add(ep.GUID, ep.EnclosureURL, ep.Title, dl)
			global.add(ep.GUID, ep.EnclosureURL, ep.Title, dl)
		}
		index[p.ID] = idx
	}
	for _, idx := range index {
		idx.fallback = global
	}
	return index
}
