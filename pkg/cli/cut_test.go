package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func writeTestMP3(t *testing.T, path string, seconds int) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-t", strconv.Itoa(seconds), "-c:a", "libmp3lame", "-b:a", "64k", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test mp3: %v: %s", err, out)
	}
}

func TestCutCommandMissingArgs(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	cli := CLIOptions{}
	err := runCutCommand(cfg, cli)
	if err == nil {
		t.Error("expected error with no arguments to cut command")
	}
}

func TestCutCommandDryRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mp3Path := filepath.Join(dir, "show.mp3")
	writeTestMP3(t, mp3Path, 10)

	cutsJSON := `{"version":1,"target_file":"show.mp3","original_duration_sec":10,
"cut_intervals":[{"start_sec":1,"end_sec":3,"reason":"ad"}]}`
	if err := os.WriteFile(filepath.Join(dir, "show.cuts.json"), []byte(cutsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cli := CLIOptions{
		Args: []string{mp3Path},
		ProcOptions: ProcOptions{
			DryRun: true,
			Quiet:  true,
		},
	}
	cfg := Config{}
	if err := runCutCommand(cfg, cli); err != nil {
		t.Fatalf("runCutCommand dry run failed: %v", err)
	}
}
