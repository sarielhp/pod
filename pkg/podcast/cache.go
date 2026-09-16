package podcast

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/pipeline"
	"pod/pkg/util"
)

// EpisodeFile is what every representation of a downloaded episode agrees on:
// where the audio is, what it is called, when it was published, how long it
// runs and how large it is. Types that describe an episode embed it rather
// than restating it, so these six fields and their JSON names are defined once.
//
// It is deliberately not a universal Episode type. The representations that
// embed it differ in what they add — cache bookkeeping, feed metadata, display
// state — and flattening those into one struct would produce a thirty-field
// record where most fields are meaningless for most callers.
type EpisodeFile struct {
	Path        string  `json:"path"`
	Filename    string  `json:"filename"`
	Title       string  `json:"title"`
	PublishedAt int64   `json:"published_at"`
	DurationSec float64 `json:"duration"`
	SizeBytes   int64   `json:"file_size"`
}

type CachedEpisodeSummary struct {
	// ID is only ever read, never written: it carries the episode short ID
	// that older versions of pod stored here, and episodeShortID still falls
	// back to it when resolving an episode from a cache written back then.
	ID string `json:"id,omitempty"`

	EpisodeFile

	Season        string `json:"season,omitempty"`
	Episode       string `json:"episode,omitempty"`
	HasAdsRemoved bool   `json:"has_ads_removed"`
	HasTranscript bool   `json:"has_transcript,omitempty"`
}

type CachedPodcastIndex struct {
	PodcastName string                 `json:"podcast_name"`
	PodcastDir  string                 `json:"podcast_dir"`
	ABSItemID   string                 `json:"abs_item_id,omitempty"`
	Author      string                 `json:"author,omitempty"`
	Description string                 `json:"description,omitempty"`
	FeedURL     string                 `json:"feed_url,omitempty"`
	CoverPath   string                 `json:"cover_path,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Episodes    []CachedEpisodeSummary `json:"episodes"`
}

type CachedEpisodeDetails struct {
	Path        string           `json:"path"`
	Filename    string           `json:"filename"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Subtitle    string           `json:"subtitle,omitempty"`
	EpisodeType string           `json:"episode_type,omitempty"`
	Genres      []string         `json:"genres,omitempty"`
	Author      string           `json:"author,omitempty"`
	FeedURL     string           `json:"feed_url,omitempty"`
	RawABS      *backend.Episode `json:"raw_abs,omitempty"`
}

func CacheBaseDir() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "cache")
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	podCacheDir := filepath.Join(cacheHome, "pod", "podcasts")
	legacyDir := filepath.Join(cacheHome, "abs", "podcasts")
	if _, err := os.Stat(podCacheDir); err == nil {
		return podCacheDir
	}
	if _, err := os.Stat(legacyDir); err == nil {
		return legacyDir
	}
	_ = os.MkdirAll(podCacheDir, 0755)
	return podCacheDir
}

func sanitizeDirName(dirPath string) string {
	clean := filepath.Clean(dirPath)
	base := filepath.Base(clean)
	h := sha256.Sum256([]byte(clean))
	hashPrefix := hex.EncodeToString(h[:4])
	safeBase := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, base)
	return safeBase + "_" + hashPrefix
}

func CacheDirForPodcast(podcastDir string) string {
	absDir, err := filepath.Abs(podcastDir)
	if err != nil {
		absDir = podcastDir
	}
	name := sanitizeDirName(absDir)
	dir := filepath.Join(CacheBaseDir(), name)
	_ = os.MkdirAll(dir, 0755)
	_ = os.MkdirAll(filepath.Join(dir, "details"), 0755)
	return dir
}

func LoadPodcastCache(podcastDir string) (*CachedPodcastIndex, error) {
	cDir := CacheDirForPodcast(podcastDir)
	indexPath := filepath.Join(cDir, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	var index CachedPodcastIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

func SavePodcastCache(podcastDir string, index *CachedPodcastIndex) error {
	cDir := CacheDirForPodcast(podcastDir)
	indexPath := filepath.Join(cDir, "index.json")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(indexPath, append(data, '\n'), 0644)
}

func detailFileName(filename string) string {
	h := sha256.Sum256([]byte(filename))
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, filename)
	if len(safe) > 32 {
		safe = safe[:32]
	}
	return safe + "_" + hex.EncodeToString(h[:4]) + ".json"
}

func LoadEpisodeDetails(podcastDir, filename string) (*CachedEpisodeDetails, error) {
	cDir := CacheDirForPodcast(podcastDir)
	detPath := filepath.Join(cDir, "details", detailFileName(filename))
	data, err := os.ReadFile(detPath)
	if err != nil {
		return nil, err
	}
	var det CachedEpisodeDetails
	if err := json.Unmarshal(data, &det); err != nil {
		return nil, err
	}
	return &det, nil
}

func SaveEpisodeDetails(podcastDir, filename string, details *CachedEpisodeDetails) error {
	cDir := CacheDirForPodcast(podcastDir)
	detPath := filepath.Join(cDir, "details", detailFileName(filename))
	data, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(detPath, append(data, '\n'), 0644)
}

func ResetCache() error {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			cacheHome = filepath.Join(os.TempDir(), "cache")
		} else {
			cacheHome = filepath.Join(home, ".cache")
		}
	}
	_ = os.RemoveAll(filepath.Join(cacheHome, "abs"))
	return os.RemoveAll(filepath.Join(cacheHome, "pod"))
}

func CacheStats() (dir string, entries int, bytes int64) {
	dir = CacheBaseDir()
	items, err := os.ReadDir(dir)
	if err != nil {
		return dir, 0, 0
	}
	entries = len(items)
	for _, it := range items {
		p := filepath.Join(dir, it.Name())
		_ = filepath.Walk(p, func(_ string, fi os.FileInfo, err error) error {
			if err == nil && fi != nil && !fi.IsDir() {
				bytes += fi.Size()
			}
			return nil
		})
	}
	return dir, entries, bytes
}

