package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/types"
)

const (
	PodcastConfigFileName = "podcast.json"

	AdRemovalNone   = "none"
	AdRemovalLatest = "latest"
	AdRemovalAll    = "all"

	DownloadPolicyNone    = "none"
	DownloadPolicyLatest  = "latest"
	DownloadPolicyLatestK = "latest_k"
	DownloadPolicyAll     = "all"
	DownloadPolicyNew     = "new"
)

type PodcastConfig struct {
	ID              string                      `json:"id,omitempty"`
	Priority        int                         `json:"priority"`
	Favorite        bool                        `json:"favorite,omitempty"`
	FavoriteSince   *time.Time                  `json:"favorite_since,omitempty"`
	AdRemoval       string                      `json:"ad_removal"`
	DownloadPolicy  string                      `json:"download_policy,omitempty"`
	DownloadK       int                         `json:"download_k,omitempty"`
	AutoDownload    *bool                       `json:"auto_download,omitempty"`
	AutoCleanup     *bool                       `json:"auto_cleanup,omitempty"`
	AutoCleanupDays int                         `json:"auto_cleanup_days,omitempty"`
	Frequency       *types.PodcastFrequencyInfo `json:"frequency,omitempty"`
	UpdatedAt       time.Time                   `json:"updated_at,omitempty"`
}

func (c *PodcastConfig) SetFavorite(fav bool) {
	c.Favorite = fav
	if fav {
		now := time.Now().UTC()
		if c.FavoriteSince == nil {
			c.FavoriteSince = &now
		}
		autoDl := true
		c.AutoDownload = &autoDl
		c.AdRemoval = AdRemovalAll
		c.DownloadPolicy = DownloadPolicyNew
	} else {
		c.FavoriteSince = nil
		if c.DownloadPolicy == DownloadPolicyNew {
			c.DownloadPolicy = DownloadPolicyNone
		}
	}
}

func (c *PodcastConfig) IsAutoDownloadEnabled() bool {
	if c.AutoDownload != nil {
		return *c.AutoDownload
	}
	return NormalizeDownloadPolicy(c.DownloadPolicy) != DownloadPolicyNone
}

func (c *PodcastConfig) SetAutoDownload(enabled bool) {
	c.AutoDownload = &enabled
	if !enabled {
		c.DownloadPolicy = DownloadPolicyNone
	} else if NormalizeDownloadPolicy(c.DownloadPolicy) == DownloadPolicyNone {
		c.DownloadPolicy = DownloadPolicyLatest
	}
}

func (c *PodcastConfig) IsAutoCleanupEnabled() bool {
	if c.AutoCleanup != nil {
		return *c.AutoCleanup
	}
	return c.AutoCleanupDays > 0
}

func (c *PodcastConfig) SetAutoCleanup(enabled bool) {
	c.AutoCleanup = &enabled
	if !enabled {
		c.AutoCleanupDays = -1
	} else if c.AutoCleanupDays <= 0 {
		c.AutoCleanupDays = 30
	}
}

// PolicyDefaults are the three application-wide settings that shape a
// podcast's own configuration when it has none of its own. Callers that hold
// only these — the podcast library, for one — pass them directly instead of
// synthesising a whole types.Config around them.
type PolicyDefaults struct {
	DownloadPolicy string
	DownloadK      int
	AdRemoval      string
}

func DefaultPodcastConfig(appCfg *types.Config) PodcastConfig {
	var d PolicyDefaults
	if appCfg != nil {
		d = PolicyDefaults{
			DownloadPolicy: appCfg.DefaultDownloadPolicy,
			DownloadK:      appCfg.DefaultDownloadK,
			AdRemoval:      appCfg.DefaultAdRemoval,
		}
	}
	return DefaultPodcastConfigFrom(d)
}

