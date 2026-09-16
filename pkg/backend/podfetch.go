package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"pod/pkg/progress"
)

type PodFetchBackend struct {
	Host        string
	User        string
	Pass        string
	Token       string
	APIKey      string
	DBPath      string
	PodcastsDir string
	MaxAttempts int
	RetryDelay  time.Duration
	httpClient  *http.Client
}

func init() {
	Register("podfetch", func(cfg Config) (Backend, error) {
		return newPodFetch(cfg), nil
	})
	Register("pod_fetch", func(cfg Config) (Backend, error) {
		return newPodFetch(cfg), nil
	})
}

func newPodFetch(cfg Config) *PodFetchBackend {
	host := strings.TrimRight(cfg.Host, "/")
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}

	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = cfg.Token
	}

	return &PodFetchBackend{
		Host:        host,
		User:        cfg.User,
		Pass:        cfg.Pass,
		Token:       cfg.Token,
		APIKey:      apiKey,
		DBPath:      cfg.DBPath,
		PodcastsDir: cfg.PodcastsDir,
		MaxAttempts: maxAttempts,
		RetryDelay:  cfg.RetryDelay,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *PodFetchBackend) Name() string {
	return "podfetch"
}

func (c *PodFetchBackend) getRetryDelay(attempt int) time.Duration {
	if c.RetryDelay > 0 {
		return c.RetryDelay
	}
	if attempt <= 5 {
		return 1 * time.Second
	}
	return 2 * time.Second
}

func (c *PodFetchBackend) Login() (string, error) {
	if c.APIKey != "" {
		return c.APIKey, nil
	}
	if c.Token != "" {
		return c.Token, nil
	}
	if c.User != "" && c.Pass != "" {
		return c.User, nil
	}
	if c.Host == "" && c.DBPath != "" {
		return "sqlite", nil
	}
	return "", nil
}

func (c *PodFetchBackend) TestConnection(rep progress.Reporter) (bool, error) {
	r := progress.Or(rep)
	if c.Host == "" && c.DBPath == "" {
		r.Warnf("ERROR: podfetch_url or podfetch_db_path is not configured.")
		return false, fmt.Errorf("podfetch_url or podfetch_db_path is not configured")
	}

	if c.Host != "" {
		r.Infof("Testing PodFetch server at: %s", c.Host)
		body, err := c.Request("/api/v1/podcasts", "GET", nil)
		if err != nil {
			body, err = c.Request("/api/v1/common/health", "GET", nil)
		}
		if err != nil {
			body, err = c.Request("/", "GET", nil)
		}
		if err != nil {
			r.Warnf("FAIL: Could not connect to PodFetch server: %v", err)
			return false, err
		}
		_ = body
		r.Infof("OK: PodFetch server is reachable.")
		return true, nil
	}

	if c.DBPath != "" {
		r.Infof("Testing PodFetch SQLite database at: %s", c.DBPath)
		_, err := fetchPodFetchPodcastsDB(c.DBPath)
		if err != nil {
			r.Warnf("FAIL: Could not query PodFetch database: %v", err)
			return false, err
		}
		r.Infof("OK: PodFetch SQLite database is valid and readable.")
		return true, nil
	}

	return true, nil
}

func (c *PodFetchBackend) Request(endpoint, method string, data interface{}) ([]byte, error) {
	if c.Host == "" {
		return nil, fmt.Errorf("host is not configured")
	}

	reqURL := fmt.Sprintf("%s%s", c.Host, endpoint)
	var jsonData []byte
	var err error
	if data != nil {
		jsonData, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	maxAttempts := c.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}

	var lastErr error
	var lastStatusCode int

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		retryDelay := c.getRetryDelay(attempt)
		req, err := buildPodFetchRequest(method, reqURL, jsonData, c.APIKey, c.Token, c.User, c.Pass)
		if err != nil {
			return nil, err
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts {
				time.Sleep(retryDelay)
				continue
			}
			return nil, err
		}

		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			lastErr = err
			if attempt < maxAttempts {
				time.Sleep(retryDelay)
				continue
			}
			return nil, err
		}

		shouldRetry, err := checkPodFetchStatus(res.StatusCode, attempt, maxAttempts, retryDelay)
		if err != nil {
			return nil, err
		}
		if shouldRetry {
			lastStatusCode = res.StatusCode
			continue
		}

		return body, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("HTTP Error %d", lastStatusCode)
}

