package podcast

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/config"
)

func TestPolicyUpdateIsEmpty(t *testing.T) {
	t.Parallel()
	if !(PolicyUpdate{}).IsEmpty() {
		t.Error("a zero PolicyUpdate should be empty")
	}
	for _, u := range []PolicyUpdate{
		{Favorite: "true"}, {AutoDownload: "false"}, {DownloadPolicy: "latest"},
		{DownloadK: 1}, {AutoCleanup: "true"}, {CleanupDays: 7}, {AdRemoval: "all"},
	} {
		if u.IsEmpty() {
			t.Errorf("%+v should not be empty", u)
		}
	}
}

func TestParsePolicyBool(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"true", "TRUE", "1", "yes", "on", "enable", "enabled", " Yes "} {
		if !ParsePolicyBool(s) {
			t.Errorf("ParsePolicyBool(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"false", "0", "no", "off", "", "maybe"} {
		if ParsePolicyBool(s) {
			t.Errorf("ParsePolicyBool(%q) = true, want false", s)
		}
	}
}

// Asking for no downloads must also switch auto-download off, or the two
// settings contradict each other. But an explicit --auto-download on the same
// command wins, because the user said it outright.
func TestApplyPolicyUpdateNoneDisablesAutoDownload(t *testing.T) {
	t.Parallel()
	cfg := config.PodcastConfig{}
	applyPolicyUpdate(&cfg, PolicyUpdate{DownloadPolicy: "none"})
	if cfg.IsAutoDownloadEnabled() {
		t.Error("policy none should disable auto-download")
	}

	cfg = config.PodcastConfig{}
	applyPolicyUpdate(&cfg, PolicyUpdate{DownloadPolicy: "none", AutoDownload: "true"})
	if !cfg.IsAutoDownloadEnabled() {
		t.Error("an explicit auto-download should survive policy none")
	}
}

func TestApplyPolicyUpdateCleanupDaysEnablesCleanup(t *testing.T) {
	t.Parallel()
	cfg := config.PodcastConfig{}
	applyPolicyUpdate(&cfg, PolicyUpdate{CleanupDays: 14})
	if !cfg.IsAutoCleanupEnabled() || cfg.AutoCleanupDays != 14 {
		t.Errorf("setting a retention should enable cleanup: %+v", cfg)
	}
}

func TestApplyPolicyUpdateLeavesUnsetFieldsAlone(t *testing.T) {
	t.Parallel()
	cfg := config.PodcastConfig{DownloadPolicy: "latest_k", DownloadK: 5, AdRemoval: "all"}
	applyPolicyUpdate(&cfg, PolicyUpdate{CleanupDays: 3})

	if cfg.DownloadPolicy != "latest_k" || cfg.DownloadK != 5 || cfg.AdRemoval != "all" {
		t.Errorf("an update touched fields it did not name: %+v", cfg)
	}
	if cfg.AutoCleanupDays != 3 {
		t.Error("the named field was not applied")
	}
}

// Marking a podcast favourite is not just one flag: it also switches
// auto-download on, ad removal to all, and the download policy to "new". That
// is long-standing behaviour in PodcastConfig.SetFavorite, and it is
// surprising enough to state outright — a user who sets --favorite true after
// configuring a download policy will find the policy replaced.
func TestApplyPolicyUpdateFavoriteRewritesRelatedSettings(t *testing.T) {
	t.Parallel()
	cfg := config.PodcastConfig{DownloadPolicy: "latest_k", DownloadK: 5, AdRemoval: "none"}
	applyPolicyUpdate(&cfg, PolicyUpdate{Favorite: "true"})

	if !cfg.Favorite {
		t.Fatal("favourite not set")
	}
	if cfg.DownloadPolicy != config.DownloadPolicyNew {
		t.Errorf("DownloadPolicy = %q, want %q", cfg.DownloadPolicy, config.DownloadPolicyNew)
	}
	if cfg.AdRemoval != config.AdRemovalAll {
		t.Errorf("AdRemoval = %q, want %q", cfg.AdRemoval, config.AdRemovalAll)
	}
	if !cfg.IsAutoDownloadEnabled() {
		t.Error("auto-download should be on for a favourite")
	}
	if cfg.FavoriteSince == nil {
		t.Error("FavoriteSince should be stamped")
	}
	// DownloadK is not part of that rewrite.
	if cfg.DownloadK != 5 {
		t.Errorf("DownloadK = %d, want 5", cfg.DownloadK)
	}
}

func TestSetGroupPolicyAppliesToEveryPodcast(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var entries []PodcastDirEntry
	for _, name := range []string{"A", "B", "C"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, PodcastDirEntry{Dir: dir, FolderName: name, Title: name, ShortID: "id" + name})
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	res, err := lib.SetGroupPolicy(entries, PolicyUpdate{DownloadPolicy: "latest_k", DownloadK: 4}, config.PolicyDefaults{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 3 {
		t.Fatalf("updated %d, want 3", res.Updated)
	}
	for _, e := range entries {
		got := config.LoadPodcastConfig(e.Dir, config.PodcastConfig{})
		if got.DownloadPolicy != config.DownloadPolicyLatestK || got.DownloadK != 4 {
			t.Errorf("%s not updated: %+v", e.Title, got)
		}
	}
}

// With no backend configured, syncing is a no-op that reports no backend
// rather than failing the whole policy change.
func TestSyncPolicyWithoutBackendIsNotAnError(t *testing.T) {
	t.Parallel()
	lib := Open(Config{}, nil, nil)
	got := lib.SyncPolicy("/lib/Show", "", "abc", config.PodcastConfig{})
	if got.Backend != "" || got.Err != nil {
		t.Errorf("expected an empty sync result, got %+v", got)
	}
	if lib.BackendName() != "" {
		t.Errorf("BackendName() = %q, want empty", lib.BackendName())
	}
}

func TestGroupPoliciesReadsStoredSettingsOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "A")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.SavePodcastConfig(dir, config.PodcastConfig{DownloadPolicy: "latest", Favorite: true}); err != nil {
		t.Fatal(err)
	}

	states := GroupPolicies([]PodcastDirEntry{{Dir: dir, Title: "A", ShortID: "idA"}})
	if len(states) != 1 {
		t.Fatalf("got %d states", len(states))
	}
	if states[0].DownloadPolicy != "latest" || !states[0].Favorite || states[0].ID != "idA" {
		t.Errorf("unexpected state: %+v", states[0])
	}
}
