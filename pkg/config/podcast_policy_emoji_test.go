package config

import (
	"testing"
)

func TestPolicyEmojiBadges(t *testing.T) {
	tests := []struct {
		name     string
		cfg      PodcastConfig
		wantDl   string
		wantRet  string
		wantAd   string
		wantComp string
	}{
		{
			name: "all defaults (latest, month, all)",
			cfg: PodcastConfig{
				DownloadPolicy: DownloadPolicyLatest,
				KeepPolicy:     KeepPolicyMonth,
				AdRemoval:      AdRemovalAll,
			},
			wantDl:   "📥New",
			wantRet:  "🗓️30d",
			wantAd:   "✂️All",
			wantComp: "📥New  🗓️30d  ✂️All",
		},
		{
			name: "disabled download and retention",
			cfg: func() PodcastConfig {
				noDl := false
				noCl := false
				return PodcastConfig{
					AutoDownload: &noDl,
					AutoCleanup:  &noCl,
					KeepPolicy:   KeepPolicyAlways,
					AdRemoval:    AdRemovalNone,
				}
			}(),
			wantDl:   "📥Off",
			wantRet:  "♾️Keep",
			wantAd:   "🚫Off",
			wantComp: "📥Off  ♾️Keep  🚫Off",
		},
		{
			name: "latest k and favorite",
			cfg: PodcastConfig{
				DownloadPolicy: DownloadPolicyLatestK,
				DownloadK:      5,
				KeepPolicy:     KeepPolicyFavorite,
				AdRemoval:      AdRemovalLatest,
			},
			wantDl:   "📥Top5",
			wantRet:  "⭐180d",
			wantAd:   "⚡New",
			wantComp: "📥Top5  ⭐180d  ⚡New",
		},
		{
			name: "custom days retention",
			cfg: PodcastConfig{
				DownloadPolicy:  DownloadPolicyAll,
				AutoCleanupDays: 14,
				KeepPolicy:      "14d",
				AdRemoval:       AdRemovalAll,
			},
			wantDl:   "📥All",
			wantRet:  "🗓️14d",
			wantAd:   "✂️All",
			wantComp: "📥All  🗓️14d  ✂️All",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := CompactPolicySummary(tt.cfg)
			if comp != tt.wantComp {
				t.Errorf("CompactPolicySummary() = %q, want %q", comp, tt.wantComp)
			}
		})
	}
}

func TestDetailedPolicySummary(t *testing.T) {
	cfg := PodcastConfig{
		DownloadPolicy: DownloadPolicyLatest,
		KeepPolicy:     KeepPolicyMonth,
		AdRemoval:      AdRemovalAll,
	}
	line := DetailedPolicySummary(cfg)
	if line == "" {
		t.Errorf("expected non-empty detailed policy line")
	}
	if !contains(line, "Download:") || !contains(line, "Retention:") || !contains(line, "Ad Removal:") {
		t.Errorf("unexpected detailed policy line: %s", line)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && findSubstr(s, substr)))
}

func findSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