func DefaultPodcastConfigFrom(d PolicyDefaults) PodcastConfig {
	dlPolicy := "latest"
	dlK := 3
	adPolicy := "all"
	if d.DownloadPolicy != "" {
		dlPolicy = d.DownloadPolicy
	}
	if d.DownloadK > 0 {
		dlK = d.DownloadK
	}
	if d.AdRemoval != "" {
		adPolicy = d.AdRemoval
	}
	autoDl := NormalizeDownloadPolicy(dlPolicy) != DownloadPolicyNone
	autoCl := false
	return PodcastConfig{
		AdRemoval:       NormalizeAdRemovalMode(adPolicy),
		DownloadPolicy:  NormalizeDownloadPolicy(dlPolicy),
		DownloadK:       dlK,
		AutoDownload:    &autoDl,
		AutoCleanup:     &autoCl,
		AutoCleanupDays: -1,
	}
}

func NormalizeAdRemovalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "latest", "last", "recent", "newest":
		return AdRemovalLatest
	case "all", "every", "full":
		return AdRemovalAll
	default:
		return AdRemovalNone
	}
}

func CycleAdRemovalMode(current string) string {
	switch NormalizeAdRemovalMode(current) {
	case AdRemovalNone:
		return AdRemovalLatest
	case AdRemovalLatest:
		return AdRemovalAll
	case AdRemovalAll:
		return AdRemovalNone
	default:
		return AdRemovalNone
	}
}

func AdRemovalModeLabel(mode string) string {
	switch NormalizeAdRemovalMode(mode) {
	case AdRemovalLatest:
		return "Remove from latest episode"
	case AdRemovalAll:
		return "Remove from all episodes"
	default:
		return "No ad removal"
	}
}

func AdRemovalModeBadge(mode string) string {
	switch NormalizeAdRemovalMode(mode) {
	case AdRemovalLatest:
		return "[Ads: Latest]"
	case AdRemovalAll:
		return "[Ads: All]"
	default:
		return "[Ads: None]"
	}
}

func NormalizeDownloadPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "latest", "last", "recent", "newest", "1", "single":
		return DownloadPolicyLatest
	case "latest_k", "latest-k", "latestk", "last_k", "last-k", "recent_k", "more_k", "more-k", "morek", "next_k", "next-k", "more":
		return DownloadPolicyLatestK
	case "all", "every", "full":
		return DownloadPolicyAll
	case "new", "favorite", "fav":
		return DownloadPolicyNew
	case "none", "off", "disabled", "no", "manual":
		return DownloadPolicyNone
	default:
		return DownloadPolicyNone
	}
}

func DownloadPolicyLabel(policy string, k int) string {
	if k <= 0 {
		k = 3
	}
	switch NormalizeDownloadPolicy(policy) {
	case DownloadPolicyLatest:
		return "Latest episode only (latest)"
	case DownloadPolicyLatestK:
		return fmt.Sprintf("Latest %d episodes (latest_k)", k)
	case DownloadPolicyNew:
		return "New episodes only (new/favorite)"
	case DownloadPolicyAll:
		return "All episodes (all)"
	default:
		return "No automatic downloads (none)"
	}
}

func DownloadPolicyBadge(policy string, k int) string {
	if k <= 0 {
		k = 3
	}
	switch NormalizeDownloadPolicy(policy) {
	case DownloadPolicyLatest:
		return "[DL: Latest]"
	case DownloadPolicyLatestK:
		return fmt.Sprintf("[DL: Latest %d]", k)
	case DownloadPolicyNew:
		return "[DL: New]"
	case DownloadPolicyAll:
		return "[DL: All]"
	default:
		return "[DL: None]"
	}
}

func LoadPodcastConfig(dir string, def PodcastConfig) PodcastConfig {
	cfgPath := filepath.Join(dir, PodcastConfigFileName)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return def
	}
	var cfg PodcastConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return def
	}
	if cfg.AdRemoval == "" || cfg.DownloadPolicy == "" || cfg.DownloadK <= 0 || cfg.ID == "" {
		extractLegacyPodcastConfigFields(data, &cfg)
	}
	normalizeLoadedPodcastConfig(&cfg, def)
	return cfg
}