var (
	feedXMLMu    util.SyncMutex
	feedXMLCache = make(map[string]map[string]time.Time)
)

func ParseAnyPublicationTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time string")
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
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
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil && !t.IsZero() {
			return t.UTC(), nil
		}
	}
	norm := normalizeFeedTimezone(s)
	for _, l := range layouts {
		if t, err := time.Parse(l, norm); err == nil && !t.IsZero() {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unknown time format: %s", s)
}

type localFeedItemXML struct {
	Title     string `xml:"title"`
	PubDate   string `xml:"pubDate"`
	Enclosure struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
}

type localFeedChannelXML struct {
	Items []localFeedItemXML `xml:"item"`
}

type localFeedRSSXML struct {
	Channel localFeedChannelXML `xml:"channel"`
}

func parseFeedXMLDates(data []byte) map[string]time.Time {
	var rss localFeedRSSXML
	if err := xml.Unmarshal(data, &rss); err != nil || len(rss.Channel.Items) == 0 {
		return nil
	}
	m := make(map[string]time.Time, len(rss.Channel.Items)*4)
	for _, it := range rss.Channel.Items {
		pubMs, _ := parseFeedDate(it.PubDate)
		if pubMs <= 0 {
			continue
		}
		t := time.UnixMilli(pubMs)
		title := strings.TrimSpace(it.Title)
		titleKey := strings.ToLower(title)
		if titleKey != "" {
			m[titleKey] = t
			m[strings.ToLower(SanitizeTitle(title))] = t
			fn := strings.ToLower(FormatEpisodeFilename(t, "", title))
			m[fn] = t
			m[strings.TrimSuffix(fn, ".mp3")] = t
		}
		if it.Enclosure.URL != "" {
			uBase := strings.ToLower(strings.TrimSpace(filepath.Base(it.Enclosure.URL)))
			if uBase != "" {
				m[uBase] = t
				m[util.StripExt(uBase)] = t
			}
		}
	}
	return m
}

func lookupFeedXMLPublicationTime(dir, filePath string) time.Time {
	if dir == "" {
		return time.Time{}
	}
	feedPath := filepath.Join(dir, "feed.xml")
	feedXMLMu.Lock()
	dates, cached := feedXMLCache[feedPath]
	feedXMLMu.Unlock()

	if !cached {
		data, err := os.ReadFile(feedPath)
		if err != nil {
			feedXMLMu.Lock()
			feedXMLCache[feedPath] = nil
			feedXMLMu.Unlock()
			return time.Time{}
		}
		dates = parseFeedXMLDates(data)
		feedXMLMu.Lock()
		feedXMLCache[feedPath] = dates
		feedXMLMu.Unlock()
	}
	if dates == nil {
		return time.Time{}
	}

	baseName := filepath.Base(filePath)
	cleanStem := strings.ToLower(strings.TrimSpace(util.StripExt(baseName)))
	cleanTitle := strings.ToLower(strings.TrimSpace(EpisodeTitleFromPath(filePath)))
	strippedStem := strings.ToLower(StripEpisodeFilenamePrefix(cleanStem))

	if t, ok := dates[cleanStem]; ok && !t.IsZero() {
		return t
	}
	if t, ok := dates[cleanTitle]; ok && !t.IsZero() {
		return t
	}
	if t, ok := dates[strings.ToLower(SanitizeTitle(cleanTitle))]; ok && !t.IsZero() {
		return t
	}
	if t, ok := dates[strippedStem]; ok && !t.IsZero() {
		return t
	}
	if t, ok := dates[strings.ToLower(SanitizeTitle(strippedStem))]; ok && !t.IsZero() {
		return t
	}
	return time.Time{}
}

func matchCachedEpisodeDate(ep CachedEpisodeSummary, dir, filePath, absolute string) time.Time {
	if ep.PublishedAt <= 0 {
		return time.Time{}
	}
	path := ep.Path
	if path == "" {
		path = ep.Filename
	}
	baseName := filepath.Base(filePath)
	cleanBase := strings.ToLower(util.StripExt(baseName))
	matched := publicationAudioPath(dir, path) == absolute ||
		strings.EqualFold(ep.Filename, baseName) ||
		strings.EqualFold(util.StripExt(ep.Filename), cleanBase) ||
		strings.EqualFold(ep.Title, cleanBase)
	if matched {
		return time.UnixMilli(ep.PublishedAt)
	}
	return time.Time{}
}

func GetEpisodePublicationTime(filePath string) time.Time {
	if date, ok := SourcePublicationTime(filePath); ok {
		return date
	}
	st, err := pipeline.LoadEpisodeStatus(pipeline.StatusPathFor(filePath))
	if err == nil && st != nil && (st.PublicationSource == "source" || st.PublicationSource == "feed") {
		if date, err := ParseAnyPublicationTime(st.PublishedAt); err == nil && !date.IsZero() {
			return date
		}
	}
	dir := DetectPodcastDirForAudio(filePath)
	if feedDate := lookupFeedXMLPublicationTime(dir, filePath); !feedDate.IsZero() {
		return feedDate
	}
	if cached, _ := LoadPodcastCache(dir); cached != nil {
		absolute, _ := filepath.Abs(filePath)
		for _, ep := range cached.Episodes {
			if t := matchCachedEpisodeDate(ep, dir, filePath, absolute); !t.IsZero() {
				return t
			}
		}
	}
	if date, ok := ParseDatePrefix(filePath); ok {
		return date
	}
	return time.Time{}
}
