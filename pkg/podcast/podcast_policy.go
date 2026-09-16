package podcast

import (
	"fmt"
	"path/filepath"
	"strings"

	"pod/pkg/config"
)

// PolicyUpdate is a request to change a podcast's settings. An empty string or
// a zero number means "leave this alone", which is how a command line with
// only some flags set reaches the library without carrying the whole flag set.
type PolicyUpdate struct {
	Favorite       string
	AutoDownload   string
	DownloadPolicy string
	DownloadK      int
	AutoCleanup    string
	CleanupDays    int
	AdRemoval      string
}

// IsEmpty reports whether the update would change nothing, which is how a
// command distinguishes "show me the policy" from "set the policy".
func (u PolicyUpdate) IsEmpty() bool {
	return u.Favorite == "" && u.AutoDownload == "" && u.DownloadPolicy == "" &&
		u.DownloadK == 0 && u.AutoCleanup == "" && u.CleanupDays == 0 && u.AdRemoval == ""
}

// ParsePolicyBool is how the library reads a boolean written on a command
// line. Anything it does not recognise as true is false.
func ParsePolicyBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on", "enable", "enabled":
		return true
	}
	return false
}

// applyPolicyUpdate folds an update into a podcast's configuration.
//
// Setting the download policy to "none" also turns auto-download off, unless
// the same command said otherwise: asking for no downloads and leaving
// auto-download on would contradict itself. Setting a retention in days
// likewise turns auto-cleanup on.
func applyPolicyUpdate(cfg *config.PodcastConfig, u PolicyUpdate) {
	if u.Favorite != "" {
		cfg.SetFavorite(ParsePolicyBool(u.Favorite))
	}
	if u.AutoDownload != "" {
		cfg.SetAutoDownload(ParsePolicyBool(u.AutoDownload))
	}
	if u.DownloadPolicy != "" {
		cfg.DownloadPolicy = config.NormalizeDownloadPolicy(u.DownloadPolicy)
		if cfg.DownloadPolicy == config.DownloadPolicyNone && u.AutoDownload == "" {
			autoDl := false
			cfg.AutoDownload = &autoDl
		}
	}
	if u.DownloadK > 0 {
		cfg.DownloadK = u.DownloadK
	}
	if u.AutoCleanup != "" {
		cfg.SetAutoCleanup(ParsePolicyBool(u.AutoCleanup))
	}
	if u.CleanupDays > 0 {
		cfg.AutoCleanupDays = u.CleanupDays
		autoCl := true
		cfg.AutoCleanup = &autoCl
	}
	if u.AdRemoval != "" {
		cfg.AdRemoval = config.NormalizeAdRemovalMode(u.AdRemoval)
	}
}

// PolicyState is one podcast's effective settings, flattened for display.
type PolicyState struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Favorite        bool   `json:"favorite"`
	AutoDownload    bool   `json:"auto_download"`
	DownloadPolicy  string `json:"download_policy"`
	DownloadK       int    `json:"download_k"`
	AutoCleanup     bool   `json:"auto_cleanup"`
	AutoCleanupDays int    `json:"auto_cleanup_days"`
	AdRemoval       string `json:"ad_removal"`
}

// PolicyStateOf flattens a podcast's configuration for display.
func PolicyStateOf(id, title string, cfg config.PodcastConfig) PolicyState {
	return PolicyState{
		ID:              id,
		Title:           title,
		Favorite:        cfg.Favorite,
		AutoDownload:    cfg.IsAutoDownloadEnabled(),
		DownloadPolicy:  cfg.DownloadPolicy,
		DownloadK:       cfg.DownloadK,
		AutoCleanup:     cfg.IsAutoCleanupEnabled(),
		AutoCleanupDays: cfg.AutoCleanupDays,
		AdRemoval:       cfg.AdRemoval,
	}
}

// GroupPolicies reads the stored settings of every podcast in a group, without
// filling in any defaults: this is what each podcast has configured, not what
// it would resolve to at download time.
func GroupPolicies(entries []PodcastDirEntry) []PolicyState {
	states := make([]PolicyState, 0, len(entries))
	for _, e := range entries {
		states = append(states, PolicyStateOf(e.ShortID, e.Title,
			config.LoadPodcastConfig(e.Dir, config.PodcastConfig{})))
	}
	return states
}

// BackendSync reports what happened when a policy change was pushed to the
// backend. Backend is empty when none is configured.
type BackendSync struct {
	Backend string
	Err     error
}

// SyncPolicy pushes a podcast's download and cleanup settings to the backend.
func (l *Library) SyncPolicy(dir, uuid, shortID string, cfg config.PodcastConfig) BackendSync {
	if l.backend == nil {
		return BackendSync{}
	}
	target := uuid
	if target == "" {
		target = shortID
	}
	if target == "" {
		target = filepath.Base(dir)
	}
	err := l.backend.UpdatePodcastSettings(target,
		cfg.IsAutoDownloadEnabled(), cfg.IsAutoCleanupEnabled(), cfg.AutoCleanupDays)
	return BackendSync{Backend: l.backend.Name(), Err: err}
}

// BackendName returns the connected backend's name, or "" when none is
// configured.
func (l *Library) BackendName() string {
	if l.backend == nil {
		return ""
	}
	return l.backend.Name()
}

// SetPodcastPolicy applies an update to one podcast and saves it, returning
// the resulting configuration and the outcome of pushing it to the backend.
func (l *Library) SetPodcastPolicy(dir, uuid, shortID string, cfg config.PodcastConfig, u PolicyUpdate) (config.PodcastConfig, BackendSync, error) {
	applyPolicyUpdate(&cfg, u)
	if err := config.SavePodcastConfig(dir, cfg); err != nil {
		return cfg, BackendSync{}, fmt.Errorf("failed to save podcast config: %w", err)
	}
	return cfg, l.SyncPolicy(dir, uuid, shortID, cfg), nil
}

// GroupPolicyResult summarises applying one update across a group. Applied is
// the resulting configuration of the last podcast changed, which the command
// line reports as representative of the group.
type GroupPolicyResult struct {
	Updated int
	Applied config.PodcastConfig
}

// SetGroupPolicy applies one update to every podcast in a group. The backend
// is reached through this Library, so the connection is established once for
// the whole group rather than rebuilt per podcast.
func (l *Library) SetGroupPolicy(entries []PodcastDirEntry, u PolicyUpdate, defaults config.PolicyDefaults) (GroupPolicyResult, error) {
	var res GroupPolicyResult
	for _, entry := range entries {
		cfg := config.LoadPodcastConfig(entry.Dir, config.DefaultPodcastConfigFrom(defaults))
		applied, _, err := l.SetPodcastPolicy(entry.Dir, "", entry.ShortID, cfg, u)
		if err != nil {
			return res, fmt.Errorf("failed to save policy for %s: %w", entry.Title, err)
		}
		res.Applied = applied
		res.Updated++
	}
	return res, nil
}
