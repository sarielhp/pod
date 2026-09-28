package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"pod/pkg/podcast/podtest"
	"pod/pkg/types"
)

func writeTestMP3(t *testing.T, path string, seconds int) {
	t.Helper()
	podtest.RequireFFmpeg(t)
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-t", strconv.Itoa(seconds), "-c:a", "libmp3lame", "-b:a", "64k", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test mp3: %v: %s", err, out)
	}
}

func TestCutFileRejectsMissingAndDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if _, err := CutFile(CutRequest{Path: filepath.Join(dir, "absent.mp3")},
		types.Config{}, types.ProcOptions{Quiet: true}, nil); err == nil {
		t.Error("a missing file should be an error")
	}
	if _, err := CutFile(CutRequest{Path: dir},
		types.Config{}, types.ProcOptions{Quiet: true}, nil); err == nil {
		t.Error("a directory should be an error")
	}
}

func TestCutFileRejectsMissingCutsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3Path := filepath.Join(dir, "ep.mp3")
	writeTestMP3(t, mp3Path, 10)

	if _, err := CutFile(CutRequest{Path: mp3Path},
		types.Config{}, types.ProcOptions{Quiet: true}, nil); err == nil {
		t.Error("expected error when .cuts.json is missing")
	}
}

func TestCutFileDryRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3Path := filepath.Join(dir, "ep.mp3")
	writeTestMP3(t, mp3Path, 10)

	cutsJSON := `{"version":1,"target_file":"ep.mp3","original_duration_sec":10,
"cut_intervals":[{"start_sec":2,"end_sec":4,"reason":"ad"}]}`
	cutsPath := filepath.Join(dir, "ep.cuts.json")
	if err := os.WriteFile(cutsPath, []byte(cutsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := CutFile(CutRequest{Path: mp3Path, DryRun: true},
		types.Config{}, types.ProcOptions{Quiet: true}, nil)
	if err != nil {
		t.Fatalf("CutFile dry run failed: %v", err)
	}

	if res.OriginalSec != 10 {
		t.Errorf("res.OriginalSec = %v, want 10", res.OriginalSec)
	}
	if res.CutSec != 2 {
		t.Errorf("res.CutSec = %v, want 2", res.CutSec)
	}
	if res.SegmentsCut != 1 {
		t.Errorf("res.SegmentsCut = %v, want 1", res.SegmentsCut)
	}
}
