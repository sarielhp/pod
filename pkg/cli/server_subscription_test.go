package cli

import (
	"io"
	"os"
	"path/filepath"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"testing"
)

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
