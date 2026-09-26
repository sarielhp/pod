package cli

import (
	"time"

	"pod/pkg/backend"
	"pod/pkg/progress"
)

type mockTestBackend struct {
	name            string
	podcasts        []backend.Podcast
	podcastMap      map[string]*backend.Podcast
	feedEpisodes    []backend.FeedEpisode
	feedEpisodesErr error
	downloadFn      func(podcastID string, episodes []backend.FeedEpisode) error
	downloadCalls   int
}

func newMockTestBackend() *mockTestBackend {
	return &mockTestBackend{
		name:       "mock",
		podcastMap: make(map[string]*backend.Podcast),
	}
}

func (m *mockTestBackend) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockTestBackend) Libraries() ([]backend.Library, error) { return nil, nil }

func (m *mockTestBackend) PodcastLibraries() ([]backend.Library, error) { return nil, nil }

func (m *mockTestBackend) Podcasts() ([]backend.Podcast, error) {
	return m.podcasts, nil
}

func (m *mockTestBackend) GetPodcast(id string) (*backend.Podcast, error) {
	if p, ok := m.podcastMap[id]; ok {
		return p, nil
	}
	return nil, nil
}

func (m *mockTestBackend) PodcastFeedEpisodes(feedURL string) ([]backend.FeedEpisode, error) {
	return m.feedEpisodes, m.feedEpisodesErr
}

func (m *mockTestBackend) ActiveDownloads(podcastID string) ([]backend.ActiveDownload, error) {
	return nil, nil
}

func (m *mockTestBackend) OpenRSSFeed(podcastID, baseURL string) (string, error) {
	return "", nil
}

func (m *mockTestBackend) DownloadCover(podcastID, destPath string) error {
	return nil
}

func (m *mockTestBackend) CreatePodcast(libraryID, folderID, path, title, feedURL string) (*backend.Podcast, error) {
	return nil, nil
}

func (m *mockTestBackend) DeletePodcast(id string) error { return nil }

func (m *mockTestBackend) DeleteItem(id string) error { return nil }

func (m *mockTestBackend) DownloadEpisodes(podcastID string, episodes []backend.FeedEpisode) error {
	m.downloadCalls++
	if m.downloadFn != nil {
		return m.downloadFn(podcastID, episodes)
	}
	return nil
}

func (m *mockTestBackend) DeletePodcastEpisode(podcastID, episodeID string) error { return nil }

func (m *mockTestBackend) ResetPodcastDateCheck(itemID, title string) error { return nil }

func (m *mockTestBackend) ResetPodcastDateCheckAPI(itemID string) error { return nil }

func (m *mockTestBackend) SyncDuration(filePath string, duration float64) error { return nil }

func (m *mockTestBackend) ApplyKeepPolicy(podcastID, podcastTitle string, keep int, dryRun bool) (int, error) {
	return 0, nil
}

func (m *mockTestBackend) UpdatePodcastSettings(podcastID string, autoDownload, autoCleanup bool, autoCleanupDays int) error {
	return nil
}

func (m *mockTestBackend) TestConnection(progress.Reporter) (bool, error) { return true, nil }

func (m *mockTestBackend) Login() (string, error) { return "", nil }

func (m *mockTestBackend) Scan(opts backend.ScanOptions) (backend.ScanResult, error) {
	return backend.ScanResult{}, nil
}

func (m *mockTestBackend) Rescan(opts backend.RescanOptions) (backend.RescanResult, error) {
	return backend.RescanResult{}, nil
}

func (m *mockTestBackend) ExportOPML(opts backend.OPMLExportOptions) ([]byte, error) {
	return nil, nil
}

func (m *mockTestBackend) ImportOPML(data []byte, opts backend.OPMLImportOptions) (backend.OPMLImportResult, error) {
	return backend.OPMLImportResult{}, nil
}

func (m *mockTestBackend) FetchPodcastFeeds() ([]backend.OPMLFeed, error) {
	return nil, nil
}

func (m *mockTestBackend) WaitForActiveDownloads(podcasts []backend.Podcast, timeout time.Duration) error {
	return nil
}
