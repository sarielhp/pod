package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"strings"
	"testing"
)

func TestServerAddRejectsSearchIDWithoutSaving(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"8056024"}, {"8056024", "Show"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := t.TempDir()
			cfg := Config{PodcastsDir: dir, SubscriptionsFile: filepath.Join(dir, "podcasts.json")}
			err := handleServerAdd(cfg, CLIOptions{Args: args, Out: io.Discard})
			if err == nil || !strings.Contains(err.Error(), "pod search add <id>") {
				t.Fatalf("expected search ID guidance, got %v", err)
			}
			if _, err := os.Stat(cfg.SubscriptionsFile); !os.IsNotExist(err) {
				t.Fatalf("invalid feed created a subscriptions file: %v", err)
			}
		})
	}
}

func TestServerSubscriptionCommands(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	subsFile := filepath.Join(tmpDir, "podcasts.json")
	podcastsDir := filepath.Join(tmpDir, "podcasts")
	_ = os.MkdirAll(podcastsDir, 0755)

	cfg := Config{
		PodcastsDir:       podcastsDir,
		SubscriptionsFile: subsFile,
		ServerBaseURL:     "http://myserver.tailscale.net:8080/podcasts",
	}

	// 1. Test Add
	addCli := CLIOptions{
		Args: []string{"https://feeds.example.com/testpodcast.xml", "Test Podcast"},
	}
	if err := handleServerAdd(cfg, addCli); err != nil {
		t.Fatalf("handleServerAdd failed: %v", err)
	}

	// 2. Test Feed generation
	feedCli := CLIOptions{
		Args: []string{"Test Podcast"},
	}
	if err := handleServerFeed(cfg, feedCli); err != nil {
		t.Fatalf("handleServerFeed failed: %v", err)
	}

	feedPath := filepath.Join(podcastsDir, podcast.SanitizeTitle("Test Podcast"), "feed.xml")
	if _, err := os.Stat(feedPath); err != nil {
		t.Fatalf("expected feed.xml at %s: %v", feedPath, err)
	}

	// 3. Test List
	listCli := CLIOptions{
		ProcOptions: ProcOptions{Quiet: true},
	}
	_ = handleServerList(cfg, listCli)

	// 4. Test Remove
	removeCli := CLIOptions{
		Args: []string{"Test Podcast"},
	}
	if err := handleServerRemove(cfg, removeCli); err != nil {
		t.Fatalf("handleServerRemove failed: %v", err)
	}
}

func TestConfigSetGetServerBaseURL(t *testing.T) {
	config.SetTestConfigPath(filepath.Join(t.TempDir(), "config.json"))
	defer config.SetTestConfigPath("")

	cfg := Config{}
	if err := handleConfigSet(io.Discard, &cfg, "server-base-url", "http://test.server:8080/podcasts/"); err != nil {
		t.Fatalf("handleConfigSet server-base-url failed: %v", err)
	}
	if cfg.ServerBaseURL != "http://test.server:8080/podcasts" {
		t.Fatalf("expected trimmed server base url, got %q", cfg.ServerBaseURL)
	}
}

func TestConfigSetGetArchive(t *testing.T) {
	config.SetTestConfigPath(filepath.Join(t.TempDir(), "config.json"))
	defer config.SetTestConfigPath("")

	cfg := Config{}
	if err := handleConfigSet(io.Discard, &cfg, "archive-enabled", "true"); err != nil {
		t.Fatalf("handleConfigSet archive-enabled failed: %v", err)
	}
	if !cfg.ArchiveEnabled {
		t.Fatal("expected ArchiveEnabled to be true")
	}

	if err := handleConfigSet(io.Discard, &cfg, "archive-dir", "/mnt/hdd/podcasts_archive"); err != nil {
		t.Fatalf("handleConfigSet archive-dir failed: %v", err)
	}
	if cfg.ArchiveDir != "/mnt/hdd/podcasts_archive" {
		t.Fatalf("expected archive dir %q, got %q", "/mnt/hdd/podcasts_archive", cfg.ArchiveDir)
	}

	if err := handleConfigSet(io.Discard, &cfg, "archive-base-url", "http://test.server:8080/podcasts/archive/"); err != nil {
		t.Fatalf("handleConfigSet archive-base-url failed: %v", err)
	}
	if cfg.ArchiveBaseURL != "http://test.server:8080/podcasts/archive" {
		t.Fatalf("expected trimmed archive base url, got %q", cfg.ArchiveBaseURL)
	}

	var buf bytes.Buffer
	if err := handleConfigGet(&buf, cfg, "archive-enabled"); err != nil {
		t.Fatalf("handleConfigGet archive-enabled failed: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "true" {
		t.Fatalf("expected true, got %q", buf.String())
	}
}
