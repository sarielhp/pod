package adremoval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/pipeline"
	"pod/pkg/types"
)

func TestAuditRepairsAndQueuesDownloadedEpisodes(t *testing.T) {
	t.Parallel()
	for _, transcript := range []string{"missing", "", "{}", "invalid", `{"text":"short"}`} {
		t.Run(transcript, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dir := filepath.Join(root, "show", "episode")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "podcast.mp3")
			if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
				t.Fatal(err)
			}
			if transcript != "missing" {
				if err := os.WriteFile(filepath.Join(dir, "podcast.transcript.json"), []byte(transcript), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := types.Config{PodcastsDir: root}
			if err := RunTranscriptAudit(cfg, nil, types.ProcOptions{DryRun: true, Quiet: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(pipeline.StatusPathFor(path)); !os.IsNotExist(err) {
				t.Fatal("dry run wrote status")
			}
			for i := 0; i < 2; i++ {
				if err := RunTranscriptAudit(cfg, []string{root, path}, types.ProcOptions{Quiet: true}); err != nil {
					t.Fatal(err)
				}
			}
			st, err := pipeline.LoadEpisodeStatus(pipeline.StatusPathFor(path))
			if err != nil || st.Status != types.StateNeedsAdR {
				t.Fatalf("status = %+v, error = %v", st, err)
			}
			queue := pipeline.QueuedEpisodes(filepath.Join(root, "show"))
			if len(queue) != 1 || queue[0] != filepath.Join("episode", "podcast.mp3") {
				t.Fatalf("queue = %v", queue)
			}
		})
	}
}

func TestAuditReportsQueueErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "episode.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	queuePath := filepath.Join(root, "queue.json")
	before := []byte("invalid queue")
	if err := os.WriteFile(queuePath, before, 0644); err != nil {
		t.Fatal(err)
	}
	if err := RunTranscriptAudit(types.Config{PodcastsDir: root}, nil, types.ProcOptions{Quiet: true}); err == nil {
		t.Fatal("expected queue error")
	}
	after, err := os.ReadFile(queuePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("queue changed: %q, %v", after, err)
	}
}

func TestAuditFailedDetectionQueuesAndPreservesTranscript(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "episode.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	transcript := []byte(`{"text":"` + strings.Repeat("words ", 20) + `","ad_detection_successful":false}`)
	transPath := filepath.Join(root, "episode.transcript.json")
	if err := os.WriteFile(transPath, transcript, 0644); err != nil {
		t.Fatal(err)
	}
	if err := RunTranscriptAudit(types.Config{PodcastsDir: root}, nil, types.ProcOptions{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if queue := pipeline.QueuedEpisodes(root); len(queue) != 1 {
		t.Fatalf("queue = %v", queue)
	}
	after, err := os.ReadFile(transPath)
	if err != nil || !bytes.Equal(after, transcript) {
		t.Fatalf("valid transcript changed: %v", err)
	}
}

func TestStaleCleanAudioMessage(t *testing.T) {
	t.Parallel()
	clean := &types.EpisodeStatusFile{Status: types.StateDone, Cleaned: types.EpisodeAudioMeta{DurationSec: 90}}
	if got := staleCleanAudioMessage(clean, 120); got == "" {
		t.Fatal("expected original-length audio to be reported as uncut")
	}
	if got := staleCleanAudioMessage(clean, 91); got != "" {
		t.Fatalf("small duration difference reported as stale: %s", got)
	}
}

func TestAuditExcludesPrecutAndWorkFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	names := []string{
		"episode.mp3", "precut.mp3", "episode.precut.mp3", "episode.PRECUT.MP3",
		"episode.mp3.precut", ".work/temp.mp3", ".work/nested/episode.mp3",
		"show/.work/temp.mp3",
	}
	targets := []string{root, filepath.Join(root, ".work")}
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, path)
	}
	files := collectAudioFilesForAudit(targets)
	if len(files) != 1 || files[0] != filepath.Join(root, "episode.mp3") {
		t.Fatalf("audit files = %v, want only episode.mp3", files)
	}
}

func TestInvalidCleanStateMessageRequiresTranscript(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "episode.transcript.json")
	clean := &types.EpisodeStatusFile{Status: types.StateDone}
	if got := invalidCleanStateMessage(clean, 0, path); got == "" {
		t.Fatal("expected missing transcript to invalidate clean status")
	}
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if got := invalidCleanStateMessage(clean, 0, path); got == "" {
		t.Fatal("expected empty transcript to invalidate clean status")
	}
}

func TestInspectEpisodeTranscriptReportsCompletedEpisodeWithoutTranscript(t *testing.T) {
	t.Parallel()
	audioPath := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	st := pipeline.GetOrCreateEpisodeStatus(audioPath)
	st.Status = types.StateDone
	if err := pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(audioPath), st); err != nil {
		t.Fatal(err)
	}

	item := inspectEpisodeTranscript(audioPath, 0.15, 50)
	if item == nil || item.cleanStateMsg == "" {
		t.Fatalf("audit item = %+v, want invalid completed state", item)
	}
}

func TestInspectEpisodeTranscriptRejectsShortCompletedTranscript(t *testing.T) {
	t.Parallel()
	audioPath := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	st := pipeline.GetOrCreateEpisodeStatus(audioPath)
	st.Status = types.StateDone
	st.Original.DurationSec = 30
	if err := pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(audioPath), st); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(audioPath), "episode.transcript.json"), []byte(`{"text":"short"}`), 0644); err != nil {
		t.Fatal(err)
	}

	item := inspectEpisodeTranscript(audioPath, 0.15, 50)
	if item == nil || !item.isSuspicious {
		t.Fatalf("audit item = %+v, want short completed transcript to be suspicious", item)
	}
}

func TestReportAndHealInvalidCleanStateResetsCleanState(t *testing.T) {
	t.Parallel()
	audioPath := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	st := pipeline.GetOrCreateEpisodeStatus(audioPath)
	st.Status = types.StateDone
	st.Cleaned = types.EpisodeAudioMeta{DurationSec: 90, AdDurationSec: 30}
	if err := pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(audioPath), st); err != nil {
		t.Fatal(err)
	}

	if err := repairAuditedEpisode(&transcriptAuditItem{audioPath: audioPath, cleanStateMsg: "duration mismatch"}, types.Config{PodcastsDir: filepath.Dir(audioPath)}, false, nil); err != nil {
		t.Fatal(err)
	}
	healed, err := pipeline.LoadEpisodeStatus(pipeline.StatusPathFor(audioPath))
	if err != nil {
		t.Fatal(err)
	}
	if healed.Status != types.StateNeedsAdR || healed.Cleaned.DurationSec != 0 {
		t.Fatalf("healed status = %+v, want NeedAdR without clean metadata", healed)
	}
	queued := pipeline.QueuedEpisodes(filepath.Dir(audioPath))
	if len(queued) != 1 || queued[0] != filepath.Base(audioPath) {
		t.Fatalf("queued episodes = %v, want %s", queued, filepath.Base(audioPath))
	}
}
