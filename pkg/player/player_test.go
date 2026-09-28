package player

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/types"
)

func TestFormatPlayerTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   float64
		want string
	}{
		{0, "00:00"},
		{45, "00:45"},
		{125, "02:05"},
		{3665, "1:01:05"},
		{7320, "2:02:00"},
	}

	for _, tt := range tests {
		got := FormatPlayerTime(tt.in)
		if got != tt.want {
			t.Errorf("FormatPlayerTime(%.1f) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderProgressBar(t *testing.T) {
	t.Parallel()
	p := &AudioPlayer{
		Position: 60,
		Duration: 120,
	}

	bar := p.RenderProgressBar(40)
	if !strings.Contains(bar, "01:00 / 02:00") {
		t.Errorf("expected time string '01:00 / 02:00', got %q", bar)
	}
	if !strings.HasPrefix(bar, "[") {
		t.Errorf("expected progress bar to start with '[', got %q", bar)
	}
}

func TestRenderVolumeBar(t *testing.T) {
	t.Parallel()
	p := &AudioPlayer{
		Volume: 70,
		Muted:  false,
	}

	bar := p.RenderVolumeBar(20)
	if !strings.Contains(bar, "70%") {
		t.Errorf("expected volume bar to contain '70%%', got %q", bar)
	}

	p.Muted = true
	barMuted := p.RenderVolumeBar(20)
	if !strings.Contains(barMuted, "MUTED") {
		t.Errorf("expected volume bar to show MUTED, got %q", barMuted)
	}
}

func TestPlayerQueueManagement(t *testing.T) {
	p := &AudioPlayer{}
	track1 := types.PlayerTrack{Title: "Ep 1", Path: "ep1.mp3", Duration: 100}
	track2 := types.PlayerTrack{Title: "Ep 2", Path: "ep2.mp3", Duration: 200}
	track3 := types.PlayerTrack{Title: "Ep 3", Path: "ep3.mp3", Duration: 300}

	p.IsPlaying = true
	p.Current = &track1
	ok1 := p.EnqueueAndPlay(track2)
	ok2 := p.EnqueueAndPlay(track3)
	if !ok1 || !ok2 {
		t.Error("expected tracks to be added")
	}

	dup1 := p.EnqueueAndPlay(track1)
	dup2 := p.EnqueueAndPlay(track2)
	if dup1 || dup2 {
		t.Error("expected duplicate tracks to be rejected")
	}

	if len(p.Queue) != 2 {
		t.Fatalf("expected 2 items in queue, got %d", len(p.Queue))
	}
	if p.Queue[0].Title != "Ep 2" || p.Queue[1].Title != "Ep 3" {
		t.Errorf("unexpected queue order: %+v", p.Queue)
	}

	p.ClearQueue()
	if len(p.Queue) != 0 {
		t.Errorf("expected empty queue after ClearQueue, got %d", len(p.Queue))
	}
}

func TestPlayerSeekBoundaries(t *testing.T) {
	t.Parallel()
	track := types.PlayerTrack{Duration: 100}
	p := &AudioPlayer{
		Current:  &track,
		Position: 50,
		Duration: 100,
		IsPaused: true,
	}

	p.Seek(30)
	if p.Position != 80 {
		t.Errorf("expected position 80, got %.1f", p.Position)
	}

	p.Seek(50)
	if p.Position != 100 {
		t.Errorf("expected position clamped to 100, got %.1f", p.Position)
	}

	p.Seek(-200)
	if p.Position != 0 {
		t.Errorf("expected position clamped to 0, got %.1f", p.Position)
	}
}

func TestUnplayableTrackDoesNotDrainTheQueue(t *testing.T) {
	orig := playerSpawnAllowed()
	SetPlayerSpawnEnabled(true)
	t.Cleanup(func() { SetPlayerSpawnEnabled(orig) })

	missing := filepath.Join(t.TempDir(), "gone")
	p := &AudioPlayer{Volume: 70}
	p.Queue = []types.PlayerTrack{
		{Title: "Ep 2", Path: missing + "-2.mp3", Duration: 1800},
		{Title: "Ep 3", Path: missing + "-3.mp3", Duration: 1800},
		{Title: "Ep 4", Path: missing + "-4.mp3", Duration: 1800},
	}
	t.Cleanup(p.Stop)

	p.PlayTrack(types.PlayerTrack{Title: "Ep 1", Path: missing + "-1.mp3", Duration: 1800})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		playing := p.IsPlaying
		p.mu.Unlock()
		if !playing {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	p.mu.Lock()
	remaining := len(p.Queue)
	lastErr := p.LastError
	playing := p.IsPlaying
	p.mu.Unlock()

	if remaining != 3 {
		t.Errorf("queue was drained: %d of 3 tracks remain after one unplayable track", remaining)
	}
	if playing {
		t.Errorf("IsPlaying is still true after playback failed")
	}
	if lastErr == "" {
		t.Errorf("playback failed with no error recorded; the user is told nothing")
	}
}

func TestUnplayableTrackDoesNotPersistAnEmptiedQueue(t *testing.T) {
	orig := playerSpawnAllowed()
	SetPlayerSpawnEnabled(true)
	t.Cleanup(func() { SetPlayerSpawnEnabled(orig) })

	missing := filepath.Join(t.TempDir(), "gone")
	p := &AudioPlayer{Volume: 70}
	p.Queue = []types.PlayerTrack{
		{Title: "Ep 2", Path: missing + "-2.mp3", Duration: 1800},
		{Title: "Ep 3", Path: missing + "-3.mp3", Duration: 1800},
	}
	t.Cleanup(p.Stop)

	p.PlayTrack(types.PlayerTrack{Title: "Ep 1", Path: missing + "-1.mp3", Duration: 1800})
	time.Sleep(600 * time.Millisecond)

	data, err := os.ReadFile(GetPlayQueueFilePath())
	if err != nil {
		t.Fatalf("no play queue file was written: %v", err)
	}
	var persisted types.PlayQueuePersist
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("play_queue.json is not valid JSON: %v", err)
	}
	if len(persisted.Queue) != 2 {
		t.Errorf("persisted queue holds %d tracks, want 2; the drain was written to disk",
			len(persisted.Queue))
	}
}

func TestPlayQueuePathIsIsolatedFromTheRealHome(t *testing.T) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		t.Fatal("XDG_CONFIG_HOME is unset: the test binary is not isolated from ~/.config")
	}
	got := GetPlayQueueFilePath()
	if !strings.HasPrefix(got, configHome+string(filepath.Separator)) {
		t.Fatalf("play queue path %q is outside the isolated config home %q", got, configHome)
	}
}
