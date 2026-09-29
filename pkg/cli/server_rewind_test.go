package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/podcast"
)

func newRewindCLIFixture(t *testing.T) (Config, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	cfg := Config{PodcastsDir: filepath.Join(root, "lib"), SubscriptionsFile: filepath.Join(root, "subs.json")}
	store, err := podcast.NewSubscriptionStore(cfg.SubscriptionsFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(podcast.Subscription{ID: "s1", Title: "Show One", FeedURL: "https://example.test/one.xml", Folder: "Show_One"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.PodcastsDir, "Show_One")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	recent := filepath.Join(dir, "recent.mp3")
	if err := os.WriteFile(recent, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "old.mp3")
	if err := os.WriteFile(old, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	return cfg, dir
}

func TestServerRewindDryRunDeletesNothing(t *testing.T) {
	cfg, dir := newRewindCLIFixture(t)
	var out bytes.Buffer
	cli := CLIOptions{Out: &out, Args: []string{"24h"}}
	cli.DryRun = true
	if err := handleServerRewind(cfg, cli); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "recent.mp3")); err != nil {
		t.Fatalf("dry run deleted the episode: %v", err)
	}
	if !strings.Contains(out.String(), "recent.mp3") || strings.Contains(out.String(), "old.mp3") {
		t.Errorf("dry run should list only the recent episode, got:\n%s", out.String())
	}
}

func TestServerRewindConfirmationGatesDeletion(t *testing.T) {
	for answer, deleted := range map[string]bool{"n\n": false, "": false, "y\n": true} {
		cfg, dir := newRewindCLIFixture(t)
		var out bytes.Buffer
		cli := CLIOptions{Out: &out, In: strings.NewReader(answer), Args: []string{"24h"}}
		if err := handleServerRewind(cfg, cli); err != nil {
			t.Fatalf("answer %q: %v", answer, err)
		}
		_, err := os.Stat(filepath.Join(dir, "recent.mp3"))
		if gone := os.IsNotExist(err); gone != deleted {
			t.Errorf("answer %q: deleted=%v, want %v", answer, gone, deleted)
		}
		if _, err := os.Stat(filepath.Join(dir, "old.mp3")); err != nil {
			t.Errorf("answer %q: old episode must survive: %v", answer, err)
		}
	}
}

func TestServerRewindRejectsBadWindow(t *testing.T) {
	cfg, _ := newRewindCLIFixture(t)
	for _, arg := range []string{"24", "soon", "-1h"} {
		if err := handleServerRewind(cfg, CLIOptions{Args: []string{arg}}); err == nil {
			t.Errorf("window %q should be rejected", arg)
		}
	}
}
