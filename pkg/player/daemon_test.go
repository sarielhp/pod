package player

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func redirectPlayerSocket(t *testing.T) string {
	t.Helper()
	orig := PlayerSocketPath
	PlayerSocketPath = filepath.Join(t.TempDir(), "p.sock")
	t.Cleanup(func() { PlayerSocketPath = orig })
	return PlayerSocketPath
}

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

func exitedWithin(cmd *exec.Cmd, d time.Duration) bool {
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func TestDaemonStopKillsTheProcessStartedBySeek(t *testing.T) {
	redirectPlayerSocket(t)
	original := startSleeper(t)
	afterSeek := startSleeper(t)
	_ = original.Process.Kill()
	_ = original.Wait()

	state := &daemonState{cmd: afterSeek}
	resp := ProcessDaemonCommand([]any{"stop"}, state, original)
	if resp.Error != "success" {
		t.Fatalf("stop returned %q", resp.Error)
	}
	if !exitedWithin(afterSeek, 2*time.Second) {
		t.Fatal("the process started by the seek is still playing after 'stop'; the daemon killed the stale original instead")
	}
}

func TestPlayerSocketIsOwnerOnly(t *testing.T) {
	path := redirectPlayerSocket(t)
	l, err := listenPlayerSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Fatalf("socket mode %o, want 0600", perm)
	}
}

func TestWaitForPlayerSocketReportsAPlayerThatNeverStarted(t *testing.T) {
	redirectPlayerSocket(t)
	err := waitForPlayerSocket(100*time.Millisecond, "/tmp/x.log")
	if err == nil {
		t.Fatal("no socket ever appeared, yet the start was reported as successful")
	}
	l, lerr := listenPlayerSocket(PlayerSocketPath)
	if lerr != nil {
		t.Fatal(lerr)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	if err := waitForPlayerSocket(time.Second, ""); err != nil {
		t.Fatalf("a live socket was reported as missing: %v", err)
	}
}
