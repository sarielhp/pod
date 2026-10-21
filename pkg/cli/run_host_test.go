package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/config"
)

func TestConfigSetGetRunHost(t *testing.T) {
	config.SetTestConfigPath(filepath.Join(t.TempDir(), "config.json"))
	defer config.SetTestConfigPath("")

	cfg := Config{}
	if err := handleConfigSet(io.Discard, &cfg, "run-host", "qed"); err != nil {
		t.Fatalf("handleConfigSet run-host failed: %v", err)
	}
	if cfg.RunHost != "qed" {
		t.Fatalf("expected RunHost 'qed', got %q", cfg.RunHost)
	}

	var buf bytes.Buffer
	if err := handleConfigGet(&buf, cfg, "run-host"); err != nil {
		t.Fatalf("handleConfigGet run-host failed: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "qed" {
		t.Fatalf("expected get output 'qed', got %q", buf.String())
	}
}

func TestExecuteBlocksOperationalCommandsOnMismatchHost(t *testing.T) {
	confDir := t.TempDir()
	confPath := filepath.Join(confDir, "config.json")
	config.SetTestConfigPath(confPath)
	defer config.SetTestConfigPath("")

	cfg := Config{
		RunHost:           "definitely-not-this-host-98765",
		AllowPrivateHosts: true,
	}
	if err := config.SaveConfig(&cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	code := Execute([]string{"queue", "list"})

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	errOutput := buf.String()

	if code != 1 {
		t.Errorf("expected exit code 1 on host mismatch, got %d", code)
	}
	if !strings.Contains(errOutput, "pod must run from") {
		t.Errorf("expected error message mentioning 'pod must run from', got: %s", errOutput)
	}
}

func TestExecuteAllowsConfigCommandsOnMismatchHost(t *testing.T) {
	confDir := t.TempDir()
	confPath := filepath.Join(confDir, "config.json")
	config.SetTestConfigPath(confPath)
	defer config.SetTestConfigPath("")

	cfg := Config{
		RunHost:           "definitely-not-this-host-98765",
		AllowPrivateHosts: true,
	}
	if err := config.SaveConfig(&cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	code := Execute([]string{"config", "get", "run-host"})
	if code != 0 {
		t.Errorf("expected config get to succeed even on host mismatch, got exit code %d", code)
	}
}

func TestExecuteBypassesHostCheckWithEnvVar(t *testing.T) {
	confDir := t.TempDir()
	confPath := filepath.Join(confDir, "config.json")
	config.SetTestConfigPath(confPath)
	defer config.SetTestConfigPath("")

	cfg := Config{
		RunHost:           "definitely-not-this-host-98765",
		AllowPrivateHosts: true,
	}
	if err := config.SaveConfig(&cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	t.Setenv("POD_IGNORE_HOST", "1")
	// With POD_IGNORE_HOST=1, Execute proceeds past CheckRunHost.
	// For "queue list", it will execute queue list (which returns 0 or succeeds).
	code := Execute([]string{"queue", "list"})
	if code != 0 {
		t.Errorf("expected queue list to proceed when POD_IGNORE_HOST=1, got %d", code)
	}
}
