package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/util"
)

func makePruneShow(t *testing.T, root, name string, episodes int, favorite bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < episodes; i++ {
		age := now.Add(-time.Duration(i) * time.Hour)
		for _, file := range []string{fmt.Sprintf("ep%d.mp3", i), fmt.Sprintf("ep%d.transcript.json", i)} {
			path := filepath.Join(dir, file)
			if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
				t.Fatal(err)
			}
			_ = os.Chtimes(path, age, age)
		}
	}
	pc := config.PodcastConfig{}
	if favorite {
		pc.SetFavorite(true)
	}
	if err := config.SavePodcastConfig(dir, pc); err != nil {
		t.Fatal(err)
	}
	return dir
}

func pruneCLI(keep int, out *bytes.Buffer) CLIOptions {
	cli := CLIOptions{Out: out, Err: out}
	cli.KeepCount = &keep
	return cli
}

func TestPruneCountCoversTheWholeLibraryNotJustOnePodcast(t *testing.T) {
	root := t.TempDir()
	dirs := []string{
		makePruneShow(t, root, "Alpha", 7, false),
		makePruneShow(t, root, "Bravo", 7, false),
		makePruneShow(t, root, "Charlie", 7, false),
		makePruneShow(t, root, "Delta", 7, false),
		makePruneShow(t, root, "Echo", 7, false),
		makePruneShow(t, root, "Foxtrot", 7, false),
	}
	var out bytes.Buffer
	cli := pruneCLI(5, &out)
	cli.ForceDelete = true
	cli.Args = []string{"5"}
	if err := handleServerKeep(Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}, cli); err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		if util.FileExists(filepath.Join(dir, "ep6.mp3")) || !util.FileExists(filepath.Join(dir, "ep4.mp3")) {
			t.Errorf("%s: the two oldest should be pruned and the newest five kept", filepath.Base(dir))
		}
		if !util.FileExists(filepath.Join(dir, "ep6.transcript.json")) {
			t.Errorf("%s: transcripts must be kept", filepath.Base(dir))
		}
	}
	if !strings.Contains(out.String(), "Deleted 12 episode(s) from 6 podcast(s)") {
		t.Errorf("summary missing:\n%s", out.String())
	}
}

func TestPruneCountSkipsFavoritesAndDryRunDeletesNothing(t *testing.T) {
	root := t.TempDir()
	plain := makePruneShow(t, root, "Plain", 7, false)
	loved := makePruneShow(t, root, "Loved", 7, true)
	lib := Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}

	var out bytes.Buffer
	dry := pruneCLI(5, &out)
	dry.DryRun, dry.SkipFavorites = true, true
	if err := handleServerKeep(lib, dry); err != nil {
		t.Fatal(err)
	}
	if !util.FileExists(filepath.Join(plain, "ep6.mp3")) || !strings.Contains(out.String(), "[dry-run] Nothing was deleted") || !strings.Contains(out.String(), "1 favorite(s) left alone") {
		t.Errorf("a dry run must delete nothing and say what it spared:\n%s", out.String())
	}

	real := pruneCLI(5, &out)
	real.SkipFavorites, real.ForceDelete = true, true
	if err := handleServerKeep(lib, real); err != nil {
		t.Fatal(err)
	}
	if util.FileExists(filepath.Join(plain, "ep6.mp3")) || !util.FileExists(filepath.Join(loved, "ep6.mp3")) {
		t.Error("the plain podcast is pruned and the favorite is spared")
	}
}

func TestPruneCountAsksBeforeDeleting(t *testing.T) {
	for answer, deleted := range map[string]bool{"n\n": false, "": false, "y\n": true} {
		root := t.TempDir()
		dir := makePruneShow(t, root, "Show", 7, false)
		var out bytes.Buffer
		cli := pruneCLI(5, &out)
		cli.In = strings.NewReader(answer)
		if err := handleServerKeep(Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}, cli); err != nil {
			t.Fatal(err)
		}
		if gone := !util.FileExists(filepath.Join(dir, "ep6.mp3")); gone != deleted {
			t.Errorf("answer %q: deleted=%v, want %v", answer, gone, deleted)
		}
	}
}

func TestServerStatusReportsUsageAndJSON(t *testing.T) {
	root := t.TempDir()
	makePruneShow(t, root, "Alpha", 7, false)
	makePruneShow(t, root, "Loved", 3, true)
	lib := Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}

	var out bytes.Buffer
	cli := CLIOptions{Out: &out}
	cli.StatusTop = 5
	if err := handleServerStatus(lib, cli); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Podcasts     2 (1 favorite)", "10 on disk", "Disk use", "Largest podcasts", "pod server prune 5 --skip-favorites --dry-run"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status output missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	cli.JSON = true
	if err := handleServerStatus(lib, cli); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\"episodes\": 10") {
		t.Errorf("JSON output should carry the episode count:\n%s", out.String())
	}
}
