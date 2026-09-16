package podcast

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/util"
)

type Subscription struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	FeedURL        string `json:"feed_url"`
	Folder         string `json:"folder,omitempty"`
	Disabled       bool   `json:"disabled,omitempty"`
	DownloadPolicy string `json:"download_policy,omitempty"`
	DownloadK      int    `json:"download_k,omitempty"`
	AdRemoval      string `json:"ad_removal,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	CreatedAt      int64  `json:"created_at,omitempty"`
}

type SubscriptionsFile struct {
	Version       int            `json:"version"`
	UpdatedAt     int64          `json:"updated_at"`
	Subscriptions []Subscription `json:"subscriptions"`
}

type SubscriptionStore struct {
	mu       util.RWMutex
	filePath string
	items    []Subscription
}

func NewSubscriptionStore(filePath string) (*SubscriptionStore, error) {
	if filePath == "" {
		filePath = config.SubscriptionsFilePath(nil)
	}
	s := &SubscriptionStore{
		filePath: filePath,
	}
	if err := s.Load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *SubscriptionStore) FilePath() string {
	return s.filePath
}

func (s *SubscriptionStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var sf SubscriptionsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return fmt.Errorf("parse subscriptions file %s: %w", s.filePath, err)
	}
	s.items = sf.Subscriptions
	return nil
}

func (s *SubscriptionStore) Save() error {
	s.mu.RLock()
	sf := SubscriptionsFile{
		Version:       1,
		UpdatedAt:     time.Now().Unix(),
		Subscriptions: make([]Subscription, len(s.items)),
	}
	copy(sf.Subscriptions, s.items)
	s.mu.RUnlock()

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return err
	}
	return util.WriteFileAtomic(s.filePath, append(data, '\n'), 0644)
}

func (s *SubscriptionStore) List() []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]Subscription, len(s.items))
	copy(res, s.items)
	return res
}

func (s *SubscriptionStore) Get(query string) *Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return nil
	}
	for i := range s.items {
		sub := &s.items[i]
		if strings.ToLower(sub.ID) == q || strings.ToLower(sub.Title) == q || strings.ToLower(sub.Folder) == q {
			res := *sub
			return &res
		}
	}
	for i := range s.items {
		sub := &s.items[i]
		if strings.Contains(strings.ToLower(sub.Title), q) {
			res := *sub
			return &res
		}
	}
	return nil
}

func (s *SubscriptionStore) Add(sub Subscription) error {
	sub.Title = strings.TrimSpace(sub.Title)
	sub.FeedURL = strings.TrimSpace(sub.FeedURL)
	if sub.FeedURL == "" {
		return errors.New("subscription feed URL cannot be empty")
	}
	if sub.Title == "" {
		sub.Title = "Untitled Podcast"
	}
	if sub.ID == "" {
		sub.ID = GeneratePodcastShortID(sub.Title)
	}
	if sub.Folder == "" {
		sub.Folder = SanitizeTitle(sub.Title)
	}
	if sub.CreatedAt == 0 {
		sub.CreatedAt = time.Now().Unix()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.items {
		if existing.FeedURL == sub.FeedURL || existing.ID == sub.ID {
			if sub.ImageURL == "" && existing.ImageURL != "" {
				sub.ImageURL = existing.ImageURL
			}
			s.items[i] = sub
			return nil
		}
	}
	s.items = append(s.items, sub)
	return nil
}

func (s *SubscriptionStore) Remove(query string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return false, nil
	}
	for i, sub := range s.items {
		if strings.ToLower(sub.ID) == q || strings.ToLower(sub.Title) == q || strings.ToLower(sub.Folder) == q {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (s *SubscriptionStore) ImportFromOPML(data []byte) (int, error) {
	feeds, err := backend.ParseOPMLXML(data)
	if err != nil {
		return 0, fmt.Errorf("parse OPML: %w", err)
	}
	count := 0
	for _, f := range feeds {
		sub := Subscription{
			Title:    f.Title,
			FeedURL:  f.URL,
			ImageURL: f.ImageURL,
		}
		if err := s.Add(sub); err == nil {
			count++
		}
	}
	if count > 0 {
		if err := s.Save(); err != nil {
			return count, err
		}
	}
	return count, nil
}

func (s *SubscriptionStore) ExportToOPML(serverBaseURL string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	feeds := make([]backend.OPMLFeed, 0, len(s.items))
	serverBaseURL = strings.TrimRight(strings.TrimSpace(serverBaseURL), "/")
	for _, it := range s.items {
		u := it.FeedURL
		if serverBaseURL != "" {
			folder := it.Folder
			if folder == "" {
				folder = SanitizeTitle(it.Title)
			}
			u = fmt.Sprintf("%s/%s/feed.xml", serverBaseURL, url.PathEscape(folder))
		}
		feeds = append(feeds, backend.OPMLFeed{
			Title:    it.Title,
			URL:      u,
			ImageURL: it.ImageURL,
		})
	}
	return backend.BuildOPMLXMLWithTitle(feeds, "Pod Podcast Subscriptions", "Pod Podcasts")
}

func (s *SubscriptionStore) ImportFromBackend(reader backend.PodcastReader) (int, error) {
	if reader == nil {
		return 0, errors.New("nil backend reader")
	}
	podcasts, err := reader.Podcasts()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, p := range podcasts {
		feedURL := p.Media.Metadata.FeedURL
		if feedURL == "" {
			continue
		}
		title := p.Media.Metadata.Title
		if title == "" {
			title = filepath.Base(p.Path)
		}
		folder := p.RelPath
		if folder == "" && p.Path != "" {
			folder = filepath.Base(p.Path)
		}
		if folder == "" || folder == "." {
			folder = SanitizeTitle(title)
		}
		sub := Subscription{
			ID:       p.ID,
			Title:    title,
			FeedURL:  feedURL,
			Folder:   folder,
			ImageURL: p.Media.Metadata.ImageURL,
		}
		if err := s.Add(sub); err == nil {
			count++
		}
	}
	if count > 0 {
		if err := s.Save(); err != nil {
			return count, err
		}
	}
	return count, nil
}

func resolvePodcastDirForSub(sub Subscription, podcastsDir string) string {
	if sub.Folder != "" {
		target := filepath.Join(podcastsDir, sub.Folder)
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			return target
		}
	}
	safeTitle := SanitizeTitle(sub.Title)
	target := filepath.Join(podcastsDir, safeTitle)
	return target
}

// SubscriptionMatches reports whether a query selects this subscription: an
// exact case-insensitive ID, or a case-insensitive substring of the title. An
// empty query selects everything, which is how "act on all subscriptions" is
// spelled at the call sites.
//
// This is deliberately narrower than matchesPodcastName, which also accepts a
// regular expression. The two should converge, but widening subscription
// matching to regexes changes which podcasts a command touches, so it is a
// behaviour change rather than a refactor.
func SubscriptionMatches(sub Subscription, query string) bool {
	if query == "" {
		return true
	}
	return strings.EqualFold(sub.ID, query) ||
		strings.Contains(strings.ToLower(sub.Title), strings.ToLower(query))
}