func extractLegacyPodcastConfigFields(data []byte, cfg *PodcastConfig) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	if cfg.ID == "" {
		if v, ok := raw["id"].(string); ok {
			cfg.ID = v
		} else if v, ok := raw["podcast_id"].(string); ok {
			cfg.ID = v
		}
	}
	if cfg.AdRemoval == "" {
		if v, ok := raw["ad_removal_mode"].(string); ok {
			cfg.AdRemoval = v
		} else if v, ok := raw["status"].(string); ok {
			cfg.AdRemoval = v
		}
	}
	if cfg.DownloadPolicy == "" {
		if v, ok := raw["download_policy"].(string); ok {
			cfg.DownloadPolicy = v
		} else if v, ok := raw["download_mode"].(string); ok {
			cfg.DownloadPolicy = v
		} else if v, ok := raw["policy"].(string); ok {
			cfg.DownloadPolicy = v
		}
	}
	if cfg.DownloadK <= 0 {
		if v, ok := raw["download_k"].(float64); ok && v > 0 {
			cfg.DownloadK = int(v)
		}
	}
}

func normalizeLoadedPodcastConfig(cfg *PodcastConfig, def PodcastConfig) {
	if cfg.AdRemoval == "" {
		cfg.AdRemoval = def.AdRemoval
	} else {
		cfg.AdRemoval = NormalizeAdRemovalMode(cfg.AdRemoval)
	}
	if cfg.DownloadPolicy == "" {
		cfg.DownloadPolicy = def.DownloadPolicy
	} else {
		cfg.DownloadPolicy = NormalizeDownloadPolicy(cfg.DownloadPolicy)
	}
	if cfg.DownloadK <= 0 {
		cfg.DownloadK = def.DownloadK
	}
	if cfg.AutoDownload == nil {
		autoDl := NormalizeDownloadPolicy(cfg.DownloadPolicy) != DownloadPolicyNone
		cfg.AutoDownload = &autoDl
	}
	if cfg.AutoCleanup == nil {
		autoCl := cfg.AutoCleanupDays > 0
		cfg.AutoCleanup = &autoCl
	}
	if cfg.AutoCleanupDays == 0 {
		if cfg.AutoCleanup != nil && *cfg.AutoCleanup {
			cfg.AutoCleanupDays = 30
		} else {
			cfg.AutoCleanupDays = -1
		}
	}
}

func SavePodcastConfig(dir string, cfg PodcastConfig) error {
	if cfg.Priority < 0 || cfg.Priority > 10 {
		return fmt.Errorf("podcast priority must be between 0 and 10")
	}
	cfg.AdRemoval = NormalizeAdRemovalMode(cfg.AdRemoval)
	cfg.DownloadPolicy = NormalizeDownloadPolicy(cfg.DownloadPolicy)
	if cfg.DownloadPolicy == DownloadPolicyNone {
		autoDl := false
		cfg.AutoDownload = &autoDl
	} else if cfg.AutoDownload != nil && !*cfg.AutoDownload {
		cfg.DownloadPolicy = DownloadPolicyNone
	} else {
		autoDl := true
		cfg.AutoDownload = &autoDl
	}

	if cfg.AutoCleanupDays <= 0 {
		autoCl := false
		cfg.AutoCleanup = &autoCl
		cfg.AutoCleanupDays = -1
	} else if cfg.AutoCleanup != nil && !*cfg.AutoCleanup {
		cfg.AutoCleanupDays = -1
	} else {
		autoCl := true
		cfg.AutoCleanup = &autoCl
	}

	if cfg.DownloadK <= 0 {
		cfg.DownloadK = 3
	}
	cfg.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, PodcastConfigFileName)
	return os.WriteFile(cfgPath, append(data, '\n'), 0644)
}
