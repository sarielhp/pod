package player

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fakePlayers(t *testing.T, names ...string) {
	t.Helper()
	bin := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

func TestSelectedBackendPrefersMpvThenFallsBackInOrder(t *testing.T) {
	fakePlayers(t, "mpg123", "cvlc", "mpv")
	if b, err := SelectedBackend(); err != nil || b.Name != "mpv" {
		t.Fatalf("got %+v, %v; want mpv", b, err)
	}
	fakePlayers(t, "mpg123", "ffplay")
	if b, err := SelectedBackend(); err != nil || b.Name != "ffplay" {
		t.Fatalf("got %+v, %v; want ffplay", b, err)
	}
	fakePlayers(t)
	if _, err := SelectedBackend(); !errors.Is(err, ErrNoBackend) {
		t.Fatalf("with no player on PATH got %v, want ErrNoBackend", err)
	}
}

func TestStartPlayerTrackFailsBeforeSpawningWhenNoPlayerExists(t *testing.T) {
	redirectPlayerSocket(t)
	fakePlayers(t)
	orig := playerSpawnAllowed()
	SetPlayerSpawnEnabled(true)
	t.Cleanup(func() { SetPlayerSpawnEnabled(orig) })
	t.Setenv("ABS_NO_AUDIO", "")
	t.Setenv("ABS_PLAYER_DISABLED", "")
	audio := filepath.Join(t.TempDir(), "ep.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := StartPlayerTrackWith(audio, "t", "p")
	if !errors.Is(err, ErrNoBackend) {
		t.Fatalf("got %v; want ErrNoBackend naming the players to install, not a spawn or a socket timeout", err)
	}
}
