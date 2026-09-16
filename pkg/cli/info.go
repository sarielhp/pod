package cli

import (
	"fmt"
	"pod/pkg/podcast"
	"strconv"

	"github.com/sarielhp/clihelp"
)

func runInfoCommand(cfg Config, cli CLIOptions) error {
	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "."
	}

	limit := cli.Count
	if limit <= 0 {
		limit = 10
	}

	args := cli.Args
	if cli.InfoSubcmd == "transcript" {
		if len(args) != 1 {
			return fmt.Errorf("use abs info transcript <episode-id>")
		}
		return showEpisodeTranscript(podcastsDir, args[0])
	}
	if cli.InfoSubcmd == "latest" || (len(args) > 0 && args[0] == "latest") || cli.Latest {
		if len(args) > 0 && args[0] == "latest" {
			args = args[1:]
		}
		if len(args) > 0 {
			if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
				limit = n
			}
		}
		return listLatestEpisodes(podcastsDir, limit, cli)
	}

	if len(args) == 0 || (len(args) == 1 && (args[0] == "podcasts" || args[0] == "all")) {
		return listAllPodcasts(podcastsDir, cli)
	}

	target := args[0]
	if n, err := strconv.Atoi(target); err == nil && n > 0 && !podcastExistsByIndexOrID(podcastsDir, target) {
		return listLatestEpisodes(podcastsDir, n, cli)
	}

	resolved, err := podcast.ResolveAnyID(podcastsDir, target)
	if err != nil {
		return err
	}

	if resolved.IsPodcast() {
		return inspectPodcastInfo(resolved.Podcast, cli, cfg.ServerBaseURL)
	}

	if resolved.IsEpisode() {
		if cli.ShowTranscript || cli.ExportFormat != "" || cli.ExportTXT || cli.ExportSRT {
			return runTranscriptForEpisode(resolved.Episode, cli)
		}
		return inspectEpisodeInfo(resolved.Episode, cli)
	}

	return fmt.Errorf("could not inspect %q", target)
}

func buildInfoCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "info",
		Description: "Library query, inspection, cuts and transcripts",
		UsageLine:   "pod info [options] [id|latest [N]|status|check]",
		Subcommands: []clihelp.Command{
			buildInfoLatestSubcommand(opts, action),
			buildInfoStatusSubcommand(opts, action),
			buildInfoCheckSubcommand(opts, action),
			buildInfoTranscriptSubcommand(opts, action),
		},
		Args: clihelp.RangeArgs(0, 2),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.JSON, "--json", false, "Output results in JSON format"),
			clihelp.Bool(&opts.ShowCuts, "--cuts", false, "Display detailed cuts breakdown"),
			clihelp.Bool(&opts.ShowTranscript, "--transcript", false, "Display episode transcript text"),
			clihelp.String(&opts.ExportFormat, "--export <format>", "", "Export transcript to format ('srt' or 'txt')"),
			clihelp.Int(&opts.Count, "-n, --limit <number>", 0, "Limit number of episodes to list"),
			clihelp.Bool(&opts.Latest, "-l, --latest", false, "List latest episodes across library"),
			clihelp.Bool(&opts.DownloadedOnly, "--downloaded", false, "List only episodes on disk, newest file first"),
			clihelp.Bool(&opts.IncludeHourly, "--hourly", false, "Include hourly news bulletins, hidden by default"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress formatting/headers"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
			clihelp.String(&opts.Output, "-o, --output <path>", "", "Output destination for export"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "info"
			if len(ctx.Args) > 0 {
				switch ctx.Args[0] {
				case "latest":
					opts.InfoSubcmd = "latest"
					opts.Args = ctx.Args[1:]
					return nil
				case "status":
					opts.InfoSubcmd = "status"
					opts.Args = ctx.Args[1:]
					return nil
				case "check":
					opts.InfoSubcmd = "check"
					opts.StatusSubcmd = "check"
					args := ctx.Args[1:]
					if len(args) > 0 && args[0] == "kitty" {
						opts.Args = args[1:]
					} else {
						opts.Args = args
					}
					return resolveTestCommandArgs(args, opts)
				}
			}
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildInfoLatestSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "latest",
		Description: "List latest published episodes across all podcasts",
		UsageLine:   "pod info latest [N] [options]",
		Parameters: []clihelp.Param{
			{Name: "[N]", Description: "Number of episodes to show (default: 10)"},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.Int(&opts.Count, "-n, --limit <number>", 10, "Number of latest episodes to list"),
			clihelp.Bool(&opts.DownloadedOnly, "--downloaded", false, "Only episodes on disk, newest file first"),
			clihelp.Bool(&opts.IncludeHourly, "--hourly", false, "Include hourly news bulletins, hidden by default"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.JSON, "--json", false, "Output results in JSON format"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "info"
			opts.InfoSubcmd = "latest"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildInfoStatusSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "status",
		Description: "Show status overview of library and worker",
		UsageLine:   "pod info status [options] [podcasts]",
		Args:        clihelp.RangeArgs(0, 2),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "info"
			opts.InfoSubcmd = "status"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildInfoCheckSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "check",
		Description: "Test external services (Whisper, Gemini, Kitty)",
		UsageLine:   "pod info check [options] [target]",
		Args:        clihelp.RangeArgs(0, 2),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.TestWhisper, "--test-whisper", false, "Test whisper server connection"),
			clihelp.Bool(&opts.TestKitty, "--test-kitty", false, "Test Kitty cover image display"),
			clihelp.Bool(&opts.TestGemini, "--test-gemini", false, "Test Gemini API key and quota status"),
			clihelp.Bool(&opts.TestModels, "--models", false, "Compare the Gemini model chain with what the key can reach"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "info"
			opts.InfoSubcmd = "check"
			opts.StatusSubcmd = "check"
			if len(ctx.Args) > 0 && ctx.Args[0] == "kitty" {
				opts.Args = ctx.Args[1:]
			} else {
				opts.Args = ctx.Args
			}
			return resolveTestCommandArgs(ctx.Args, opts)
		},
	}
}
