package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pod/pkg/audio"
	"pod/pkg/types"
	"pod/pkg/util"
)

const LegacyPlayerSocketPath = "/tmp/abs_player.sock"

// PlayerSocketPath is the daemon's control socket. It is a variable so tests
// can point it into a temporary directory instead of the real /tmp path.
var PlayerSocketPath = "/tmp/pod_player.sock"

type mpvCommand struct {
	Command []any `json:"command"`
}

type mpvResponse struct {
	Data  any    `json:"data"`
	Error string `json:"error"`
}

type daemonState struct {
	mu       util.Mutex
	path     string
	title    string
	podcast  string
	paused   bool
	start    time.Time
	offset   float64
	duration float64
	volume   float64
	cmd      *exec.Cmd
}

func DialPlayerSocket() (net.Conn, error) {
	conn, err := net.DialTimeout("unix", PlayerSocketPath, 250*time.Millisecond)
	if err == nil {
		return conn, nil
	}
	return net.DialTimeout("unix", LegacyPlayerSocketPath, 250*time.Millisecond)
}

func IsPlayerSocketAlive() bool {
	conn, err := DialPlayerSocket()
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func SendPlayerIpcCommand(cmd []any) (any, error) {
	conn, err := DialPlayerSocket()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return SendMpvRawCommand(conn, cmd)
}

func SendMpvRawCommand(conn net.Conn, cmd []any) (any, error) {
	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))
	payload, err := json.Marshal(mpvCommand{Command: cmd})
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	if _, err := conn.Write(payload); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		var resp mpvResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		if resp.Error != "" && resp.Error != "success" {
			return nil, fmt.Errorf("player error: %s", resp.Error)
		}
		return resp.Data, nil
	}
}

func StopPlayerSocket() error {
	if !IsPlayerSocketAlive() {
		_ = os.Remove(PlayerSocketPath)
		_ = os.Remove(LegacyPlayerSocketPath)
		return nil
	}
	_, err := SendPlayerIpcCommand([]any{"quit"})
	time.Sleep(100 * time.Millisecond)
	_ = os.Remove(PlayerSocketPath)
	_ = os.Remove(LegacyPlayerSocketPath)
	return err
}

func PausePlayerSocket() (bool, error) {
	res, err := SendPlayerIpcCommand([]any{"cycle", "pause"})
	if err != nil {
		return false, err
	}
	if b, ok := res.(bool); ok {
		return b, nil
	}
	st, err := QueryPlayerStatus()
	if err == nil && st != nil {
		return st.IsPaused, nil
	}
	return false, nil
}

func ResumePlayerSocket() error {
	_, err := SendPlayerIpcCommand([]any{"set_property", "pause", false})
	return err
}

func SeekPlayerSocket(deltaSec float64) error {
	_, err := SendPlayerIpcCommand([]any{"seek", deltaSec, "relative"})
	return err
}

func LoadfilePlayerSocket(path string) error {
	_, err := SendPlayerIpcCommand([]any{"loadfile", path, "replace"})
	return err
}

func QueryPlayerStatus() (*types.PlayerStatusDTO, error) {
	conn, err := DialPlayerSocket()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	titleData, _ := SendMpvRawCommand(conn, []any{"get_property", "media-title"})
	pauseData, _ := SendMpvRawCommand(conn, []any{"get_property", "pause"})
	timeData, _ := SendMpvRawCommand(conn, []any{"get_property", "time-pos"})
	durData, _ := SendMpvRawCommand(conn, []any{"get_property", "duration"})

	title, _ := titleData.(string)
	paused, _ := pauseData.(bool)
	pos, _ := timeData.(float64)
	dur, _ := durData.(float64)

	volData, _ := SendMpvRawCommand(conn, []any{"get_property", "volume"})
	vol := 100.0
	if v, ok := volData.(float64); ok && v > 0 {
		vol = v
	}

	return &types.PlayerStatusDTO{
		IsRunning: true,
		IsPaused:  paused,
		Title:     title,
		Position:  pos,
		Duration:  dur,
		Volume:    int(vol),
	}, nil
}

func StartPlayerTrack(audioPath, title, podcast string) error {
	_, err := StartPlayerTrackWith(audioPath, title, podcast)
	return err
}

// StartPlayerTrackWith starts playback and reports which Backend is doing it,
// so the caller can tell the user when the better player is missing. With no
// supported player on PATH it fails before spawning anything.
func StartPlayerTrackWith(audioPath, title, podcast string) (Backend, error) {
	if IsAudioSpawnDisabled() {
		return Backend{}, nil
	}

	if _, err := os.Stat(audioPath); err != nil {
		return Backend{}, err
	}

	backend, err := SelectedBackend()
	if err != nil {
		return Backend{}, err
	}

	if IsPlayerSocketAlive() {
		if err := LoadfilePlayerSocket(audioPath); err == nil {
			_ = ResumePlayerSocket()
			return backend, nil
		}
		_ = StopPlayerSocket()
	}

	_ = os.Remove(PlayerSocketPath)

	if backend.Name == "mpv" {
		return backend, SpawnDetachedMpv(audioPath, title)
	}
	return backend, SpawnDetachedDaemon(audioPath, title, podcast)
}

