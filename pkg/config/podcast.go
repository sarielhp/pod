package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pod/pkg/types"
	"pod/pkg/util"
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

	KeepPolicyAlways   = "always"
	KeepPolicyMonth    = "month"
	KeepPolicyFavorite = "favorite"
	KeepPolicyHourly   = "hourly"
)

// BoilerplatePhrase is text that recurs across a show's episodes and is cut
// from each new one before ad detection: intros, credits, house promos, standing
// sponsor reads. `pod analyze` writes them; Disabled keeps a phrase on file
// without cutting it, and Manual protects an entry from being dropped when the
// analysis is rerun.
type BoilerplatePhrase struct {
	Text     string `json:"text"`
	Episodes int    `json:"episodes,omitempty"`
	Position string `json:"position,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Manual   bool   `json:"manual,omitempty"`
}

type PodcastConfig struct {
	ID              string                      `json:"id,omitempty"`
	Icon            string                      `json:"icon,omitempty"`
	Summary         string                      `json:"summary,omitempty"`
	Priority        int                         `json:"priority"`
	Favorite        bool                        `json:"favorite,omitempty"`
	FavoriteSince   *time.Time                  `json:"favorite_since,omitempty"`
	AdRemoval       string                      `json:"ad_removal"`
	DownloadPolicy  string                      `json:"download_policy,omitempty"`
	DownloadK       int                         `json:"download_k,omitempty"`
	AutoDownload    *bool                       `json:"auto_download,omitempty"`
	AutoCleanup     *bool                       `json:"auto_cleanup,omitempty"`
	AutoCleanupDays int                         `json:"auto_cleanup_days,omitempty"`
	KeepPolicy      string                      `json:"keep_policy,omitempty"`
	Frequency       *types.PodcastFrequencyInfo `json:"frequency,omitempty"`
	Boilerplate     []BoilerplatePhrase         `json:"boilerplate,omitempty"`
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
		if c.KeepPolicy == "" || c.KeepPolicy == KeepPolicyMonth {
			c.SetKeepPolicy(KeepPolicyFavorite)
		}
	} else {
		c.FavoriteSince = nil
		if c.DownloadPolicy == DownloadPolicyNew {
			c.DownloadPolicy = DownloadPolicyNone
		}
		if c.KeepPolicy == KeepPolicyFavorite {
			c.SetKeepPolicy(KeepPolicyMonth)
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
		c.KeepPolicy = KeepPolicyAlways
	} else if c.AutoCleanupDays <= 0 {
		c.AutoCleanupDays = 30
		c.KeepPolicy = KeepPolicyMonth
	}
}

func (c *PodcastConfig) EffectiveKeepPolicy() string {
	if c.KeepPolicy != "" {
		return NormalizeKeepPolicy(c.KeepPolicy)
	}
	if c.AutoCleanup != nil && !*c.AutoCleanup && c.AutoCleanupDays < 0 {
		return KeepPolicyAlways
	}
	if c.AutoCleanupDays > 0 {
		switch c.AutoCleanupDays {
		case 30:
			return KeepPolicyMonth
		case 180:
			return KeepPolicyFavorite
		case 1:
			return KeepPolicyHourly
		default:
			return fmt.Sprintf("%dd", c.AutoCleanupDays)
		}
	}
	if c.Favorite {
		return KeepPolicyFavorite
	}
	if c.Frequency != nil && c.Frequency.Type == "hourly" {
		return KeepPolicyHourly
	}
	return KeepPolicyMonth
}

func (c *PodcastConfig) EffectiveCleanupDays() int {
	days, ok := ParseKeepPolicyDays(c.EffectiveKeepPolicy())
	if ok {
		return days
	}
	if c.AutoCleanupDays > 0 {
		return c.AutoCleanupDays
	}
	return -1
}

func (c *PodcastConfig) SetKeepPolicy(policy string) {
	norm := NormalizeKeepPolicy(policy)
	c.KeepPolicy = norm
	days, ok := ParseKeepPolicyDays(norm)
	if !ok || days <= 0 {
		c.SetAutoCleanup(false)
	} else {
		c.AutoCleanupDays = days
		autoCl := true
		c.AutoCleanup = &autoCl
	}
}

// PolicyDefaults are the application-wide settings that shape a
// podcast's own configuration when it has none of its own.
type PolicyDefaults struct {
	DownloadPolicy string
	DownloadK      int
	AdRemoval      string
	KeepPolicy     string
}

func DefaultPodcastConfig(appCfg *types.Config) PodcastConfig {
	var d PolicyDefaults
	if appCfg != nil {
		d = PolicyDefaults{
			DownloadPolicy: appCfg.DefaultDownloadPolicy,
			DownloadK:      appCfg.DefaultDownloadK,
			AdRemoval:      appCfg.DefaultAdRemoval,
			KeepPolicy:     appCfg.DefaultKeepPolicy,
		}
	}
	return DefaultPodcastConfigFrom(d)
}

func DefaultPodcastConfigFrom(d PolicyDefaults) PodcastConfig {
	dlPolicy := "latest"
	dlK := 3
	adPolicy := "all"
	keepPolicy := KeepPolicyMonth
	if d.DownloadPolicy != "" {
		dlPolicy = d.DownloadPolicy
	}
	if d.DownloadK > 0 {
		dlK = d.DownloadK
	}
	if d.AdRemoval != "" {
		adPolicy = d.AdRemoval
	}
	if d.KeepPolicy != "" {
		keepPolicy = d.KeepPolicy
	}
	autoDl := NormalizeDownloadPolicy(dlPolicy) != DownloadPolicyNone
	autoCl := true
	days := 30
	if NormalizeKeepPolicy(keepPolicy) == KeepPolicyAlways {
		autoCl = false
		days = -1
	} else if pDays, ok := ParseKeepPolicyDays(keepPolicy); ok {
		days = pDays
	}
	return PodcastConfig{
		AdRemoval:       NormalizeAdRemovalMode(adPolicy),
		DownloadPolicy:  NormalizeDownloadPolicy(dlPolicy),
		DownloadK:       dlK,
		AutoDownload:    &autoDl,
		AutoCleanup:     &autoCl,
		AutoCleanupDays: days,
		KeepPolicy:      NormalizeKeepPolicy(keepPolicy),
	}
}

// DefaultDiscoveredPodcastConfig returns defaults for an unconfigured podcast
// found on disk without its own podcast.json. Ad removal defaults to "all" (or
// app default), download policy defaults to "none" (avoiding unintended downloads),
// and keep policy defaults to "month" (30 days).
func DefaultDiscoveredPodcastConfig(appCfg *types.Config) PodcastConfig {
	var d PolicyDefaults
	if appCfg != nil {
		d = PolicyDefaults{
			DownloadPolicy: DownloadPolicyNone,
			DownloadK:      3,
			AdRemoval:      appCfg.DefaultAdRemoval,
			KeepPolicy:     appCfg.DefaultKeepPolicy,
		}
	} else {
		d = PolicyDefaults{
			DownloadPolicy: DownloadPolicyNone,
			DownloadK:      3,
			AdRemoval:      AdRemovalAll,
			KeepPolicy:     KeepPolicyMonth,
		}
	}
	return DefaultPodcastConfigFrom(d)
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

func NormalizeKeepPolicy(policy string) string {
	s := strings.ToLower(strings.TrimSpace(policy))
	switch s {
	case "always", "forever", "infinite", "none", "keep-always", "keep_always", "never", "all", "keep-all", "keep_all":
		return KeepPolicyAlways
	case "month", "regular", "30d", "30days", "30", "monthly", "1m", "1month":
		return KeepPolicyMonth
	case "favorite", "6months", "6month", "6m", "180d", "180days", "180", "half-year":
		return KeepPolicyFavorite
	case "hourly", "news", "day", "daily", "1d", "1day", "24h", "1":
		return KeepPolicyHourly
	}
	s = strings.TrimSuffix(s, "days")
	s = strings.TrimSuffix(s, "day")
	s = strings.TrimSuffix(s, "d")
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		return fmt.Sprintf("%dd", n)
	}
	return s
}

func ParseKeepPolicyDays(policy string) (int, bool) {
	norm := NormalizeKeepPolicy(policy)
	switch norm {
	case KeepPolicyAlways:
		return -1, true
	case KeepPolicyMonth:
		return 30, true
	case KeepPolicyFavorite:
		return 180, true
	case KeepPolicyHourly:
		return 1, true
	}
	s := strings.TrimSuffix(norm, "d")
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n, true
	}
	return 0, false
}

func CycleKeepPolicy(current string) string {
	switch NormalizeKeepPolicy(current) {
	case KeepPolicyAlways:
		return KeepPolicyMonth
	case KeepPolicyMonth:
		return KeepPolicyFavorite
	case KeepPolicyFavorite:
		return KeepPolicyHourly
	case KeepPolicyHourly:
		return KeepPolicyAlways
	default:
		return KeepPolicyAlways
	}
}

func KeepPolicyLabel(policy string, days int) string {
	switch NormalizeKeepPolicy(policy) {
	case KeepPolicyAlways:
		return "Keep always (forever)"
	case KeepPolicyMonth:
		return "Regular (1 month / 30 days)"
	case KeepPolicyFavorite:
		return "Favorite (6 months / 180 days)"
	case KeepPolicyHourly:
		return "Hourly news (1 day)"
	default:
		if days > 0 {
			return fmt.Sprintf("Custom (%d days)", days)
		}
		if n, ok := ParseKeepPolicyDays(policy); ok && n > 0 {
			return fmt.Sprintf("Custom (%d days)", n)
		}
		return "Keep always (forever)"
	}
}

func KeepPolicyBadge(policy string, days int) string {
	switch NormalizeKeepPolicy(policy) {
	case KeepPolicyAlways:
		return "[Keep: Always]"
	case KeepPolicyMonth:
		return "[Keep: 1mo]"
	case KeepPolicyFavorite:
		return "[Keep: 6mo]"
	case KeepPolicyHourly:
		return "[Keep: 1d]"
	default:
		if days > 0 {
			return fmt.Sprintf("[Keep: %dd]", days)
		}
		if n, ok := ParseKeepPolicyDays(policy); ok && n > 0 {
			return fmt.Sprintf("[Keep: %dd]", n)
		}
		return "[Keep: Always]"
	}
}

func DownloadPolicyEmojiBadge(policy string, k int, enabled bool) string {
	if !enabled {
		return "📥Off"
	}
	switch NormalizeDownloadPolicy(policy) {
	case DownloadPolicyLatest:
		return "📥New"
	case DownloadPolicyLatestK:
		if k <= 0 {
			k = 3
		}
		return fmt.Sprintf("📥Top%d", k)
	case DownloadPolicyNew:
		return "📥New"
	case DownloadPolicyAll:
		return "📥All"
	default:
		return "📥Off"
	}
}

func KeepPolicyEmojiBadge(policy string, days int, enabled bool) string {
	if !enabled || NormalizeKeepPolicy(policy) == KeepPolicyAlways {
		return "♾️Keep"
	}
	switch NormalizeKeepPolicy(policy) {
	case KeepPolicyMonth:
		return "🗓️30d"
	case KeepPolicyFavorite:
		return "⭐180d"
	case KeepPolicyHourly:
		return "⏱️1d"
	default:
		if days > 0 {
			return fmt.Sprintf("🗓️%dd", days)
		}
		if n, ok := ParseKeepPolicyDays(policy); ok && n > 0 {
			return fmt.Sprintf("🗓️%dd", n)
		}
		return "♾️Keep"
	}
}

func AdRemovalEmojiBadge(mode string) string {
	switch NormalizeAdRemovalMode(mode) {
	case AdRemovalAll:
		return "✂️All"
	case AdRemovalLatest:
		return "⚡New"
	default:
		return "🚫Off"
	}
}

func CompactPolicySummary(cfg PodcastConfig) string {
	autoDl := cfg.IsAutoDownloadEnabled()
	autoCl := true
	if cfg.AutoCleanup != nil && !*cfg.AutoCleanup {
		autoCl = false
	} else if cfg.EffectiveKeepPolicy() == KeepPolicyAlways {
		autoCl = false
	}
	dl := DownloadPolicyEmojiBadge(cfg.DownloadPolicy, cfg.DownloadK, autoDl)
	ret := KeepPolicyEmojiBadge(cfg.EffectiveKeepPolicy(), cfg.EffectiveCleanupDays(), autoCl)
	adr := AdRemovalEmojiBadge(cfg.AdRemoval)
	return fmt.Sprintf("%s  %s  %s", dl, ret, adr)
}

func DetailedPolicySummary(cfg PodcastConfig) string {
	dl := DownloadPolicyLabel(cfg.DownloadPolicy, cfg.DownloadK)
	ret := KeepPolicyLabel(cfg.EffectiveKeepPolicy(), cfg.EffectiveCleanupDays())
	adr := AdRemovalModeLabel(cfg.AdRemoval)
	return fmt.Sprintf("Policy: 📥 Download: %s │ 🗓️ Retention: %s │ ✂️ Ad Removal: %s", dl, ret, adr)
}

func LoadPodcastConfig(dir string, def PodcastConfig) PodcastConfig {
	cfg, _ := LoadPodcastConfigErr(dir, def)
	return cfg
}

// LoadPodcastConfigErr loads dir's podcast.json. A missing file yields def and
// no error; a file that exists but cannot be parsed yields def and the parse
// error, so a caller about to write can tell a fresh podcast from a corrupt one.
func LoadPodcastConfigErr(dir string, def PodcastConfig) (PodcastConfig, error) {
	cfgPath := filepath.Join(dir, PodcastConfigFileName)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return def, nil
		}
		return def, fmt.Errorf("read %s: %w", cfgPath, err)
	}
	var cfg PodcastConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return def, fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	if cfg.AdRemoval == "" || cfg.DownloadPolicy == "" || cfg.DownloadK <= 0 || cfg.ID == "" {
		extractLegacyPodcastConfigFields(data, &cfg)
	}
	normalizeLoadedPodcastConfig(&cfg, def)
	return cfg, nil
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
	if cfg.KeepPolicy != "" {
		cfg.KeepPolicy = NormalizeKeepPolicy(cfg.KeepPolicy)
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

	if cfg.KeepPolicy != "" {
		cfg.KeepPolicy = NormalizeKeepPolicy(cfg.KeepPolicy)
	} else {
		cfg.KeepPolicy = cfg.EffectiveKeepPolicy()
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
	if err := preserveCorruptPodcastConfig(cfgPath); err != nil {
		return err
	}
	return util.WriteFileAtomic(cfgPath, append(data, '\n'), 0644)
}

func preserveCorruptPodcastConfig(cfgPath string) error {
	existing, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil
	}
	var probe PodcastConfig
	if json.Unmarshal(existing, &probe) == nil {
		return nil
	}
	backup := fmt.Sprintf("%s.corrupt-%s", cfgPath, time.Now().UTC().Format("20060102-150405.000000000"))
	if err := os.WriteFile(backup, existing, 0644); err != nil {
		return fmt.Errorf("back up unreadable %s before overwriting it: %w", cfgPath, err)
	}
	return nil
}
