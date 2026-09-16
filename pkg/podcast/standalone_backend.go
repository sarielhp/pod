package podcast

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/pipeline"
	"pod/pkg/progress"
	"pod/pkg/util"
)

type StandaloneBackend struct {
	cfg        backend.Config
	store      *SubscriptionStore
	downloader *Downloader
}

func init() {
	backend.Register("standalone", func(cfg backend.Config) (backend.Backend, error) {
		return newStandaloneBackend(cfg), nil
	})
	backend.Register("pod", func(cfg backend.Config) (backend.Backend, error) {
		return newStandaloneBackend(cfg), nil
	})
	backend.Register("abs", func(cfg backend.Config) (backend.Backend, error) {
		return newStandaloneBackend(cfg), nil
	})
	backend.Register("local", func(cfg backend.Config) (backend.Backend, error) {
		return newStandaloneBackend(cfg), nil
	})
}

func resolveStorePath(cfg backend.Config) string {
	if cfg.SubscriptionsFile != "" {
		return cfg.SubscriptionsFile
	}
	if cfg.PodcastsDir != "" {
		subFile := filepath.Join(cfg.PodcastsDir, "podcasts.json")
		if _, err := os.Stat(subFile); err == nil {
			return subFile
		}
		if strings.HasPrefix(cfg.PodcastsDir, os.TempDir()) {
			return subFile
		}
	}
	return ""
}

func newStandaloneBackend(cfg backend.Config) *StandaloneBackend {
	storePath := resolveStorePath(cfg)
	store, _ := NewSubscriptionStore(storePath)
	return &StandaloneBackend{
		cfg:        cfg,
		store:      store,
		downloader: NewDownloader(),
	}
}

func (b *StandaloneBackend) Name() string {
	return "standalone"
}

func (b *StandaloneBackend) basePodcastsDir() string {
	if b.cfg.PodcastsDir != "" {
		return b.cfg.PodcastsDir
	}
	return "/media/podcasts/clean/"
}

func (b *StandaloneBackend) getStore() (*SubscriptionStore, error) {
	if b.store != nil {
		return b.store, nil
	}
	storePath := resolveStorePath(b.cfg)
	s, err := NewSubscriptionStore(storePath)
	if err != nil {
		return nil, err
	}
	b.store = s
	return s, nil
}

func (b *StandaloneBackend) Libraries() ([]backend.Library, error) {
	return b.PodcastLibraries()
}

func (b *StandaloneBackend) PodcastLibraries() ([]backend.Library, error) {
	dir := b.basePodcastsDir()
	return []backend.Library{{
		ID:        "default",
		Name:      "Podcasts",
		MediaType: "podcast",
		Folders: []backend.LibraryFolder{{
			ID:        "default",
			FullPath:  dir,
			LibraryID: "default",
		}},
	}}, nil
}

func (b *StandaloneBackend) Podcasts() ([]backend.Podcast, error) {
	store, err := b.getStore()
	if err != nil {
		return nil, fmt.Errorf("load subscriptions store: %w", err)
	}
	subs := store.List()
	baseDir := b.basePodcastsDir()

	res := make([]backend.Podcast, 0, len(subs))
	for _, sub := range subs {
		p := subToBackendPodcast(sub, baseDir)
		res = append(res, p)
	}

	sort.Slice(res, func(i, j int) bool {
		return strings.ToLower(res[i].Media.Metadata.Title) < strings.ToLower(res[j].Media.Metadata.Title)
	})
	return res, nil
}

func subToBackendPodcast(sub Subscription, podcastsDir string) backend.Podcast {
	podDir := resolvePodcastDirForSub(sub, podcastsDir)
	localEps := CollectLocalEpisodes(podDir, nil)
	eps := make([]backend.Episode, 0, len(localEps))
	for _, le := range localEps {
		eps = append(eps, backend.Episode{
			ID:          le.GUID,
			Title:       le.Title,
			GUID:        le.GUID,
			PubDate:     le.PubDate,
			PublishedAt: le.PublishedAt,
			Duration:    le.DurationSec,
			Size:        le.SizeBytes,
			AudioFile: &backend.PodcastAudioFile{
				Duration: le.DurationSec,
				Metadata: &backend.AudioFileMetadata{
					Filename: le.Filename,
					Path:     le.Path,
					Size:     le.SizeBytes,
				},
			},
		})
	}

	folder := sub.Folder
	if folder == "" {
		folder = filepath.Base(podDir)
	}

	return backend.Podcast{
		ID:        sub.ID,
		Path:      podDir,
		RelPath:   folder,
		MediaType: "podcast",
		Media: backend.PodcastMedia{
			ID: sub.ID,
			Metadata: backend.PodcastMetadata{
				Title:    sub.Title,
				FeedURL:  sub.FeedURL,
				ImageURL: sub.ImageURL,
			},
			Episodes:  eps,
			CoverPath: findCoverRelativePath(podDir, ""),
		},
	}
}