func SpawnDetachedMpv(audioPath, title string) error {
	args := []string{
		"--no-video",
		"--no-terminal",
		fmt.Sprintf("--input-ipc-server=%s", PlayerSocketPath),
	}
	if script := FindMprisScript(); script != "" {
		args = append(args, fmt.Sprintf("--script=%s", script))
	}
	if title != "" {
		args = append(args, fmt.Sprintf("--force-media-title=%s", title))
	}
	args = append(args, audioPath)

	cmd := exec.Command("mpv", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	logFile := attachPlayerLog(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return waitForPlayerSocket(playerStartTimeout, logFile)
}

const playerStartTimeout = 3 * time.Second

// waitForPlayerSocket polls until the control socket answers. A player that
// never opens it has died, and saying so beats reporting a start that did
// not happen; the log path is where its stderr went.
func waitForPlayerSocket(timeout time.Duration, logFile string) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsPlayerSocketAlive() {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	if logFile != "" {
		return fmt.Errorf("player did not open %s within %s; see %s", PlayerSocketPath, timeout, logFile)
	}
	return fmt.Errorf("player did not open %s within %s", PlayerSocketPath, timeout)
}

// PlayerLogPath is where a detached player's stdout and stderr are appended,
// so a player that dies on startup leaves its reason somewhere.
func PlayerLogPath() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "pod", "player.log")
}

func attachPlayerLog(cmd *exec.Cmd) string {
	path := PlayerLogPath()
	if path == "" {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return ""
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return ""
	}
	fmt.Fprintf(f, "\n=== %s %s\n", time.Now().Format(time.RFC3339), strings.Join(cmd.Args, " "))
	cmd.Stdout = f
	cmd.Stderr = f
	return path
}

func FindMprisScript() string {
	candidates := []string{
		"/usr/lib/mpv/mpris.so",
		"/usr/lib64/mpv/mpris.so",
		"/usr/lib/x86_64-linux-gnu/mpv/mpris.so",
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".config/mpv/scripts/mpris.so"),
			filepath.Join(home, ".local/share/mpv/scripts/mpris.so"),
		)
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

func SpawnDetachedDaemon(audioPath, title, podcast string) error {
	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}
	args := []string{"player", "daemon", audioPath, "--title", title, "--podcast", podcast}
	cmd := exec.Command(execPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	logFile := attachPlayerLog(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return waitForPlayerSocket(playerStartTimeout, logFile)
}

// listenPlayerSocket opens the control socket and restricts it to its owner.
// /tmp is shared, and a socket created under a permissive umask would let any
// local user stop, seek, or query playback.
func listenPlayerSocket(path string) (net.Listener, error) {
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return listener, nil
}

func RunPlayerDaemon(audioPath, title, podcast string) error {
	listener, err := listenPlayerSocket(PlayerSocketPath)
	if err != nil {
		return err
	}

	dur := audio.GetAudioDuration(audioPath)
	state := &daemonState{
		path:     audioPath,
		title:    title,
		podcast:  podcast,
		duration: dur,
		start:    time.Now(),
		volume:   100,
	}

	cmd, err := StartDaemonPlayback(audioPath)
	if err != nil {
		_ = listener.Close()
		_ = os.Remove(PlayerSocketPath)
		return err
	}
	state.cmd = cmd

	defer func() {
		_ = listener.Close()
		_ = os.Remove(PlayerSocketPath)
		state.mu.Lock()
		if state.cmd != nil && state.cmd.Process != nil {
			_ = state.cmd.Process.Signal(syscall.SIGCONT)
			_ = state.cmd.Process.Kill()
		}
		state.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		for {
			state.mu.Lock()
			current := state.cmd
			state.mu.Unlock()
			if current == nil || current.Process == nil {
				break
			}
			_ = current.Wait()
			state.mu.Lock()
			same := (state.cmd == current)
			state.mu.Unlock()
			if same {
				break
			}
		}
		close(done)
	}()

	go HandleDaemonIpc(listener, state, cmd)

	<-done
	return nil
}

func StartDaemonPlayback(audioPath string) (*exec.Cmd, error) {
	return StartDaemonPlaybackAt(audioPath, 0)
}

func StartDaemonPlaybackAt(audioPath string, offset float64) (*exec.Cmd, error) {
	if _, err := exec.LookPath("cvlc"); err == nil {
		args := []string{"--no-video", "--intf", "dummy", "--control", "dbus"}
		if offset > 0 {
			args = append(args, fmt.Sprintf("--start-time=%.2f", offset))
		}
		args = append(args, audioPath)
		cmd := exec.Command("cvlc", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}
	if _, err := exec.LookPath("ffplay"); err == nil {
		args := []string{"-nodisp", "-autoexit", "-loglevel", "quiet"}
		if offset > 0 {
			args = append(args, "-ss", fmt.Sprintf("%.2f", offset))
		}
		args = append(args, audioPath)
		cmd := exec.Command("ffplay", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}
	args := []string{"-q"}
	if offset > 0 {
		args = append(args, "-k", strconv.Itoa(int(offset*38.28)))
	}
	args = append(args, audioPath)
	cmd := exec.Command("mpg123", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func HandleDaemonIpc(listener net.Listener, state *daemonState, cmd *exec.Cmd) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go HandleDaemonConn(conn, state, cmd)
	}
}

func HandleDaemonConn(conn net.Conn, state *daemonState, cmd *exec.Cmd) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var req mpvCommand
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &req); err != nil || len(req.Command) == 0 {
			continue
		}
		resp := ProcessDaemonCommand(req.Command, state, cmd)
		respBytes, _ := json.Marshal(resp)
		respBytes = append(respBytes, '\n')
		_, _ = conn.Write(respBytes)
	}
}

