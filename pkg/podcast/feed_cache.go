package podcast

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
	// Both fields arrive carrying markup from some feeds, so strip
	// unconditionally rather than trusting the name of the field.
	text = strings.Join(strings.Fields(stripMarkup(text)), " ")
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

type rssXML struct {
	XMLName xml.Name   `xml:"rss"`
	Channel channelXML `xml:"channel"`
}

type channelXML struct {
	Title         string                `xml:"title"`
	LastBuildDate string                `xml:"lastBuildDate"`
	PubDate       string                `xml:"pubDate"`
	Image         channelImageXML       `xml:"image"`
	ITunesImage   channelItunesImageXML `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	ITunesImage2  channelItunesImageXML `xml:"http://www.itunes.com/DTDs/Podcast-1.0.dtd image"`
	Items         []itemXML             `xml:"item"`
}

type channelImageXML struct {
	URL  string `xml:"url"`
	Href string `xml:"href,attr"`
}

type channelItunesImageXML struct {
	Href string `xml:"href,attr"`
	URL  string `xml:"url,attr"`
}

type itemXML struct {
	Title       string        `xml:"title"`
	Description string        `xml:"description"`
	PubDate     string        `xml:"pubDate"`
	GUID        guidXML       `xml:"guid"`
	ID          string        `xml:"id"`
	Enclosure   *enclosureXML `xml:"enclosure"`
	Duration    string        `xml:"duration"`
	Season      string        `xml:"season"`
	Episode     string        `xml:"episode"`
}

type guidXML struct {
	Value string `xml:",chardata"`
}

type enclosureXML struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

var feedTZOffsets = [...]struct {
	suffix string
	offset string
}{
	{" PDT", " -0700"},
	{" PST", " -0800"},
	{" EDT", " -0400"},
	{" EST", " -0500"},
	{" CDT", " -0500"},
	{" CST", " -0600"},
	{" MDT", " -0600"},
	{" MST", " -0700"},
	{" AKDT", " -0800"},
	{" AKST", " -0900"},
	{" HST", " -1000"},
	{" UTC", " +0000"},
	{" GMT", " +0000"},
	{" UT", " +0000"},
	{" Z", " +0000"},
	{" BST", " +0100"},
	{" WEST", " +0100"},
	{" WET", " +0000"},
	{" CEST", " +0200"},
	{" CET", " +0100"},
	{" EEST", " +0300"},
	{" EET", " +0200"},
	{" IDT", " +0300"},
	{" IST", " +0200"},
	{" MSK", " +0300"},
	{" JST", " +0900"},
	{" KST", " +0900"},
	{" AEST", " +1000"},
	{" AEDT", " +1100"},
	{" AWST", " +0800"},
	{" NZST", " +1200"},
	{" NZDT", " +1300"},
}

var feedDateFormats = [...]string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.RFC3339,
	time.RFC3339Nano,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05-0700",
	"2006-01-02",
}

func normalizeFeedTimezone(s string) string {
	for _, tz := range feedTZOffsets {
		if len(s) >= len(tz.suffix) && strings.EqualFold(s[len(s)-len(tz.suffix):], tz.suffix) {
			return s[:len(s)-len(tz.suffix)] + tz.offset
		}
	}
	return s
}

func parseFeedDate(pubDate string) (int64, string) {
	pubDate = strings.TrimSpace(pubDate)
	if pubDate == "" {
		return 0, ""
	}

	normalizedPubDate := normalizeFeedTimezone(pubDate)
	for _, layout := range feedDateFormats {
		if t, err := time.Parse(layout, normalizedPubDate); err == nil {
			return t.UnixMilli(), pubDate
		}
	}
	return 0, pubDate
}

var (
	itunesImageRegex = regexp.MustCompile(`(?i)<itunes:image[^>]+href=["']([^"']+)["']`)
	rssImageRegex    = regexp.MustCompile(`(?i)<image>[\s\S]*?<url>([^<]+)</url>`)
)

func extractChannelImageURL(c channelXML, data []byte) string {
	if u := strings.TrimSpace(c.ITunesImage.Href); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.ITunesImage.URL); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.ITunesImage2.Href); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.Image.URL); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.Image.Href); u != "" {
		return u
	}
	return scanRawXMLForImage(data)
}

func scanRawXMLForImage(data []byte) string {
	s := string(data)
	channelIdx := strings.Index(s, "<channel")
	if channelIdx == -1 {
		return ""
	}
	itemIdx := strings.Index(s, "<item")
	channelHeader := s[channelIdx:]
	if itemIdx > channelIdx {
		channelHeader = s[channelIdx:itemIdx]
	}
	if m := itunesImageRegex.FindStringSubmatch(channelHeader); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m := rssImageRegex.FindStringSubmatch(channelHeader); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// FeedDocument is a parsed RSS feed: its episodes plus the channel-level
// freshness markers used to decide whether the feed changed at all.
type FeedDocument struct {
	Title          string
	ImageURL       string
	LastBuildDate  string
	ChannelPubDate string
	Episodes       []backend.FeedEpisode
}

// LatestGUID identifies the newest episode in the feed. It is the last-resort
// change marker for feeds that serve neither HTTP validators nor a
// lastBuildDate, where the only way to tell whether anything is new is to look
// at the content itself.
func (d *FeedDocument) LatestGUID() string {
	if d == nil {
		return ""
	}
	var newest *backend.FeedEpisode
	for i := range d.Episodes {
		ep := &d.Episodes[i]
		if newest == nil || ep.PublishedAt > newest.PublishedAt {
			newest = ep
		}
	}
	if newest == nil {
		return ""
	}
	return episodeIdentity(*newest)
}

// episodeIdentity returns the most stable identifier available for an episode,
// preferring the GUID and falling back to the enclosure URL and then the title.
func episodeIdentity(ep backend.FeedEpisode) string {
	if g := strings.TrimSpace(ep.GUID); g != "" {
		return g
	}
	if ep.Enclosure != nil && strings.TrimSpace(ep.Enclosure.URL) != "" {
		return strings.TrimSpace(ep.Enclosure.URL)
	}
	if u := strings.TrimSpace(ep.EnclosureURL); u != "" {
		return u
	}
	return strings.ToLower(strings.TrimSpace(ep.Title))
}

func parseRSSFeed(data []byte) (*FeedDocument, error) {
	var rss rssXML
	if err := xml.Unmarshal(data, &rss); err != nil {
		return nil, err
	}

	doc := &FeedDocument{
		Title:          strings.TrimSpace(rss.Channel.Title),
		ImageURL:       extractChannelImageURL(rss.Channel, data),
		LastBuildDate:  strings.TrimSpace(rss.Channel.LastBuildDate),
		ChannelPubDate: strings.TrimSpace(rss.Channel.PubDate),
	}
	var episodes []backend.FeedEpisode
	for _, it := range rss.Channel.Items {
		if it.Enclosure == nil || strings.TrimSpace(it.Enclosure.URL) == "" {
			continue
		}

		guid := strings.TrimSpace(it.GUID.Value)
		if guid == "" {
			guid = strings.TrimSpace(it.ID)
		}
		if guid == "" && it.Enclosure != nil {
			guid = it.Enclosure.URL
		}

		pubMS, pubStr := parseFeedDate(it.PubDate)

		ep := backend.FeedEpisode{
			Title:            strings.TrimSpace(it.Title),
			DescriptionPlain: strings.TrimSpace(it.Description),
			PubDate:          pubStr,
			PublishedAt:      pubMS,
			GUID:             guid,
			Season:           strings.TrimSpace(it.Season),
			Episode:          strings.TrimSpace(it.Episode),
		}
		if it.Enclosure != nil && it.Enclosure.URL != "" {
			encURL := strings.TrimSpace(it.Enclosure.URL)
			if strings.HasPrefix(encURL, "http://") {
				encURL = "https://" + strings.TrimPrefix(encURL, "http://")
			}
			ep.EnclosureURL = encURL
			ep.Enclosure = &backend.FeedEnclosure{
				URL:  encURL,
				Type: strings.TrimSpace(it.Enclosure.Type),
			}
		}
		episodes = append(episodes, ep)
	}
	doc.Episodes = episodes
	return doc, nil
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
	client := opts.Client
	if client == nil {
		client = feedHTTPClient(opts.Timeout)
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultFeedFetchAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		res, retry, err := fetchFeedAttempt(client, feedURL, opts)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !retry || attempt == maxAttempts {
			return FeedFetchResult{}, err
		}
		feedSleepBackoff(attempt)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("failed to fetch feed")
	}
	return FeedFetchResult{}, lastErr
}

func fetchFeedAttempt(client *http.Client, feedURL string, opts FeedFetchOptions) (FeedFetchResult, bool, error) {
	req, err := http.NewRequest("GET", feedURL, nil)
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

	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return FeedFetchResult{
			ETag:         opts.ETag,
			LastModified: opts.LastModified,
			NotModified:  true,
		}, false, nil
	}
	if isTransientHTTPStatus(resp.StatusCode) {
		resp.Body.Close()
		return FeedFetchResult{}, true, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return FeedFetchResult{}, false, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}
	return readFeedResponse(resp)
}

func readFeedResponse(resp *http.Response) (FeedFetchResult, bool, error) {
	defer resp.Body.Close()
	res := FeedFetchResult{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedSize))
	if err != nil {
		return FeedFetchResult{}, true, err
	}
	doc, err := parseRSSFeed(body)
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