func (b *StandaloneBackend) GetPodcast(id string) (*backend.Podcast, error) {
	pods, err := b.Podcasts()
	if err != nil {
		return nil, err
	}
	matched, err := MatchBackendPodcasts(pods, id)
	if err != nil {
		return nil, err
	}
	return matched, nil
}

func (b *StandaloneBackend) PodcastFeedEpisodes(feedURL string) ([]backend.FeedEpisode, error) {
	feedEps, _, _, _, err := FetchFeedDirect(feedURL, "", "")
	return feedEps, err
}

func (b *StandaloneBackend) ActiveDownloads(podcastID string) ([]backend.ActiveDownload, error) {
	return nil, nil
}

func (b *StandaloneBackend) OpenRSSFeed(podcastID, baseURL string) (string, error) {
	p, err := b.GetPodcast(podcastID)
	if err != nil {
		return "", err
	}
	feedPath := filepath.Join(p.Path, "feed.xml")
	if _, err := os.Stat(feedPath); err != nil {
		return "", fmt.Errorf("feed.xml not found for %s", p.Media.Metadata.Title)
	}
	return feedPath, nil
}

func (b *StandaloneBackend) DownloadCover(podcastID, destPath string) error {
	p, err := b.GetPodcast(podcastID)
	if err != nil {
		return err
	}
	sub := Subscription{
		ID:       p.ID,
		Title:    p.Media.Metadata.Title,
		FeedURL:  p.Media.Metadata.FeedURL,
		ImageURL: p.Media.Metadata.ImageURL,
	}
	cover := ensurePodcastCover(filepath.Dir(destPath), &sub)
	if cover == "" {
		return fmt.Errorf("no cover available for %s", p.Media.Metadata.Title)
	}
	return nil
}

func (b *StandaloneBackend) CreatePodcast(libraryID, folderID, path, title, feedURL string) (*backend.Podcast, error) {
	store, err := b.getStore()
	if err != nil {
		return nil, err
	}
	sub := Subscription{
		Title:   title,
		FeedURL: feedURL,
	}
	if err := store.Add(sub); err != nil {
		return nil, err
	}
	if err := store.Save(); err != nil {
		return nil, err
	}
	added := store.Get(feedURL)
	if added == nil {
		added = store.Get(title)
	}
	if added == nil {
		return nil, errors.New("subscription added but not found")
	}
	p := subToBackendPodcast(*added, b.basePodcastsDir())
	return &p, nil
}

func (b *StandaloneBackend) DeletePodcast(id string) error {
	store, err := b.getStore()
	if err != nil {
		return err
	}
	removed, err := store.Remove(id)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("podcast %q not found", id)
	}
	return store.Save()
}

func (b *StandaloneBackend) DeleteItem(id string) error {
	return b.DeletePodcast(id)
}

func (b *StandaloneBackend) DownloadEpisodes(podcastID string, episodes []backend.FeedEpisode) error {
	p, err := b.GetPodcast(podcastID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.Path, 0755); err != nil {
		return err
	}
	for _, ep := range episodes {
		encURL := ep.EnclosureURL
		if ep.Enclosure != nil && ep.Enclosure.URL != "" {
			encURL = ep.Enclosure.URL
		}
		if encURL == "" {
			continue
		}
		pubMs := GetPubMS(ep)
		var pubTime time.Time
		if pubMs > 0 {
			pubTime = time.UnixMilli(pubMs).UTC()
		}
		fn := FormatEpisodeFilename(pubTime, ep.Episode, ep.Title)
		destPath := filepath.Join(p.Path, fn)
		if err := b.downloader.DownloadEpisode(context.Background(), encURL, destPath, b.cfg.Progress); err != nil {
			return fmt.Errorf("download %s: %w", ep.Title, err)
		}
		initDownloadedEpisodeStatus(destPath, fn, ep, pubTime)
	}
	sub := Subscription{
		ID:       p.ID,
		Title:    p.Media.Metadata.Title,
		FeedURL:  p.Media.Metadata.FeedURL,
		Folder:   p.RelPath,
		ImageURL: p.Media.Metadata.ImageURL,
	}
	_ = PublishPodcast(p.Path, sub, b.cfg.ServerBaseURL, nil)
	return nil
}

func initDownloadedEpisodeStatus(audioPath, filename string, ep backend.FeedEpisode, pubTime time.Time) {
	st := pipeline.GetOrCreateEpisodeStatus(audioPath)
	if st != nil {
		if !pubTime.IsZero() {
			st.PublishedAt = pubTime.UTC().Format(time.RFC3339)
			st.PublicationSource = "feed"
		}
		if st.MediaFile == "" {
			st.MediaFile = filename
		}
		_ = pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(audioPath), st)
	}
}

