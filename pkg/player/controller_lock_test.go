package player

import (
	"net"
	"sync"
	"testing"
	"time"

	"pod/pkg/types"
)

func stallingPlayerSocket(t *testing.T, path string) (closeAll func()) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
			go func(c net.Conn) {
				buf := make([]byte, 4096)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
					time.Sleep(3 * time.Second)
				}
			}(conn)
		}
	}()
	return func() {
		_ = l.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	}
}

func TestUpdatePositionDoesNotHoldTheLockAcrossASlowSocket(t *testing.T) {
	path := redirectPlayerSocket(t)
	closeAll := stallingPlayerSocket(t, path)
	orig := playerSpawnAllowed()
	SetPlayerSpawnEnabled(true)
	t.Cleanup(func() { SetPlayerSpawnEnabled(orig) })
	t.Setenv("ABS_NO_AUDIO", "")
	t.Setenv("ABS_PLAYER_DISABLED", "")

	p := &AudioPlayer{Volume: 70, IsPlaying: true, Current: &types.PlayerTrack{Title: "x"}}
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		p.UpdatePosition()
		close(done)
	}()
	<-started
	time.Sleep(150 * time.Millisecond)

	t0 := time.Now()
	p.mu.Lock()
	p.mu.Unlock()
	held := time.Since(t0)
	closeAll()
	<-done
	if held > 500*time.Millisecond {
		t.Fatalf("p.mu was held for %v while UpdatePosition waited on the socket; the TUI freezes for that long every tick", held)
	}
}
