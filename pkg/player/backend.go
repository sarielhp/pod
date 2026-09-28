package player

import (
	"errors"
	"os/exec"
)

// Backend is one playback program pod can drive. They are tried in the order
// below: mpv seeks in place, exposes MPRIS to desktop media keys and answers
// status queries natively; the daemon-driven players restart the process on
// every seek, and ffplay and mpg123 cannot pause through the socket.
type Backend struct {
	Name string
	Path string
	Note string
}

var backendOrder = []Backend{
	{Name: "mpv", Note: "in-place seeking, MPRIS media keys, native status"},
	{Name: "cvlc", Note: "driven by the pod daemon; a seek restarts the player"},
	{Name: "ffplay", Note: "driven by the pod daemon; no pause, a seek restarts the player"},
	{Name: "mpg123", Note: "driven by the pod daemon; no pause, a seek restarts the player"},
}

// ErrNoBackend is returned when none of the supported players is on PATH.
var ErrNoBackend = errors.New("no audio player found on PATH; install mpv (recommended), vlc (cvlc), ffmpeg (ffplay) or mpg123")

// Backends reports every supported player, with Path set for the ones found.
func Backends() []Backend {
	out := make([]Backend, len(backendOrder))
	copy(out, backendOrder)
	for i := range out {
		if p, err := exec.LookPath(out[i].Name); err == nil {
			out[i].Path = p
		}
	}
	return out
}

// SelectedBackend is the player 'pod player play' will use right now.
func SelectedBackend() (Backend, error) {
	for _, b := range Backends() {
		if b.Path != "" {
			return b, nil
		}
	}
	return Backend{}, ErrNoBackend
}
