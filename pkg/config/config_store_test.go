package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigRoundTripsThroughLoadConfig(t *testing.T) {
	t.Setenv("WHISPER_URL", "")
	confPath := filepath.Join(t.TempDir(), "nested", "config.json")
	SetTestConfigPath(confPath)
	defer SetTestConfigPath("")

	if err := SaveConfig(nil); err == nil {
		t.Fatal("a nil config must be rejected")
	}

	cfg := DefaultConfig()
	cfg.WhisperURL = "http://localhost:9999/inference"
	cfg.ActiveProfileID = 7
	if err := SaveConfig(&cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	fi, err := os.Stat(confPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("config holds API keys and must be 0600, got %o", fi.Mode().Perm())
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.WhisperURL != cfg.WhisperURL || loaded.ActiveProfileID != 7 {
		t.Fatalf("round trip lost fields: whisper %q profile %d", loaded.WhisperURL, loaded.ActiveProfileID)
	}
}

func TestSavePodcastConfigNormalisesWhatLoadPodcastConfigReads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := SavePodcastConfig(dir, PodcastConfig{Priority: 11}); err == nil {
		t.Fatal("priority 11 must be rejected")
	}
	if _, err := os.Stat(filepath.Join(dir, PodcastConfigFileName)); !os.IsNotExist(err) {
		t.Fatalf("a rejected save must not write podcast.json: %v", err)
	}

	in := PodcastConfig{ID: "abc", Priority: 5, AdRemoval: "LATEST", DownloadPolicy: DownloadPolicyNone, Summary: "notes"}
	if err := SavePodcastConfig(dir, in); err != nil {
		t.Fatalf("SavePodcastConfig: %v", err)
	}
	got := LoadPodcastConfig(dir, DefaultPodcastConfig(nil))
	if got.ID != "abc" || got.Priority != 5 || got.Summary != "notes" {
		t.Fatalf("plain fields lost on round trip: %+v", got)
	}
	if got.AdRemoval != AdRemovalLatest {
		t.Fatalf("AdRemoval = %q, want normalised %q", got.AdRemoval, AdRemovalLatest)
	}
	if got.DownloadPolicy != DownloadPolicyNone || got.AutoDownload == nil || *got.AutoDownload {
		t.Fatalf("download policy none must persist with auto_download false: %+v", got)
	}
	if got.AutoCleanupDays != -1 || got.AutoCleanup == nil || *got.AutoCleanup {
		t.Fatalf("no cleanup days must persist as disabled cleanup: days %d auto %v", got.AutoCleanupDays, got.AutoCleanup)
	}
	if got.DownloadK != 3 {
		t.Fatalf("DownloadK defaulted to %d, want 3", got.DownloadK)
	}
	if got.KeepPolicy == "" || got.UpdatedAt.IsZero() {
		t.Fatalf("keep policy %q and updated_at %v must be filled in", got.KeepPolicy, got.UpdatedAt)
	}
}
