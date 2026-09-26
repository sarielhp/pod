package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/types"
)

func TestDefaultConfigNoUsername(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	u := os.Getenv("USER")
	if u != "" && strings.Contains(strings.ToLower(string(data)), strings.ToLower(u)) {
		t.Errorf("DefaultConfig contains current username %q: %s", u, string(data))
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.ActiveProfileID != 5 {
		t.Errorf("expected ActiveProfileID 5, got %d", cfg.ActiveProfileID)
	}
	if len(cfg.Profiles) == 0 {
		t.Error("expected default profiles to not be empty")
	}
	if len(cfg.WhisperProfiles) == 0 {
		t.Error("expected default whisper profiles to not be empty")
	}
}

func TestEnsureConfigExists(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "config.json")
	SetTestConfigPath(confPath)
	defer SetTestConfigPath("")

	cfg, err := EnsureConfigExists()
	if err != nil {
		t.Fatalf("EnsureConfigExists failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if _, err := os.Stat(confPath); err != nil {
		t.Fatalf("expected config file to be created at %s", confPath)
	}

	// Loading again should read existing
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.ActiveProfileID != cfg.ActiveProfileID {
		t.Errorf("expected ActiveProfileID %d, got %d", cfg.ActiveProfileID, loaded.ActiveProfileID)
	}
}

func TestPodcastConfigCycle(t *testing.T) {
	t.Parallel()
	mode := AdRemovalNone
	mode = CycleAdRemovalMode(mode)
	if mode != AdRemovalLatest {
		t.Errorf("expected AdRemovalLatest, got %s", mode)
	}
	mode = CycleAdRemovalMode(mode)
	if mode != AdRemovalAll {
		t.Errorf("expected AdRemovalAll, got %s", mode)
	}
	mode = CycleAdRemovalMode(mode)
	if mode != AdRemovalNone {
		t.Errorf("expected AdRemovalNone, got %s", mode)
	}
}

func TestSubscriptionsFilePath(t *testing.T) {
	t.Parallel()
	defaultPath := SubscriptionsFilePath(nil)
	if !strings.HasSuffix(defaultPath, "podcasts.json") {
		t.Errorf("expected default path to end in podcasts.json, got %s", defaultPath)
	}

	customCfg := &types.Config{SubscriptionsFile: "/tmp/my_subs.json"}
	if got := SubscriptionsFilePath(customCfg); got != "/tmp/my_subs.json" {
		t.Errorf("expected /tmp/my_subs.json, got %s", got)
	}
}

func TestKeepPolicyNormalizationAndCycle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
		days  int
	}{
		{"always", KeepPolicyAlways, -1},
		{"forever", KeepPolicyAlways, -1},
		{"never", KeepPolicyAlways, -1},
		{"month", KeepPolicyMonth, 30},
		{"regular", KeepPolicyMonth, 30},
		{"30d", KeepPolicyMonth, 30},
		{"favorite", KeepPolicyFavorite, 180},
		{"6months", KeepPolicyFavorite, 180},
		{"hourly", KeepPolicyHourly, 1},
		{"day", KeepPolicyHourly, 1},
		{"1d", KeepPolicyHourly, 1},
		{"14d", "14d", 14},
		{"14", "14d", 14},
	}
	for _, tc := range cases {
		norm := NormalizeKeepPolicy(tc.input)
		if norm != tc.want {
			t.Errorf("NormalizeKeepPolicy(%q) = %q, want %q", tc.input, norm, tc.want)
		}
		days, ok := ParseKeepPolicyDays(tc.input)
		if !ok || days != tc.days {
			t.Errorf("ParseKeepPolicyDays(%q) = (%d, %v), want (%d, true)", tc.input, days, ok, tc.days)
		}
	}

	p := KeepPolicyAlways
	p = CycleKeepPolicy(p)
	if p != KeepPolicyMonth {
		t.Errorf("expected KeepPolicyMonth, got %s", p)
	}
	p = CycleKeepPolicy(p)
	if p != KeepPolicyFavorite {
		t.Errorf("expected KeepPolicyFavorite, got %s", p)
	}
	p = CycleKeepPolicy(p)
	if p != KeepPolicyHourly {
		t.Errorf("expected KeepPolicyHourly, got %s", p)
	}
	p = CycleKeepPolicy(p)
	if p != KeepPolicyAlways {
		t.Errorf("expected KeepPolicyAlways, got %s", p)
	}
}

func TestPodcastConfigEffectiveKeepPolicy(t *testing.T) {
	t.Parallel()
	// Regular podcast defaults to month (30 days)
	regular := PodcastConfig{}
	if regular.EffectiveKeepPolicy() != KeepPolicyMonth || regular.EffectiveCleanupDays() != 30 {
		t.Errorf("expected regular to be month (30d), got %s (%d)", regular.EffectiveKeepPolicy(), regular.EffectiveCleanupDays())
	}

	// Favorite podcast defaults to favorite (180 days / 6 months)
	fav := PodcastConfig{Favorite: true}
	if fav.EffectiveKeepPolicy() != KeepPolicyFavorite || fav.EffectiveCleanupDays() != 180 {
		t.Errorf("expected fav to be favorite (180d), got %s (%d)", fav.EffectiveKeepPolicy(), fav.EffectiveCleanupDays())
	}

	// Hourly news podcast defaults to hourly (1 day)
	hourly := PodcastConfig{Frequency: &types.PodcastFrequencyInfo{Type: "hourly"}}
	if hourly.EffectiveKeepPolicy() != KeepPolicyHourly || hourly.EffectiveCleanupDays() != 1 {
		t.Errorf("expected hourly to be hourly (1d), got %s (%d)", hourly.EffectiveKeepPolicy(), hourly.EffectiveCleanupDays())
	}

	// Explicit keep always
	always := PodcastConfig{}
	always.SetKeepPolicy("always")
	if always.EffectiveKeepPolicy() != KeepPolicyAlways || always.EffectiveCleanupDays() != -1 {
		t.Errorf("expected always to be always (-1d), got %s (%d)", always.EffectiveKeepPolicy(), always.EffectiveCleanupDays())
	}

	// Custom days
	custom := PodcastConfig{}
	custom.SetKeepPolicy("14d")
	if custom.EffectiveKeepPolicy() != "14d" || custom.EffectiveCleanupDays() != 14 {
		t.Errorf("expected custom 14d, got %s (%d)", custom.EffectiveKeepPolicy(), custom.EffectiveCleanupDays())
	}
}
