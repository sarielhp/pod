package player

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"pod/pkg/audio"
	"pod/pkg/types"
	"pod/pkg/util"
)

type AudioPlayer struct {
	mu             util.Mutex
	Current        *types.PlayerTrack
	Queue          []types.PlayerTrack
	IsPlaying      bool
	IsPaused       bool
	Position       float64
	Duration       float64
	cmd            *exec.Cmd
	startPlayTime  time.Time
	startOffsetSec float64
	Volume         int
	Muted          bool
	CurrentSpeaker string
	Sinks          []types.AudioSink
	LastError      string
	isStarting     bool
}

var globalPlayer = &AudioPlayer{
	Volume: 70,
}

// playerSpawnEnabled gates spawning a real audio process. Tests turn it off,
// and production code reads it from whatever goroutine is handling playback,
// so it needs a guard rather than being a bare package variable.
var (
	playerSpawnMu      util.Mutex
	playerSpawnEnabled = true
)

func GetGlobalPlayer() *AudioPlayer {
	return globalPlayer
}

func SetPlayerSpawnEnabled(enabled bool) {
	playerSpawnMu.Lock()
	defer playerSpawnMu.Unlock()
	playerSpawnEnabled = enabled
}

func playerSpawnAllowed() bool {
	playerSpawnMu.Lock()
	defer playerSpawnMu.Unlock()
	return playerSpawnEnabled
}

func IsAudioSpawnDisabled() bool {
	return !playerSpawnAllowed() || os.Getenv("ABS_NO_AUDIO") == "1" || os.Getenv("ABS_PLAYER_DISABLED") == "1"
}

func init() {
	globalPlayer.LoadQueueFromFile()
}

func (p *AudioPlayer) EnqueueAndPlay(track types.PlayerTrack) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Current != nil && p.Current.Path == track.Path {
		return false
	}

	for _, q := range p.Queue {
		if q.Path == track.Path {
			return false
		}
	}

	if p.IsPlaying && p.Current != nil {
		p.Queue = append(p.Queue, track)
		p.saveQueueLocked()
		return true
	}

	p.playTrackLocked(track)
	return true
}

func (p *AudioPlayer) PlayTrack(track types.PlayerTrack) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playTrackLocked(track)
}

func (p *AudioPlayer) playTrackLocked(track types.PlayerTrack) {
	p.stopLocked()
	p.Current = &track
	p.Duration = track.Duration
	if p.Duration <= 0 {
		p.Duration = audio.GetAudioDuration(track.Path)
	}
	p.Position = 0
	p.startOffsetSec = 0
	p.saveQueueLocked()
	p.startProcessLocked(0)
}

func (p *AudioPlayer) startProcessLocked(startSec float64) {
	if p.Current == nil || p.Current.Path == "" || p.isStarting {
		return
	}
	p.isStarting = true
	defer func() {
		p.isStarting = false
	}()

	p.killProcessLocked()

	if IsAudioSpawnDisabled() {
		p.IsPlaying = true
		p.IsPaused = false
		p.startPlayTime = time.Now()
		p.startOffsetSec = startSec
		return
	}

	startErr := StartPlayerTrack(p.Current.Path, p.Current.Title, p.Current.Podcast)
	if startErr != nil {
		p.cmd = nil
		p.IsPlaying = false
		p.IsPaused = false
		p.LastError = fmt.Sprintf("could not play %s", filepath.Base(p.Current.Path))
		return
	}

	p.LastError = ""
	p.IsPlaying = true
	p.IsPaused = false
	p.startPlayTime = time.Now()
	p.startOffsetSec = startSec

	go p.watchSocketPlayback(p.Current.Path)
}

