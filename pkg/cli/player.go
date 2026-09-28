package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"pod/pkg/format"
	"pod/pkg/player"
	"pod/pkg/podcast"
	"pod/pkg/types"
	"pod/pkg/util"

	"github.com/sarielhp/clihelp"
)

var globalPlayer = player.GetGlobalPlayer()

func runPlayerCommand(cfg Config, cli CLIOptions) error {
	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "."
	}

	subcmd := cli.PlayerSubcmd
	args := cli.Args
	if subcmd == "" && len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "play", "stop", "pause", "status", "daemon":
			subcmd = strings.ToLower(args[0])
			args = args[1:]
		default:
			subcmd = "play"
		}
	} else if subcmd == "" {
		subcmd = "status"
	}

	switch subcmd {
	case "play":
		return handlePlayerPlay(outFor(cli), progressFor(cli), podcastsDir, args)
	case "stop":
		return handlePlayerStop(outFor(cli))
	case "pause":
		return handlePlayerPause(outFor(cli))
	case "status":
		return handlePlayerStatus(outFor(cli))
	case "daemon":
		return handlePlayerDaemon(args, cli.PlayerTitle, cli.Podcast)
	default:
		return fmt.Errorf("unknown player action %q (use play, stop, pause, or status)", subcmd)
	}
}

func handlePlayerPlay(w, prog io.Writer, podcastsDir string, args []string) error {
	if len(args) == 0 {
		if player.IsPlayerSocketAlive() {
			if err := player.ResumePlayerSocket(); err == nil {
				fmt.Fprintln(w, "Playback resumed.")
				return nil
			}
		}
		return fmt.Errorf("missing episode identifier for player play")
	}

	target := args[0]
	res, err := podcast.ResolveAnyID(podcastsDir, target)
	if err != nil {
		return err
	}
	if !res.IsEpisode() {
		return fmt.Errorf("identifier %q is a podcast; please specify an episode ID to play", target)
	}

	ep := res.Episode
	fmt.Fprintf(w, "Playing: %s [%s]\n", util.Bold(ep.Title), util.BoldCyan(ep.ShortID))
	fmt.Fprintf(w, "Audio file: %s\n", ep.Path)

	backend, err := player.StartPlayerTrackWith(ep.Path, ep.Title, ep.PodcastTitle)
	if err != nil {
		return fmt.Errorf("failed to start player: %w", err)
	}
	if backend.Name != "" && backend.Name != "mpv" {
		fmt.Fprintf(prog, "Playing with %s (install mpv for in-place seeking and media keys)\n", backend.Name)
	}

	track := types.PlayerTrack{
		Title:   ep.Title,
		Podcast: ep.PodcastTitle,
		Path:    ep.Path,
	}
	globalPlayer.Current = &track
	globalPlayer.IsPlaying = true
	fmt.Fprintf(w, "Started background playback (socket: %s)\n", player.PlayerSocketPath)
	return nil
}

func handlePlayerStop(w io.Writer) error {
	if !player.IsPlayerSocketAlive() {
		fmt.Fprintln(w, "Player is not running.")
		return nil
	}
	if err := player.StopPlayerSocket(); err != nil {
		return err
	}
	globalPlayer.Stop()
	fmt.Fprintln(w, "Playback stopped.")
	return nil
}

func handlePlayerPause(w io.Writer) error {
	if !player.IsPlayerSocketAlive() {
		fmt.Fprintln(w, "Player is not running.")
		return nil
	}
	paused, err := player.PausePlayerSocket()
	if err != nil {
		return err
	}
	if paused {
		fmt.Fprintln(w, "Playback paused.")
	} else {
		fmt.Fprintln(w, "Playback resumed.")
	}
	return nil
}

func handlePlayerStatus(w io.Writer) error {
	st, err := player.QueryPlayerStatus()
	if err != nil || st == nil || !st.IsRunning {
		fmt.Fprintln(w, "No active playback session (player is stopped).")
		return nil
	}

	statusLabel := "Playing"
	if st.IsPaused {
		statusLabel = "Paused"
	}

	fmt.Fprintf(w, "Playback Status:  %s\n", util.Bold(statusLabel))
	if st.Title != "" {
		fmt.Fprintf(w, "Track:            %s\n", util.BoldCyan(st.Title))
	}
	pct := 0.0
	if st.Duration > 0 {
		pct = (st.Position / st.Duration) * 100
	}
	fmt.Fprintf(w, "Position:         %s / %s (%.0f%%)\n", player.FormatPlayerTime(st.Position), player.FormatPlayerTime(st.Duration), pct)
	fmt.Fprintf(w, "Socket:           %s\n", player.PlayerSocketPath)
	return nil
}

func handlePlayerDaemon(args []string, title, podcast string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing audio path for player daemon")
	}
	return player.RunPlayerDaemon(args[0], title, podcast)
}

func printTranscriptText(w io.Writer, jsonPath string) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	var td TranscriptionData
	if err := json.Unmarshal(data, &td); err == nil && len(td.Segments) > 0 {
		for _, seg := range td.Segments {
			timeStr := fmt.Sprintf("[%s -> %s]", format.FormatSRTTime(seg.Start), format.FormatSRTTime(seg.End))
			fmt.Fprintf(w, "%s %s\n", util.BoldCyan(timeStr), strings.TrimSpace(seg.Text))
		}
		return nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err == nil {
		if text, ok := raw["text"].(string); ok && text != "" {
			fmt.Fprintln(w, text)
			return nil
		}
	}

	fmt.Fprintln(w, string(data))
	return nil
}

func buildPlayerCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "player",
		Description: "Control background audio playback",
		UsageLine:   "pod player [command]",
		Subcommands: []clihelp.Command{
			{
				Name:        "play",
				Description: "Play an episode or resume playback",
				UsageLine:   "pod player play [id]",
				Args:        clihelp.RangeArgs(0, 1),
				Run: func(ctx *clihelp.Context) error {
					*action = "player"
					opts.PlayerSubcmd = "play"
					opts.Args = ctx.Args
					return nil
				},
			},
			{
				Name:        "stop",
				Description: "Stop background audio playback",
				UsageLine:   "pod player stop",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					*action = "player"
					opts.PlayerSubcmd = "stop"
					return nil
				},
			},
			{
				Name:        "pause",
				Description: "Toggle playback pause state",
				UsageLine:   "pod player pause",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					*action = "player"
					opts.PlayerSubcmd = "pause"
					return nil
				},
			},
			{
				Name:        "status",
				Description: "Display player status and progress",
				UsageLine:   "pod player status",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					*action = "player"
					opts.PlayerSubcmd = "status"
					return nil
				},
			},
			{
				Name:        "daemon",
				Hidden:      true,
				Description: "Internal background player daemon",
				UsageLine:   "pod player daemon <audio> [--title <title>] [--podcast <name>]",
				Args:        clihelp.ExactArgs(1),
				Options: []clihelp.Option{
					clihelp.String(&opts.PlayerTitle, "--title <title>", "", "Track title to announce"),
					clihelp.String(&opts.Podcast, "--podcast <name>", "", "Podcast the track belongs to"),
				},
				Run: func(ctx *clihelp.Context) error {
					*action = "player"
					opts.PlayerSubcmd = "daemon"
					opts.Args = ctx.Args
					return nil
				},
			},
		},
		Args: clihelp.RangeArgs(0, 2),
		Run: func(ctx *clihelp.Context) error {
			*action = "player"
			opts.Args = ctx.Args
			return nil
		},
	}
}
