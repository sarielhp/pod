package episode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/backend"
)

func TestEnsureABSIgnore(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	if err := EnsureABSIgnore(tempDir); err != nil {
		t.Fatalf("EnsureABSIgnore failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, ".absignore"))
	if err != nil {
		t.Fatalf("reading .absignore failed: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "*.precut") || !strings.Contains(content, "*.bak") {
		t.Errorf("expected .absignore to contain *.precut and *.bak, got %q", content)
	}
}

func TestQuarantineAbandonedDuplicates(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "Hard Fork")
	_ = os.MkdirAll(podDir, 0755)

	guidMP3 := filepath.Join(podDir, "OpenAI Pause (90b50030-4e0f-4e45-af9d-6).mp3")
	bareMP3 := filepath.Join(podDir, "OpenAI Pause.mp3")
	bareCuts := filepath.Join(podDir, "OpenAI Pause.cuts.json")
	bareTranscript := filepath.Join(podDir, "OpenAI Pause.transcript.json")
	barePrecut := filepath.Join(podDir, "OpenAI Pause.mp3.precut")

	_ = os.WriteFile(guidMP3, []byte("tracked audio"), 0644)
	_ = os.WriteFile(bareMP3, []byte("abandoned audio"), 0644)
	_ = os.WriteFile(bareCuts, []byte("{}"), 0644)
	_ = os.WriteFile(bareTranscript, []byte("{}"), 0644)
	_ = os.WriteFile(barePrecut, []byte("precut audio"), 0644)

	trackedEpisodes := []backend.Episode{
		{
			Title: "OpenAI Pause",
			AudioFile: &backend.PodcastAudioFile{
				Metadata: &backend.AudioFileMetadata{
					Filename: "OpenAI Pause (90b50030-4e0f-4e45-af9d-6).mp3",
				},
			},
		},
	}

	quarantined := QuarantineAbandonedDuplicates(podDir, trackedEpisodes)
	if len(quarantined) != 1 {
		t.Fatalf("expected 1 quarantined file, got %d (%v)", len(quarantined), quarantined)
	}
	if quarantined[0] != "OpenAI Pause.mp3" {
		t.Errorf("expected 'OpenAI Pause.mp3', got %q", quarantined[0])
	}

	if _, err := os.Stat(bareMP3); !os.IsNotExist(err) {
		t.Errorf("expected bare MP3 to no longer exist")
	}
	if _, err := os.Stat(bareMP3 + ".bak"); err != nil {
		t.Errorf("expected bare MP3 .bak to exist: %v", err)
	}
	if _, err := os.Stat(bareCuts + ".bak"); err != nil {
		t.Errorf("expected bare cuts .bak to exist: %v", err)
	}
	if _, err := os.Stat(bareTranscript + ".bak"); err != nil {
		t.Errorf("expected bare transcript .bak to exist: %v", err)
	}
	if _, err := os.Stat(barePrecut + ".bak"); err != nil {
		t.Errorf("expected bare precut .bak to exist: %v", err)
	}
}
