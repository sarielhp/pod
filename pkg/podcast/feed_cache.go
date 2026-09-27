package podcast

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/util"
)

type FeedCacheEntry struct {
	FeedURL      string                `json:"feed_url"`
	ETag         string                `json:"etag,omitempty"`
	LastModified string                `json:"last_modified,omitempty"`
	LastChecked  time.Time             `json:"last_checked"`
	LatestGUID   string                `json:"latest_guid,omitempty"`
	ImageURL     string                `json:"image_url,omitempty"`
	Episodes     []backend.FeedEpisode `json:"episodes,omitempty"`

	// Channel-level freshness markers, used for feeds that serve no usable
	// ETag or Last-Modified. A feed whose lastBuildDate and newest item are
	// unchanged since the previous check has nothing new to offer.
	LastBuildDate  string `json:"last_build_date,omitempty"`
	ChannelPubDate string `json:"channel_pub_date,omitempty"`

	// EpisodeCount is how many episodes the feed carried at the last check. It
	// lets an unchanged feed be reported without re-transferring its body.
	EpisodeCount int `json:"episode_count,omitempty"`

	// PubDates is the compact publication history the frequency analysis reads
	// back. Episodes, its predecessor, held whole episode records: descriptions
	// included, none of which any caller reads. It is still read so existing
	// caches keep working, but it is no longer written.
	PubDates []FeedCachePubDate `json:"pub_dates,omitempty"`
}

// FeedCachePubDate is all the frequency analysis needs of an episode.
type FeedCachePubDate struct {
	Title       string `json:"title,omitempty"`
	PublishedAt int64  `json:"published_at"`

	// GUID, URL and DurationSec let an episode that was never downloaded be
	// republished pointing at its original audio, so a listener subscribing
	// to the local feed gets the show's whole run. Descriptions are still
	// left out: they were the bulk of the old whole-episode cache and a
	// listing does not need them.
	GUID        string  `json:"guid,omitempty"`
	URL         string  `json:"url,omitempty"`
	DurationSec float64 `json:"duration_sec,omitempty"`

	// Desc is the episode's show notes, trimmed of markup and truncated.
	// Whole descriptions were what made the old episode cache expensive —
	// they run to a thousand characters each and nothing read them back. Now
	// something does: an episode published from the catalogue rather than
	// from disk has no other source of notes, and a feed item carrying only
	// its own title as its description is poor.
	Desc string `json:"desc,omitempty"`
}

// FeedCacheDescLimit caps a retained description.
//
// Measured against this library, descriptions average about a thousand
// characters; keeping them whole would add some seven megabytes to a cache
// read by every command. This keeps the opening of the notes, which is what a
// podcast client shows in a list, and drops the footer of links and credits.
const FeedCacheDescLimit = 700

const FeedCacheDefaultTTL = 48 * time.Hour

// FeedCacheRetention is how long an entry survives without being revisited. It
// is far longer than the freshness TTL, so a feed checked only occasionally
// keeps its validators, but bounded so the cache cannot grow without limit as
// subscriptions come and go.
const FeedCacheRetention = 30 * 24 * time.Hour

// FeedEpisodes reconstructs the episodes stored for the frequency analysis.
// Only titles and publication times are retained.
func (e *FeedCacheEntry) FeedEpisodes() []backend.FeedEpisode {
	if e == nil {
		return nil
	}
	if len(e.PubDates) == 0 {
		return e.Episodes
	}
	eps := make([]backend.FeedEpisode, 0, len(e.PubDates))
	for _, pd := range e.PubDates {
		eps = append(eps, backend.FeedEpisode{
			Title:            pd.Title,
			PublishedAt:      pd.PublishedAt,
			GUID:             pd.GUID,
			EnclosureURL:     pd.URL,
			DurationSeconds:  pd.DurationSec,
			Description:      plainDescription(pd.Desc),
			DescriptionPlain: plainDescription(pd.Desc),
		})
	}
	return eps
}

// FeedCachePubDateLimit caps the publication history kept per feed.
//
// Only a title and a timestamp are stored per episode, so this is cheap: what
// made the old whole-episode cache expensive was the descriptions, which no
// caller reads back. The history has two readers — the frequency analysis,
// which wants a long run of dates, and the catalogue listing, which wants the
// most recent titles — and a hundred serves both.
const FeedCachePubDateLimit = 100