func ProcessDaemonCommand(args []any, state *daemonState, cmd *exec.Cmd) mpvResponse {
	if len(args) == 0 {
		return mpvResponse{Error: "success"}
	}
	cname, _ := args[0].(string)
	switch cname {
	case "quit", "stop":
		if active := activeDaemonCmd(state, cmd); active != nil && active.Process != nil {
			_ = active.Process.Kill()
		}
		_ = os.Remove(PlayerSocketPath)
		return mpvResponse{Error: "success"}
	case "cycle":
		return HandleDaemonCycle(args, state, cmd)
	case "set_property":
		return HandleDaemonSetProp(args, state, cmd)
	case "get_property":
		return HandleDaemonGetProp(args, state)
	case "seek":
		return HandleDaemonSeek(args, state)
	default:
		return mpvResponse{Error: "success"}
	}
}

// activeDaemonCmd is the process currently playing: a seek replaces the one
// RunPlayerDaemon started, so the *exec.Cmd threaded through the IPC handlers
// is only a fallback for the moment before state.cmd is set.
func activeDaemonCmd(state *daemonState, fallback *exec.Cmd) *exec.Cmd {
	if state == nil {
		return fallback
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.cmd != nil {
		return state.cmd
	}
	return fallback
}

func HandleDaemonCycle(args []any, state *daemonState, cmd *exec.Cmd) mpvResponse {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.paused = !state.paused
	activeCmd := cmd
	if state.cmd != nil {
		activeCmd = state.cmd
	}
	SignalDaemonProcess(activeCmd, state.paused)
	return mpvResponse{Data: state.paused, Error: "success"}
}

func HandleDaemonSetProp(args []any, state *daemonState, cmd *exec.Cmd) mpvResponse {
	if len(args) > 2 {
		state.mu.Lock()
		defer state.mu.Unlock()
		prop, _ := args[1].(string)
		switch prop {
		case "pause":
			if p, ok := args[2].(bool); ok {
				state.paused = p
				activeCmd := cmd
				if state.cmd != nil {
					activeCmd = state.cmd
				}
				SignalDaemonProcess(activeCmd, state.paused)
			}
		case "volume":
			if v, ok := args[2].(float64); ok {
				state.volume = v
			}
		}
	}
	return mpvResponse{Error: "success"}
}

func SignalDaemonProcess(cmd *exec.Cmd, paused bool) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	sig := syscall.SIGCONT
	if paused {
		sig = syscall.SIGSTOP
	}
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		_ = syscall.Kill(-pgid, sig)
	} else {
		_ = cmd.Process.Signal(sig)
	}
}

func HandleDaemonGetProp(args []any, state *daemonState) mpvResponse {
	if len(args) < 2 {
		return mpvResponse{Error: "success"}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	prop, _ := args[1].(string)
	switch prop {
	case "pause":
		return mpvResponse{Data: state.paused, Error: "success"}
	case "media-title":
		return mpvResponse{Data: state.title, Error: "success"}
	case "time-pos", "playback-time":
		pos := state.offset
		if !state.paused && !state.start.IsZero() {
			pos += time.Since(state.start).Seconds()
		}
		return mpvResponse{Data: pos, Error: "success"}
	case "duration":
		return mpvResponse{Data: state.duration, Error: "success"}
	case "volume":
		vol := state.volume
		if vol <= 0 {
			vol = 100
		}
		return mpvResponse{Data: vol, Error: "success"}
	default:
		return mpvResponse{Error: "success"}
	}
}

func HandleDaemonSeek(args []any, state *daemonState) mpvResponse {
	if len(args) < 2 {
		return mpvResponse{Error: "success"}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	delta, _ := args[1].(float64)
	if !state.paused && !state.start.IsZero() {
		state.offset += time.Since(state.start).Seconds()
		state.start = time.Now()
	}
	state.offset += delta
	if state.offset < 0 {
		state.offset = 0
	}
	if state.duration > 0 && state.offset > state.duration {
		state.offset = state.duration
	}
	if state.cmd != nil && state.cmd.Process != nil {
		_ = state.cmd.Process.Signal(syscall.SIGCONT)
		_ = state.cmd.Process.Kill()
	}
	if newCmd, err := StartDaemonPlaybackAt(state.path, state.offset); err == nil {
		state.cmd = newCmd
		state.start = time.Now()
		if state.paused {
			SignalDaemonProcess(newCmd, true)
		}
	}
	return mpvResponse{Error: "success"}
}
