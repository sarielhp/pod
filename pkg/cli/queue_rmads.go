package cli

import (
	"github.com/sarielhp/clihelp"
)

func buildQueueRecutSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "recut",
		Description: "Recut audio files from their recorded cuts, adding the podcast's boilerplate",
		UsageLine:   "pod queue recut [options] [path...]",
		Options: []clihelp.Option{
			clihelp.String(&opts.Output, "-o, --output <path>", "", "Output MP3 path or directory"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
			clihelp.Int(&opts.Count, "-n, --limit <number>", 0, "Maximum number of episodes to recut"),
			clihelp.Bool(&opts.NoBoilerplate, "--no-boilerplate", false, "Only redo the recorded cuts; do not also cut the podcast's boilerplate (see 'pod analyze')"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Report what the boilerplate cut would change without changing anything"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "recut"
			opts.ProcSubcmd = "recut"
			opts.Recut = true
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildQueueExportSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "export",
		Description: "Export transcript JSON to SRT subtitles or text",
		UsageLine:   "pod queue export [command] [options] <path...>",
		Subcommands: []clihelp.Command{
			{
				Name:        "srt",
				Description: "Export transcript to SubRip (.srt) subtitle format",
				UsageLine:   "pod queue export srt <path1> [path2 ...] [options]",
				Parameters: []clihelp.Param{
					{Name: "<path1> [path2 ...]", Description: "Transcript JSON files or directories to export"},
				},
				Args: clihelp.MinimumNArgs(1),
				Options: []clihelp.Option{
					clihelp.String(&opts.Output, "-o, --output <path>", "", "Custom output .srt file path or directory"),
					clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
				},
				Run: func(ctx *clihelp.Context) error {
					*action = "queue"
					opts.QueueSubcmd = "export"
					opts.ProcSubcmd = "export"
					opts.ExportFormat = "srt"
					opts.ExportSRT = true
					opts.Args = ctx.Args
					return nil
				},
			},
			{
				Name:        "txt",
				Description: "Export transcript to plain text (.txt) format",
				UsageLine:   "pod queue export txt <path1> [path2 ...] [options]",
				Parameters: []clihelp.Param{
					{Name: "<path1> [path2 ...]", Description: "Transcript JSON files or directories to export"},
				},
				Args: clihelp.MinimumNArgs(1),
				Options: []clihelp.Option{
					clihelp.String(&opts.Output, "-o, --output <path>", "", "Custom output .txt file path or directory"),
					clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
				},
				Run: func(ctx *clihelp.Context) error {
					*action = "queue"
					opts.QueueSubcmd = "export"
					opts.ProcSubcmd = "export"
					opts.ExportFormat = "txt"
					opts.ExportTXT = true
					opts.Args = ctx.Args
					return nil
				},
			},
		},
		Options: []clihelp.Option{
			clihelp.String(&opts.ExportFormat, "--format <format>", "", "Export format: 'srt' or 'txt'"),
			clihelp.String(&opts.Output, "-o, --output <path>", "", "Custom output file path or directory"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "export"
			opts.ProcSubcmd = "export"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildQueueAuditSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "audit",
		Description: "Scan and heal invalid clean states plus suspicious episode transcripts",
		UsageLine:   "pod queue audit [paths...] [options]",
		Parameters: []clihelp.Param{
			{Name: "[paths...]", Description: "Podcast directories or audio files to audit (defaults to configured podcasts_dir)"},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&opts.DryRun, "-d, --dry-run", false, "Report suspicious transcripts without modifying or deleting files"),
			clihelp.String(&opts.AuditMinRatioStr, "--min-ratio <ratio>", "", "Minimum speech coverage ratio of audio duration (default: 0.15)"),
			clihelp.Int(&opts.AuditMinChars, "--min-chars <num>", 50, "Minimum character count for non-empty transcript"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress non-warning output"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Print detailed inspection of every file"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "queue"
			opts.QueueSubcmd = "audit"
			opts.ProcSubcmd = "audit"
			opts.Args = ctx.Args
			return nil
		},
	}
}
