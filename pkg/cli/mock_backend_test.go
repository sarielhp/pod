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

func (m *mockTestBackend) PodcastFeedEpisodes(_ string) ([]backend.FeedEpisode, error) {
	return m.feedEpisodes, m.feedEpisodesErr
}

func (m *mockTestBackend) ActiveDownloads(_ string) ([]backend.ActiveDownload, error) {
	return nil, nil
}

func (m *mockTestBackend) OpenRSSFeed(_, _ string) (string, error) {
	return "", nil
}

func (m *mockTestBackend) DownloadCover(_, _ string) error {
	return nil
}

func (m *mockTestBackend) CreatePodcast(_, _, _, _, _ string) (*backend.Podcast, error) {
	return nil, nil
}

func (m *mockTestBackend) DeletePodcast(_ string) error { return nil }

func (m *mockTestBackend) DeleteItem(_ string) error { return nil }

func (m *mockTestBackend) DownloadEpisodes(podcastID string, episodes []backend.FeedEpisode) error {
	m.downloadCalls++
	if m.downloadFn != nil {
		return m.downloadFn(podcastID, episodes)
	}
	return nil
}

func (m *mockTestBackend) DeletePodcastEpisode(_, _ string) error { return nil }

func (m *mockTestBackend) ResetPodcastDateCheck(_, _ string) error { return nil }

func (m *mockTestBackend) ResetPodcastDateCheckAPI(_ string) error { return nil }

func (m *mockTestBackend) SyncDuration(_ string, _ float64) error { return nil }

func (m *mockTestBackend) ApplyKeepPolicy(_, _ string, _ int, _ bool) (int, error) {
	return 0, nil
}

func (m *mockTestBackend) UpdatePodcastSettings(_ string, _, _ bool, _ int) error {
	return nil
}

func (m *mockTestBackend) TestConnection(progress.Reporter) (bool, error) { return true, nil }

func (m *mockTestBackend) Login() (string, error) { return "", nil }

func (m *mockTestBackend) Scan(_ backend.ScanOptions) (backend.ScanResult, error) {
	return backend.ScanResult{}, nil
}

func (m *mockTestBackend) Rescan(_ backend.RescanOptions) (backend.RescanResult, error) {
	return backend.RescanResult{}, nil
}

func (m *mockTestBackend) ExportOPML(_ backend.OPMLExportOptions) ([]byte, error) {
	return nil, nil
}

func (m *mockTestBackend) ImportOPML(_ []byte, _ backend.OPMLImportOptions) (backend.OPMLImportResult, error) {
	return backend.OPMLImportResult{}, nil
}

func (m *mockTestBackend) FetchPodcastFeeds() ([]backend.OPMLFeed, error) {
	return nil, nil
}

func (m *mockTestBackend) WaitForActiveDownloads(_ []backend.Podcast, _ time.Duration) error {
	return nil
}
