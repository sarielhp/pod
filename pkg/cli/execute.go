package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/tui"
	"pod/pkg/util"
)

func Execute(args []string) int {
	action, cli, err := parseFlagsArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if action == "" {
		return 0
	}
	_, _ = config.EnsureConfigExists()
	appCfg := loadConfig()
	if !cli.DryRun {
		root := appCfg.PodcastsDir
		if cli.PodcastsDir != "" {
			root = cli.PodcastsDir
		}
		removed, err := util.CleanupStaleWorkDirs(root, time.Now())
		if err != nil {
			fmt.Fprintf(errFor(cli), "Warning: stale work cleanup: %v\n", err)
		}
		if removed > 0 {
			fmt.Fprintf(errFor(cli), "Removed %d stale .work directories (older than 24 hours).\n", removed)
		}
	}

	if err := dispatch(action, &appCfg, cli); err != nil {
		if errors.Is(err, podcast.ErrAmbiguousPodcast) {
			var ambErr *podcast.AmbiguousPodcastError
			if errors.As(err, &ambErr) {
				fmt.Fprintln(outFor(cli), podcast.FormatPodcastMatches(ambErr.Matches))
			} else {
				fmt.Fprintln(outFor(cli), err.Error())
			}
			return 1
		}
		fmt.Fprintf(errFor(cli), "Error: %v\n", err)
		return 1
	}
	return 0
}

// dispatch routes a parsed command to its handler. Every top-level command has
// exactly one case here, in the order the commands are registered. There used
// to be a second table, handleParityCommands, consulted first and holding an
// unrelated four of them.
func dispatch(action string, config *Config, cli CLIOptions) error {
	switch action {
	case "config":
		return runConfigCommand(config, cli)
	case "info":
		if cli.InfoSubcmd == "status" || cli.InfoSubcmd == "check" {
			return runStatusCommand(config, cli)
		}
		return runInfoCommand(*config, cli)
	case "player":
		return runPlayerCommand(*config, cli)
	case "queue":
		return runQueueCommand(*config, cli)
	case "rm_ads":
		return runRmAdsCommand(*config, cli, action)
	case "server", "sync":
		return handleServerCommand(*config, cli)
	case "transcribe":
		return runTranscribeCommand(*config, cli)
	case "tui":
		return tui.RunTUI(config, cli.PodcastsDir)
	}
	return nil
}