func buildPodFetchRequest(method, reqURL string, jsonData []byte, apiKey, token, user, pass string) (*http.Request, error) {
	var req *http.Request
	var err error
	if jsonData != nil {
		req, err = http.NewRequest(method, reqURL, bytes.NewBuffer(jsonData))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	} else {
		req, err = http.NewRequest(method, reqURL, nil)
	}
	if err != nil {
		return nil, err
	}

	if user != "" && pass != "" {
		req.SetBasicAuth(user, pass)
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
		if user == "" || pass == "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		}
	} else if token != "" && (user == "" || pass == "") {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	return req, nil
}

func checkPodFetchStatus(statusCode int, attempt, maxAttempts int, retryDelay time.Duration) (bool, error) {
	if statusCode >= 500 || statusCode == 429 || statusCode == 408 {
		if attempt < maxAttempts {
			time.Sleep(retryDelay)
			return true, nil
		}
		return false, fmt.Errorf("HTTP Error %d", statusCode)
	}
	if statusCode < 200 || statusCode >= 300 {
		return false, fmt.Errorf("HTTP Error %d", statusCode)
	}
	return false, nil
}

func (c *PodFetchBackend) Libraries() ([]Library, error) {
	return []Library{
		{
			ID:        "podfetch-default",
			Name:      "PodFetch Library",
			MediaType: "podcast",
			Folders: []LibraryFolder{
				{
					ID:        "default",
					FullPath:  c.PodcastsDir,
					LibraryID: "podfetch-default",
				},
			},
		},
	}, nil
}

func (c *PodFetchBackend) PodcastLibraries() ([]Library, error) {
	return c.Libraries()
}

func (c *PodFetchBackend) CreatePodcast(libraryID, folderID, path, title, feedURL string) (*Podcast, error) {
	return nil, fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) DeletePodcast(id string) error {
	return fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) DeleteItem(id string) error {
	return fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) DownloadEpisodes(podcastID string, episodes []FeedEpisode) error {
	return fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) DeletePodcastEpisode(podcastID, episodeID string) error {
	return fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) ResetPodcastDateCheck(itemID, title string) error {
	return nil
}

func (c *PodFetchBackend) ResetPodcastDateCheckAPI(itemID string) error {
	return nil
}

func (c *PodFetchBackend) SyncDuration(filePath string, duration float64) error {
	return nil
}

func (c *PodFetchBackend) ApplyKeepPolicy(podcastID, podcastTitle string, keep int, dryRun bool) (int, error) {
	return 0, fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) UpdatePodcastSettings(podcastID string, autoDownload, autoCleanup bool, autoCleanupDays int) error {
	return fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) Scan(opts ScanOptions) (ScanResult, error) {
	return ScanResult{}, nil
}

func (c *PodFetchBackend) Rescan(opts RescanOptions) (RescanResult, error) {
	return RescanResult{}, nil
}

func (c *PodFetchBackend) ExportOPML(opts OPMLExportOptions) ([]byte, error) {
	return nil, fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) ImportOPML(data []byte, opts OPMLImportOptions) (OPMLImportResult, error) {
	return OPMLImportResult{}, fmt.Errorf("podfetch is an import-only backend")
}

func (c *PodFetchBackend) FetchPodcastFeeds() ([]OPMLFeed, error) {
	return nil, nil
}

func (c *PodFetchBackend) WaitForActiveDownloads(podcasts []Podcast, timeout time.Duration) error {
	return nil
}

func (c *PodFetchBackend) ActiveDownloads(podcastID string) ([]ActiveDownload, error) {
	return nil, nil
}

func (c *PodFetchBackend) OpenRSSFeed(podcastID, baseURL string) (string, error) {
	return "", nil
}

func (c *PodFetchBackend) DownloadCover(podcastID, destPath string) error {
	return nil
}

func (c *PodFetchBackend) PodcastFeedEpisodes(feedURL string) ([]FeedEpisode, error) {
	return nil, nil
}
