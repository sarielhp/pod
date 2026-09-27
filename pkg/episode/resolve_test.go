package episode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAudioFiles(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mp3Path := filepath.Join(tempDir, "ep.mp3")
	_ = os.WriteFile(mp3Path, []byte("audio"), 0644)

	mainMP3, precut, src := ResolveAudioFiles(mp3Path, false)
	if mainMP3 != mp3Path {
		t.Errorf("expected mainMP3 %q, got %q", mp3Path, mainMP3)
	}
	if precut != mp3Path+".precut" {
		t.Errorf("expected precut %q, got %q", mp3Path+".precut", precut)
	}
	if src != mp3Path {
		t.Errorf("expected src %q, got %q", mp3Path, src)
	}
}

func TestResolveOutputFile(t *testing.T) {
	t.Parallel()
	mainMP3 := "/podcasts/ep1.mp3"
	out := ResolveOutputFile(mainMP3, "", 1)
	if out != mainMP3 {
		t.Errorf("expected default to mainMP3, got %s", out)
	}

	custom := "/tmp/output.mp3"
	outCustom := ResolveOutputFile(mainMP3, custom, 1)
	if outCustom != custom {
		t.Errorf("expected custom output, got %s", outCustom)
	}
}
