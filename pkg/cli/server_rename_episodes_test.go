package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/backend"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

func renameShow(t *testing.T) (Config, string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Show")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(dir, "2026-09-28_Some_Long_Title.mp3")
	for path, body := range map[string]string{
		audio: "audio", strings.TrimSuffix(audio, ".mp3") + ".transcript.json": `{"text":"kept"}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fe := backend.FeedEpisode{GUID: "guid-1", Title: "Some Long Title", PublishedAt: 1_790_000_000_000, EnclosureURL: "https://x.test/a.mp3"}
	if err := podcast.RecordFeedEpisode(audio, fe, ""); err != nil {
		t.Fatal(err)
	}
	return Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}, dir, audio
}

func TestRenameEpisodesDryRunChangesNothing(t *testing.T) {
	cfg, _, audio := renameShow(t)
	var out bytes.Buffer
	cli := CLIOptions{Out: &out}
	cli.DryRun = true
	if err := handleServerRenameEpisodes(cfg, cli); err != nil {
		t.Fatal(err)
	}
	if !util.FileExists(audio) || !strings.Contains(out.String(), "1 episode(s) to rename") || !strings.Contains(out.String(), "[dry-run] Nothing was renamed") {
		t.Errorf("a dry run reports and changes nothing:\n%s", out.String())
	}
}

func TestRenameEpisodesBacksUpRenamesAndUndoes(t *testing.T) {
	cfg, dir, audio := renameShow(t)
	backup := filepath.Join(t.TempDir(), "backup")
	var out bytes.Buffer
	cli := CLIOptions{Out: &out, Err: &out}
	cli.ForceDelete = true
	cli.RenameBackup = backup
	if err := handleServerRenameEpisodes(cfg, cli); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if util.FileExists(audio) {
		t.Fatal("the audio file should have been renamed")
	}
	if !strings.Contains(out.String(), "Renamed 1 episode(s)") || !strings.Contains(out.String(), "--undo "+backup) {
		t.Errorf("the summary should say what happened and how to undo it:\n%s", out.String())
	}
	if !util.FileExists(filepath.Join(backup, "Show", "2026-09-28_Some_Long_Title.transcript.json")) {
		t.Error("the transcript must be in the backup, under its old name")
	}
	if got, _ := filepath.Glob(filepath.Join(dir, "*.transcript.json")); len(got) != 1 || strings.Contains(got[0], "Long_Title") {
		t.Errorf("the transcript should have moved with the audio, found %v", got)
	}

	undo := CLIOptions{Out: &out}
	undo.RenameUndo = backup
	if err := handleServerRenameEpisodes(cfg, undo); err != nil {
		t.Fatal(err)
	}
	if !util.FileExists(audio) {
		t.Error("undo should put the audio back under its old name")
	}
}
