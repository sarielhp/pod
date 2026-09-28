package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/audio"
	"pod/pkg/util"
)

type fakeCutter struct {
	err      error
	cutTo    string
	segments [][2]float64
}

func (f *fakeCutter) Duration(context.Context, string) (float64, error) {
	return 0, errors.New("not used")
}

func (f *fakeCutter) Truncate(context.Context, string, string, float64) error {
	return errors.New("not used")
}

func (f *fakeCutter) ExtractTags(context.Context, string) (map[string]string, error) {
	return nil, errors.New("not used")
}

func (f *fakeCutter) PreserveMetadata(context.Context, string, string) error {
	return errors.New("not used")
}

func (f *fakeCutter) Cut(_ context.Context, _ string, segs [][2]float64, out string) error {
	f.cutTo = out
	f.segments = segs
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(out, []byte("cut audio"), 0644)
}

func useFakeCutter(t *testing.T, f *fakeCutter) {
	t.Helper()
	old := audio.DefaultProcessor
	audio.DefaultProcessor = f
	t.Cleanup(func() { audio.DefaultProcessor = old })
}

func cutFixture(t *testing.T) (dir, mp3, precut string) {
	t.Helper()
	dir = t.TempDir()
	mp3 = filepath.Join(dir, "ep.mp3")
	precut = filepath.Join(dir, "ep.precut.mp3")
	if err := os.WriteFile(mp3, []byte("original audio"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, mp3, precut
}

func TestExecuteCutProcessingInstallsTheCutAndKeepsAPrecut(t *testing.T) {
	fake := &fakeCutter{}
	useFakeCutter(t, fake)
	_, mp3, precut := cutFixture(t)
	segs := [][2]float64{{0, 2}, {4, 10}}

	if err := executeCutProcessing(mp3, mp3, precut, mp3, segs); err != nil {
		t.Fatalf("executeCutProcessing: %v", err)
	}
	if !strings.Contains(fake.cutTo, "/.work/") {
		t.Fatalf("ffmpeg output must be staged in .work/, got %s", fake.cutTo)
	}
	if len(fake.segments) != 2 {
		t.Fatalf("keep segments not passed through: %v", fake.segments)
	}
	if data, _ := os.ReadFile(mp3); string(data) != "cut audio" {
		t.Fatalf("cut audio not installed over the original: %q", data)
	}
	if data, _ := os.ReadFile(precut); string(data) != "original audio" {
		t.Fatalf("precut must preserve the original: %q", data)
	}
	if _, err := os.Stat(util.WorkDirFor(mp3)); !os.IsNotExist(err) {
		t.Fatalf("the scratch dir must be removed after a successful cut: %v", err)
	}
}

func TestExecuteCutProcessingLeavesTheOriginalWhenTheCutFails(t *testing.T) {
	useFakeCutter(t, &fakeCutter{err: errors.New("ffmpeg exploded")})
	_, mp3, precut := cutFixture(t)

	err := executeCutProcessing(mp3, mp3, precut, mp3, [][2]float64{{0, 5}})
	if err == nil || !strings.Contains(err.Error(), "ffmpeg exploded") {
		t.Fatalf("the cutter's error must surface, got %v", err)
	}
	if data, _ := os.ReadFile(mp3); string(data) != "original audio" {
		t.Fatalf("a failed cut must not touch the original: %q", data)
	}
	if _, err := os.Stat(precut); !os.IsNotExist(err) {
		t.Fatalf("a failed cut must not leave a precut: %v", err)
	}
	if _, err := os.Stat(util.WorkDirFor(mp3)); !os.IsNotExist(err) {
		t.Fatalf("the scratch dir must be removed after a failed cut: %v", err)
	}
}
