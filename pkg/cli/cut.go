package cli

import (
	"fmt"
	"path/filepath"

	"github.com/sarielhp/clihelp"

	"pod/pkg/format"
	"pod/pkg/pipeline"
)

func buildCutCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "cut",
		Hidden:      true,
		Description: "Cut advertisements from audio using cuts metadata",
		UsageLine:   "pod cut [options] <episode>",
		Parameters: []clihelp.Param{
			{Name: "<episode>", Description: "Audio file to cut"},
		},
		Args: clihelp.MinimumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.CutsFile, "--cuts <file>", "", "Cuts metadata JSON file (default: beside episode)"),
			clihelp.String(&opts.Output, "-o, --output <path>", "", "Output file path (default: overwrite in-place)"),
			clihelp.Bool(&opts.DryRun, "-d, --dry-run", false, "Preview what would be cut without cutting"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod cut episode.mp3", Description: "Cut audio using episode.cuts.json"},
			{Line: "pod cut episode.mp3 --cuts manual.cuts.json", Description: "Cut using a specific cuts file"},
			{Line: "pod cut episode.mp3 -o clean.mp3", Description: "Write cut audio to clean.mp3"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "cut"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func runCutCommand(cfg Config, cli CLIOptions) error {
	if len(cli.Args) == 0 {
		return fmt.Errorf("missing audio file to cut")
	}

	req := pipeline.CutRequest{
		Path:     cli.Args[0],
		CutsPath: cli.CutsFile,
		Output:   cli.Output,
		DryRun:   cli.DryRun,
	}

	opts := cli.ProcOptions
	opts.Normalize()

	res, err := pipeline.CutFile(req, cfg, opts, reporter(cli))
	if err != nil {
		return err
	}

	prog := progressFor(cli)
	if cli.DryRun {
		fmt.Fprintf(prog, "[Dry run] Would cut %s (%d segments, %s trimmed, new length %s)\n",
			filepath.Base(res.InputPath), res.SegmentsCut,
			format.FormatTime(res.CutSec), format.FormatMinutes(res.CleanedSec))
		return nil
	}

	fmt.Fprintf(prog, "Cut %s: %d segments cut, %s trimmed, saved to %s\n",
		filepath.Base(res.InputPath), res.SegmentsCut,
		format.FormatTime(res.CutSec), res.OutputPath)
	return nil
}
