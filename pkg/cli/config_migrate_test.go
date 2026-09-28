package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigMigrateSkipsPostProcessorsThatDoNotResolve(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	good := filepath.Join(home, "good.sh")
	if err := os.WriteFile(good, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(home, "missing", "bad.sh")

	dir := filepath.Join(home, ".config", "podcasts_manager")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"post_processors": [%q, %q]}`, good, bad)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cfg := Config{}
	if !migratePodcastsManagerConfig(&out, &cfg) {
		t.Fatalf("expected the valid post-processor to be migrated; output: %s", out.String())
	}
	if len(cfg.PostProcessors) != 1 || cfg.PostProcessors[0] != good {
		t.Fatalf("expected only %q to be imported, got %v", good, cfg.PostProcessors)
	}
	if !strings.Contains(out.String(), bad) {
		t.Errorf("expected a warning naming the skipped program, got: %q", out.String())
	}
}

func TestConfigMigrateWithNoResolvablePostProcessorImportsNone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", "podcasts_manager")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(home, "missing", "bad.sh")
	body := fmt.Sprintf(`{"post_processors": [%q]}`, bad)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cfg := Config{}
	if migratePodcastsManagerConfig(&out, &cfg) {
		t.Fatalf("expected nothing to be migrated, got %v", cfg.PostProcessors)
	}
	if len(cfg.PostProcessors) != 0 {
		t.Fatalf("expected no post-processors, got %v", cfg.PostProcessors)
	}
}