func (b *StandaloneBackend) DeletePodcastEpisode(podcastID, episodeID string) error {
	p, err := b.GetPodcast(podcastID)
	if err != nil {
		return err
	}
	for _, ep := range p.Media.Episodes {
		if ep.ID == episodeID || ep.GUID == episodeID || strings.EqualFold(ep.Title, episodeID) {
			if ep.AudioFile != nil && ep.AudioFile.Metadata != nil && ep.AudioFile.Metadata.Path != "" {
				_ = os.Remove(ep.AudioFile.Metadata.Path)
				sub := Subscription{
					ID:       p.ID,
					Title:    p.Media.Metadata.Title,
					FeedURL:  p.Media.Metadata.FeedURL,
					Folder:   p.RelPath,
					ImageURL: p.Media.Metadata.ImageURL,
				}
				_ = PublishPodcast(p.Path, sub, b.cfg.ServerBaseURL, nil)
				return nil
			}
		}
	}
	return fmt.Errorf("episode %q not found in %s", episodeID, p.Media.Metadata.Title)
}

func (b *StandaloneBackend) ResetPodcastDateCheck(itemID, title string) error {
	return nil
}

func (b *StandaloneBackend) ResetPodcastDateCheckAPI(itemID string) error {
	return nil
}

func (b *StandaloneBackend) SyncDuration(filePath string, duration float64) error {
	return nil
}

func (b *StandaloneBackend) ApplyKeepPolicy(podcastID, podcastTitle string, keep int, dryRun bool) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	target := podcastID
	if target == "" {
		target = podcastTitle
	}
	p, err := b.GetPodcast(target)
	if err != nil {
		return 0, err
	}
	files := util.FindMP3Files(p.Path)
	if len(files) <= keep {
		return 0, nil
	}
	sort.Slice(files, func(i, j int) bool {
		fi1, _ := os.Stat(files[i])
		fi2, _ := os.Stat(files[j])
		if fi1 == nil || fi2 == nil {
			return files[i] < files[j]
		}
		return fi1.ModTime().Before(fi2.ModTime())
	})
	toDelete := files[:len(files)-keep]
	deleted := 0
	for _, f := range toDelete {
		if !dryRun {
			_ = os.Remove(f)
		}
		deleted++
	}
	return deleted, nil
}

func (b *StandaloneBackend) UpdatePodcastSettings(podcastID string, autoDownload, autoCleanup bool, autoCleanupDays int) error {
	store, err := b.getStore()
	if err != nil {
		return err
	}
	sub := store.Get(podcastID)
	if sub == nil {
		return fmt.Errorf("subscription %q not found", podcastID)
	}
	if !autoDownload {
		sub.DownloadPolicy = "none"
	} else if sub.DownloadPolicy == "" || sub.DownloadPolicy == "none" {
		sub.DownloadPolicy = "latest"
	}
	_ = store.Add(*sub)
	return store.Save()
}

func (b *StandaloneBackend) TestConnection(rep progress.Reporter) (bool, error) {
	return true, nil
}

func (b *StandaloneBackend) Login() (string, error) {
	return "local", nil
}

func (b *StandaloneBackend) Scan(opts backend.ScanOptions) (backend.ScanResult, error) {
	return backend.ScanResult{}, nil
}

func (b *StandaloneBackend) Rescan(opts backend.RescanOptions) (backend.RescanResult, error) {
	return backend.RescanResult{}, nil
}

func (b *StandaloneBackend) ExportOPML(opts backend.OPMLExportOptions) ([]byte, error) {
	store, err := b.getStore()
	if err != nil {
		return nil, err
	}
	return store.ExportToOPML("")
}

func (b *StandaloneBackend) ImportOPML(data []byte, opts backend.OPMLImportOptions) (backend.OPMLImportResult, error) {
	store, err := b.getStore()
	if err != nil {
		return backend.OPMLImportResult{}, err
	}
	n, err := store.ImportFromOPML(data)
	return backend.OPMLImportResult{Subscribed: n, TotalFeeds: n}, err
}

func (b *StandaloneBackend) FetchPodcastFeeds() ([]backend.OPMLFeed, error) {
	store, err := b.getStore()
	if err != nil {
		return nil, err
	}
	subs := store.List()
	feeds := make([]backend.OPMLFeed, 0, len(subs))
	for _, sub := range subs {
		feeds = append(feeds, backend.OPMLFeed{
			Title:    sub.Title,
			URL:      sub.FeedURL,
			ImageURL: sub.ImageURL,
		})
	}
	return feeds, nil
}

func (b *StandaloneBackend) WaitForActiveDownloads(podcasts []backend.Podcast, timeout time.Duration) error {
	return nil
}
