package audio

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeSilentWav(t *testing.T, seconds int) string {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	const rate = 8000
	data := make([]byte, seconds*rate*2)
	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(data)))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], 1)
	binary.LittleEndian.PutUint32(header[24:], rate)
	binary.LittleEndian.PutUint32(header[28:], rate*2)
	binary.LittleEndian.PutUint16(header[32:], 2)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(data)))
	path := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(path, append(header, data...), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckKeepFractionExplainsTheRefusal(t *testing.T) {
	path := writeSilentWav(t, 20)
	if err := CheckKeepFraction(path, [][2]float64{{0, 15}}); err != nil {
		t.Fatalf("a cut keeping 75%% is plausible: %v", err)
	}
	err := CheckKeepFraction(path, [][2]float64{{0, 1}})
	if err == nil || !strings.Contains(err.Error(), "refusing to cut") || !strings.Contains(err.Error(), "5.0%") {
		t.Fatalf("expected a refusal that states the kept fraction, got %v", err)
	}
}

func TestCutReportsWhyItRefused(t *testing.T) {
	path := writeSilentWav(t, 20)
	err := NewFFmpegProcessor().Cut(context.Background(), path, [][2]float64{{0, 1}}, filepath.Join(t.TempDir(), "out.wav"))
	if err == nil || !strings.Contains(err.Error(), "implausible timestamps") {
		t.Fatalf("Cut should carry the refusal reason, got %v", err)
	}
}