// mergePubDates folds freshly fetched episodes into the retained history.
//
// A fetch used to carry the previous history across unchanged, so nothing ever
// added the episodes it had just read: the catalogue went stale the moment it
// was first written, and an episode that was never downloaded was known to no
// part of pod. Newest entries are kept when the limit bites, because that is
// what a "what is new" listing asks for.
func mergePubDates(existing []FeedCachePubDate, episodes []backend.FeedEpisode, limit int) []FeedCachePubDate {
	merged := make([]FeedCachePubDate, 0, len(existing)+len(episodes))
	// Indexed by publication time rather than by title as well, so that the
	// same episode recorded once without a title and once with one collapses
	// into a single entry instead of both surviving.
	byTime := make(map[int64][]int, len(existing)+len(episodes))

	add := func(pd FeedCachePubDate) {
		if pd.PublishedAt <= 0 && pd.Title == "" {
			return
		}
		for _, idx := range byTime[pd.PublishedAt] {
			if merged[idx].Title == pd.Title || merged[idx].Title == "" || pd.Title == "" {
				enrichPubDate(&merged[idx], pd)
				return
			}
		}
		byTime[pd.PublishedAt] = append(byTime[pd.PublishedAt], len(merged))
		merged = append(merged, pd)
	}

	for _, pd := range existing {
		add(pd)
	}
	for _, ep := range episodes {
		add(FeedCachePubDate{
			Title:       ep.Title,
			PublishedAt: ep.PublishedAt,
			GUID:        ep.GUID,
			URL:         episodeEnclosureURL(ep),
			DurationSec: ep.DurationSeconds,
			Desc:        trimDescription(ep),
		})
	}

	sort.Slice(merged, func(i, j int) bool { return merged[i].PublishedAt > merged[j].PublishedAt })
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// trimDescription reduces an episode's notes to plain text within the cache's
// budget, preferring the feed's own plain-text form when it offers one.
func trimDescription(ep backend.FeedEpisode) string {
	text := strings.TrimSpace(ep.DescriptionPlain)
	if text == "" {
		text = strings.TrimSpace(ep.Description)
	}
	if strings.ContainsRune(text, '<') || strings.ContainsRune(text, '&') {
		text = stripMarkup(text)
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= FeedCacheDescLimit {
		return text
	}
	cut := FeedCacheDescLimit
	// Prefer a word boundary so the notes do not end mid-word.
	if idx := strings.LastIndexByte(text[:cut], ' '); idx > FeedCacheDescLimit/2 {
		cut = idx
	}
	return strings.TrimSpace(text[:cut]) + "…"
}

// plainDescription cleans a retained description on the way out.
//
// Stripping only on the way in would leave entries written by an earlier
// version carrying markup for as long as they survive, and a truncated
// description ending mid-tag is worse than one with no markup at all.
func plainDescription(s string) string {
	if !strings.ContainsRune(s, '<') && !strings.ContainsRune(s, '&') {
		return s
	}
	return strings.Join(strings.Fields(stripMarkup(s)), " ")
}

var markupTag = regexp.MustCompile(`<[^>]*>`)

func stripMarkup(s string) string {
	return html.UnescapeString(markupTag.ReplaceAllString(s, " "))
}

// episodeEnclosureURL is where a feed says the audio lives.
// enrichPubDate fills gaps in a retained record from a newly seen one. A
// history written before enclosure URLs were kept has titles and timestamps
// only, and must gain the rest as feeds are re-read rather than staying
// half-empty forever.
func enrichPubDate(dst *FeedCachePubDate, src FeedCachePubDate) {
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.GUID == "" {
		dst.GUID = src.GUID
	}
	if dst.URL == "" {
		dst.URL = src.URL
	}
	if dst.DurationSec <= 0 {
		dst.DurationSec = src.DurationSec
	}
	if dst.Desc == "" {
		dst.Desc = src.Desc
	}
}

func episodeEnclosureURL(ep backend.FeedEpisode) string {
	if ep.EnclosureURL != "" {
		return ep.EnclosureURL
	}
	if ep.Enclosure != nil {
		return ep.Enclosure.URL
	}
	return ""
}

func pubDatesFromEpisodes(episodes []backend.FeedEpisode) []FeedCachePubDate {
	dates := make([]FeedCachePubDate, 0, len(episodes))
	for _, ep := range episodes {
		dates = append(dates, FeedCachePubDate{Title: ep.Title, PublishedAt: ep.PublishedAt})
	}
	return dates
}

func (e *FeedCacheEntry) IsExpired(ttl time.Duration) bool {
	if e == nil || e.LastChecked.IsZero() {
		return true
	}
	return time.Since(e.LastChecked) > ttl
}

type FeedCacheManager struct {
	mu        util.RWMutex
	cacheFile string
	entries   map[string]*FeedCacheEntry
	dirty     bool
}

var globalFeedCacheOnce util.Once
var globalFeedCache *FeedCacheManager

// defaultFeedCache is the process-wide feed cache for the default cache path.
// It is unexported: callers outside this package reach it through
// Library.FeedCache, so library state is owned rather than ambient.
func defaultFeedCache() *FeedCacheManager {
	globalFeedCacheOnce.Do(func() {
		globalFeedCache = newFeedCacheManager("")
	})
	return globalFeedCache
}

func feedCachePath() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "feed_cache.json")
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	podDir := filepath.Join(cacheHome, "pod")
	if _, err := os.Stat(filepath.Join(podDir, "feed_cache.json")); err == nil {
		return filepath.Join(podDir, "feed_cache.json")
	}
	legacyDir := filepath.Join(cacheHome, "abs")
	if _, err := os.Stat(filepath.Join(legacyDir, "feed_cache.json")); err == nil {
		return filepath.Join(legacyDir, "feed_cache.json")
	}
	_ = os.MkdirAll(podDir, 0755)
	return filepath.Join(podDir, "feed_cache.json")
}

