package audio

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeToneMP3(t *testing.T, seconds int) string {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
	path := filepath.Join(t.TempDir(), "tone.mp3")
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=440:sample_rate=44100:duration="+strconv.Itoa(seconds),
		"-c:a", "libmp3lame", "-b:a", "64k", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate tone: %v: %s", err, out)
	}
	return path
}

func probeBitrate(t *testing.T, path string) float64 {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=bit_rate",
		"-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		t.Fatalf("ffprobe bitrate: %v", err)
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatalf("parse bitrate %q: %v", out, err)
	}
	return rate
}

func scratchDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".work")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

var testKeep = [][2]float64{{0, 8}, {12, 20}, {26, 30}}

const testKeptSeconds = 20.0

func assertLosslessCut(t *testing.T, output string) {
	t.Helper()
	if got := GetAudioDuration(output); math.Abs(got-testKeptSeconds) > 0.8 {
		t.Fatalf("cut is %.2fs, want about %.0fs", got, testKeptSeconds)
	}
	if rate := probeBitrate(t, output); rate > 90000 {
		t.Fatalf("cut has bitrate %.0f, want the source's 64k: the audio was re-encoded", rate)
	}
}

func TestCutKeepsTheSourceBitrate(t *testing.T) {
	src := writeToneMP3(t, 30)
	out := filepath.Join(t.TempDir(), "cut.mp3")
	if err := NewFFmpegProcessor().Cut(context.Background(), src, testKeep, out); err != nil {
		t.Fatal(err)
	}
	assertLosslessCut(t, out)
}

func TestCutStreamCopyIsLossless(t *testing.T) {
	src := writeToneMP3(t, 30)
	out := filepath.Join(t.TempDir(), "cut.mp3")
	if !cutStreamCopy(src, testKeep, out, scratchDir(t)) {
		t.Fatal("stream copy cut failed")
	}
	assertLosslessCut(t, out)
}

func TestCutMp3spltIsLossless(t *testing.T) {
	if !hasMp3splt() {
		t.Skip("mp3splt not installed")
	}
	src := writeToneMP3(t, 30)
	out := filepath.Join(t.TempDir(), "cut.mp3")
	if !cutMp3splt(src, testKeep, out, scratchDir(t), 30) {
		t.Fatal("mp3splt cut failed")
	}
	assertLosslessCut(t, out)
}

func TestLosslessCutRejectsWrongLength(t *testing.T) {
	src := writeToneMP3(t, 30)
	if cutLengthMatches(src, [][2]float64{{0, 10}}, 30) {
		t.Fatal("a 30s file must not pass for a 10s keep")
	}
	if !cutLengthMatches(src, [][2]float64{{0, 30}}, 30) {
		t.Fatal("a 30s file should pass for a 30s keep")
	}
}

func TestWriteConcatManifestEscapesQuotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.txt")
	if err := writeConcatManifest(path, []string{"/a/it's.mp3"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if want := "file '/a/it'\\''s.mp3'\n"; string(data) != want {
		t.Fatalf("manifest = %q, want %q", data, want)
	}
}