func (p *AudioPlayer) watchSocketPlayback(path string) {
	for {
		time.Sleep(500 * time.Millisecond)
		p.mu.Lock()
		if p.Current == nil || p.Current.Path != path || !p.IsPlaying {
			p.mu.Unlock()
			return
		}
		if !IsPlayerSocketAlive() {
			p.IsPlaying = false
			p.IsPaused = false
			p.nextLocked()
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
	}
}

func (p *AudioPlayer) killProcessLocked() {
	if p.cmd != nil && p.cmd.Process != nil {
		if pgid, err := syscall.Getpgid(p.cmd.Process.Pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = p.cmd.Process.Kill()
		}
		p.cmd = nil
	}
}

func (p *AudioPlayer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *AudioPlayer) stopLocked() {
	p.killProcessLocked()
	if !IsAudioSpawnDisabled() {
		_ = StopPlayerSocket()
	}
	p.IsPlaying = false
	p.IsPaused = false
	p.Position = 0
	p.Current = nil
}

func (p *AudioPlayer) TogglePause() {
	st := pollSocketStatus()
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Current == nil {
		return
	}
	if !p.IsPlaying {
		p.startProcessLocked(p.Position)
		return
	}

	p.updatePositionLocked(st)

	if st != nil {
		paused, err := PausePlayerSocket()
		if err == nil {
			p.IsPaused = paused
			return
		}
	}

	if p.IsPaused {
		p.startProcessLocked(p.Position)
	} else {
		p.IsPaused = true
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(syscall.SIGSTOP)
		}
	}
}

func (p *AudioPlayer) Seek(deltaSec float64) {
	st := pollSocketStatus()
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Current == nil {
		return
	}
	p.updatePositionLocked(st)
	newPos := p.Position + deltaSec
	if newPos < 0 {
		newPos = 0
	}
	if p.Duration > 0 && newPos > p.Duration {
		newPos = p.Duration
	}
	p.Position = newPos
	if st != nil {
		_ = SeekPlayerSocket(deltaSec)
		return
	}
	if !p.IsPaused {
		p.startProcessLocked(newPos)
	}
}

func (p *AudioPlayer) Next() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextLocked()
}

func (p *AudioPlayer) nextLocked() {
	if len(p.Queue) > 0 {
		next := p.Queue[0]
		p.Queue = p.Queue[1:]
		p.playTrackLocked(next)
	} else {
		p.stopLocked()
	}
}

func (p *AudioPlayer) ClearQueue() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Queue = nil
	p.saveQueueLocked()
}

func (p *AudioPlayer) MoveTrack(from, to int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if from < 0 || from >= len(p.Queue) || to < 0 || to >= len(p.Queue) || from == to {
		return
	}
	track := p.Queue[from]
	p.Queue = append(p.Queue[:from], p.Queue[from+1:]...)
	p.Queue = append(p.Queue[:to], append([]types.PlayerTrack{track}, p.Queue[to:]...)...)
	p.saveQueueLocked()
}

func (p *AudioPlayer) RemoveTrack(idx int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if idx < 0 || idx >= len(p.Queue) {
		return
	}
	p.Queue = append(p.Queue[:idx], p.Queue[idx+1:]...)
	p.saveQueueLocked()
}

func (p *AudioPlayer) PlayQueueIndex(idx int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if idx < 0 || idx >= len(p.Queue) {
		return
	}
	track := p.Queue[idx]
	p.Queue = append(p.Queue[:idx], p.Queue[idx+1:]...)
	p.playTrackLocked(track)
}

// UpdatePosition is the TUI's 500 ms tick. The socket round-trips happen
// before p.mu is taken: each carries a one-second deadline, and holding the
// lock across them froze every other player call for seconds whenever the
// daemon stalled.
func (p *AudioPlayer) UpdatePosition() {
	st := pollSocketStatus()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updatePositionLocked(st)
}

func pollSocketStatus() *types.PlayerStatusDTO {
	if IsAudioSpawnDisabled() || !IsPlayerSocketAlive() {
		return nil
	}
	st, err := QueryPlayerStatus()
	if err != nil {
		return nil
	}
	return st
}

func (p *AudioPlayer) updatePositionLocked(st *types.PlayerStatusDTO) {
	if st != nil {
		p.Position = st.Position
		if st.Duration > 0 {
			p.Duration = st.Duration
		}
		p.IsPaused = st.IsPaused
		p.IsPlaying = st.IsRunning
		return
	}
	if p.IsPlaying && !p.IsPaused {
		elapsed := time.Since(p.startPlayTime).Seconds()
		p.Position = p.startOffsetSec + elapsed
		if p.Duration > 0 && p.Position > p.Duration {
			p.Position = p.Duration
		}
	}
}