func newFeedCacheManager(cachePath string) *FeedCacheManager {
	if cachePath == "" {
		cachePath = feedCachePath()
	}
	mgr := &FeedCacheManager{
		cacheFile: cachePath,
		entries:   make(map[string]*FeedCacheEntry),
	}
	mgr.load()
	// Persist a prune straight away: a run that only reads the cache would
	// otherwise leave the pruned entries on disk indefinitely.
	_ = mgr.Save()
	return mgr
}

func (m *FeedCacheManager) load() {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.cacheFile)
	if err != nil {
		return
	}
	var loaded map[string]*FeedCacheEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	m.entries = loaded
	m.dirty = pruneFeedEntries(loaded, FeedCacheRetention)
}

// pruneFeedEntries drops entries nothing has revisited inside the retention
// window and folds legacy whole-episode records into the compact publication
// history that replaced them. Nothing ever evicted anything before, so a
// long-lived cache accumulated one entry per feed URL ever seen, the bulk of
// the bytes being episode descriptions no caller reads back. It reports whether
// anything changed, so the caller knows to rewrite the file.
func pruneFeedEntries(entries map[string]*FeedCacheEntry, retention time.Duration) bool {
	changed := false
	for url, entry := range entries {
		if entry == nil || entry.IsExpired(retention) {
			delete(entries, url)
			changed = true
			continue
		}
		if len(entry.Episodes) > 0 {
			if len(entry.PubDates) == 0 {
				entry.PubDates = pubDatesFromEpisodes(entry.Episodes)
			}
			entry.Episodes = nil
			changed = true
		}
	}
	return changed
}

func (m *FeedCacheManager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.dirty {
		return nil
	}
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return err
	}
	err = util.WriteFileAtomic(m.cacheFile, data, 0644)
	if err == nil {
		m.dirty = false
	}
	return err
}

func (m *FeedCacheManager) Get(feedURL string) *FeedCacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if entry, ok := m.entries[feedURL]; ok {
		cp := *entry
		return &cp
	}
	return nil
}

func (m *FeedCacheManager) Put(feedURL string, entry *FeedCacheEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[feedURL] = entry
	m.dirty = true
}
