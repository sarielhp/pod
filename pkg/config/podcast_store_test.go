package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePodcastConfigReplacesAReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, PodcastConfigFileName)
	if err := os.WriteFile(path, []byte(`{"id":"old"}`), 0444); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultPodcastConfig(nil)
	cfg.ID = "new"
	if err := SavePodcastConfig(dir, cfg); err != nil {
		t.Fatalf("save over a read-only podcast.json (the atomic rename path) failed: %v", err)
	}
	got := LoadPodcastConfig(dir, PodcastConfig{})
	if got.ID != "new" {
		t.Fatalf("loaded ID = %q, want new", got.ID)
	}
}

func TestSavePodcastConfigPreservesACorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, PodcastConfigFileName)
	corrupt := []byte(`{"id":"keep-me", "keep_policy": "always"`)
	if err := os.WriteFile(path, corrupt, 0644); err != nil {
		t.Fatal(err)
	}
	if err := SavePodcastConfig(dir, DefaultPodcastConfig(nil)); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), PodcastConfigFileName+".corrupt-") {
			backups = append(backups, e.Name())
		}
	}
	if len(backups) != 1 {
		t.Fatalf("corrupt podcast.json was overwritten with no backup; dir holds %v", names(entries))
	}
	data, _ := os.ReadFile(filepath.Join(dir, backups[0]))
	if string(data) != string(corrupt) {
		t.Fatalf("backup content differs from the corrupt original")
	}
	if _, err := LoadPodcastConfigErr(dir, PodcastConfig{}); err != nil {
		t.Fatalf("the rewritten podcast.json does not load: %v", err)
	}
}

func TestLoadPodcastConfigErrDistinguishesMissingFromCorrupt(t *testing.T) {
	dir := t.TempDir()
	def := DefaultPodcastConfig(nil)
	def.ID = "default"
	got, err := LoadPodcastConfigErr(dir, def)
	if err != nil {
		t.Fatalf("missing file must load defaults without error, got %v", err)
	}
	if got.ID != "default" {
		t.Fatalf("missing file returned ID %q", got.ID)
	}
	if err := os.WriteFile(filepath.Join(dir, PodcastConfigFileName), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPodcastConfigErr(dir, def); err == nil {
		t.Fatal("corrupt file loaded without error")
	}
	valid, _ := json.Marshal(PodcastConfig{ID: "real"})
	if err := os.WriteFile(filepath.Join(dir, PodcastConfigFileName), valid, 0644); err != nil {
		t.Fatal(err)
	}
	got, err = LoadPodcastConfigErr(dir, def)
	if err != nil || got.ID != "real" {
		t.Fatalf("valid file: ID %q err %v", got.ID, err)
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
