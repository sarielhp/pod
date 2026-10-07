package cli

import (
	"errors"
	"os"
	"path/filepath"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"testing"
)

type flushTestBackend struct {
	backend.Backend
	settingsErr error
	disabled    bool
	active      []backend.ActiveDownload
}

func (b *flushTestBackend) UpdatePodcastSettings(_ string, download, _ bool, _ int) error {
	b.disabled = !download
	return b.settingsErr
}

func (b *flushTestBackend) ActiveDownloads(string) ([]backend.ActiveDownload, error) {
	return b.active, nil
}

func TestFlushPreservesTranscriptsAndDisablesDownloads(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"flush", "dry-run", "settings failure", "active download"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "show")
			files := []string{"episode.mp3", "episode.mp3.precut", "episode.precut.mp3", "nested/podcast.mp3", ".work/temp.mp3"}
			kept := []string{"episode.transcript.json", "episode.srt", "episode.txt", "episode.mp3.json"}
			for _, name := range append(append([]string{}, files...), kept...) {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			b := &flushTestBackend{}
			if mode == "settings failure" {
				b.settingsErr = errors.New("offline")
			}
			if mode == "active download" {
				b.active = []backend.ActiveDownload{{}}
			}
			q := podcast.NewDownloadQueue(filepath.Join(root, "downloads.json"))
			if err := q.Save(&podcast.DownloadQueuePersist{Items: []podcast.DownloadQueueItem{
				{ID: "selected", PodcastID: "123", Status: "queued"},
				{ID: "other", PodcastID: "456", Status: "queued"},
			}}); err != nil {
				t.Fatal(err)
			}
			item := backend.Podcast{ID: "123"}
			err := flushPodcastAudio(b, item, dir, CLIOptions{ProcOptions: ProcOptions{DryRun: mode == "dry-run", Quiet: true}}, q)
			if (err != nil) != (mode == "settings failure" || mode == "active download") {
				t.Fatalf("flush error: %v", err)
			}
			for _, name := range files {
				_, err := os.Stat(filepath.Join(dir, name))
				if os.IsNotExist(err) != (mode == "flush") {
					t.Fatalf("unexpected deletion of %s in %s: %v", name, mode, err)
				}
			}
			for _, name := range kept {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(data) != "original" {
					t.Fatalf("metadata changed: %s", name)
				}
			}
			if mode == "flush" {
				cfg := config.LoadPodcastConfig(dir, config.PodcastConfig{})
				if !b.disabled || cfg.IsAutoDownloadEnabled() {
					t.Fatal("downloads still enabled")
				}
				if items := q.Items(); len(items) != 1 || items[0].ID != "other" {
					t.Fatalf("download queue = %v", items)
				}
			}
		})
	}
}

func TestFlushRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, path := range []string{".", "..", "../outside", "/tmp"} {
		if _, err := flushPodcastDir(root, path); err == nil {
			t.Fatalf("accepted unsafe target %q", path)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "show"), 0755); err != nil {
		t.Fatal(err)
	}
	item := backend.Podcast{ID: "123", RelPath: "show"}
	for _, items := range [][]backend.Podcast{nil, {item, item}} {
		if _, _, err := resolveFlushPodcast(root, "123", items); err == nil {
			t.Fatal("accepted missing/ambiguous target")
		}
	}
}

func TestFlushCommandParsing(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	if err := buildCLIApp(&action, &opts).Execute([]string{"library", "flush", "123", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if action != "library" || opts.ServerSubcmd != "flush" || !opts.DryRun || len(opts.Args) != 1 {
		t.Fatalf("unexpected options: %+v", opts)
	}
}
