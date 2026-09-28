package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/config"
)

func unwritableConfigPath(t *testing.T) string {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(blocker, "config.json")
	config.SetTestConfigPath(p)
	t.Cleanup(func() { config.SetTestConfigPath("") })
	return p
}

func TestConfigSetReportsAFailedSave(t *testing.T) {
	unwritableConfigPath(t)
	var buf bytes.Buffer
	var cfg Config
	err := handleConfigSet(&buf, &cfg, "whisper-url", "http://localhost:9000")
	if err == nil {
		t.Fatal("handleConfigSet returned nil although the config could not be written")
	}
	if strings.Contains(buf.String(), "Updated") {
		t.Fatalf("printed success after a failed save: %q", buf.String())
	}
}

func TestConfigMutatorsReportAFailedSave(t *testing.T) {
	unwritableConfigPath(t)
	cases := map[string]func(cfg *Config, w *bytes.Buffer) error{
		"setPodcastsDir": func(cfg *Config, w *bytes.Buffer) error { return setPodcastsDir(w, cfg, "/tmp/x") },
		"addWhisperProfile": func(cfg *Config, w *bytes.Buffer) error {
			return addWhisperProfile(w, cfg, "local|http://localhost:9000")
		},
		"setDefaultWhisperProfile": func(cfg *Config, w *bytes.Buffer) error {
			return setDefaultWhisperProfile(w, cfg, 0)
		},
		"processorDel": func(cfg *Config, w *bytes.Buffer) error {
			cfg.PostProcessors = []string{"/bin/true"}
			return handleConfigProcessor(w, cfg, "del", "1")
		},
	}
	for name, run := range cases {
		var buf bytes.Buffer
		var cfg Config
		if err := run(&cfg, &buf); err == nil {
			t.Errorf("%s: returned nil although the config could not be written; output %q", name, buf.String())
		}
	}
}
