package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sarielhp/clihelp"
	"github.com/sarielhp/clihelp/tree"
)

func resolveTestCommandArgs(args []string, opts *CLIOptions) error {
	if len(args) == 0 {
		opts.TestWhisper = true
		return nil
	}
	switch args[0] {
	case "whisper", "whisper-server":
		opts.TestWhisper = true
	case "kitty":
		opts.TestKitty = true
	case "gemini":
		opts.TestGemini = true
	default:
		return fmt.Errorf("unknown test target %q (valid targets: whisper, kitty, gemini)", args[0])
	}
	return nil
}

func hideOption(o clihelp.Option) clihelp.Option {
	o.Hidden = true
	return o
}

func getTranscriptionOptions(opts *CLIOptions) []clihelp.Option {
	return []clihelp.Option{
		clihelp.String(&opts.Output, "-o, --output <path>", "", "Output MP3 path or directory"),
		clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
		clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
		hideOption(clihelp.BoolToggle(&opts.SaveTranscript, "--[no-]transcript", true, "Save default .transcript.json file")),
		hideOption(clihelp.Bool(&opts.UseChunks, "--use-chunks", false, "Split audio into chunks")),
		hideOption(clihelp.String(&opts.TranscribeMin, "-t, --tminutes <minutes>", "", "Transcribe first N minutes")),
		hideOption(clihelp.Bool(&opts.Recut, "--recut", false, "Recut audio using existing cuts metadata")),
		clihelp.String(&opts.Force, "-f, --force <stage>", "", "Force: 'whisper', 'llm', or 'all'"),
		clihelp.String(&opts.UseLLM, "--profile <id/name>", "", "Select LLM profile ID or name"),
		clihelp.Bool(&opts.DryRun, "--dry-run", false, "Preview actions without file changes"),
		// Not hidden: bounding a run is what makes ad removal safe to schedule.
		// An unbounded sweep over this library is sixteen hours of GPU and a
		// real bill, and it holds the library lock for all of it.
		clihelp.Int(&opts.Count, "-n, --limit <number>", 0, "Process at most N episodes"),
		hideOption(clihelp.Int(&opts.Priority, "-P, --priority <level>", 0, "Priority level for processing")),
		clihelp.String(&opts.Podcast, "-p, --podcast <name>", "", "Target podcast by ID, index, or name"),
		hideOption(clihelp.String(&opts.WhisperEngine, "--whisper-engine <engine>", "", "Engine: local, docker, remote, gemini")),
		hideOption(clihelp.String(&opts.WhisperModel, "--whisper-model <model>", "", "Model name or alias (e.g. tiny.en, base)")),
	}
}

func parseFlagsArgs(args []string) (string, CLIOptions, error) {
	var action string
	opts := CLIOptions{
		ProcOptions: ProcOptions{
			SaveTranscript: true,
		},
	}

	for i, a := range args {
		if a == "--tree" || (a == "help" && i+1 < len(args) && args[i+1] == "tree") {
			app := buildCLIApp(&action, &opts)
			tree.Render(os.Stdout, app, tree.Options{})
			return "", opts, nil
		}
	}

	app := buildCLIApp(&action, &opts)
	if handleExamplesCLI(os.Stdout, app, args) {
		return "", opts, nil
	}

	err := app.Execute(args)
	if err != nil {
		return "", opts, err
	}

	if action == "" {
		return "", opts, nil
	}

	return action, opts, nil
}

func parseFlags() (string, CLIOptions) {
	action, opts, err := parseFlagsArgs(os.Args[1:])
	if err != nil {
		fatalError("Error: %v\n", err)
	}
	if action == "" {
		os.Exit(0)
	}
	return action, opts
}

// isExamplesFlag recognises pod's own spellings, including a bundled short such
// as "-xE". clihelp answers "help examples"; these are the extra ways pod has
// always accepted the request, kept so that no user's habit breaks.
func isExamplesFlag(arg string) bool {
	if arg == "-E" || arg == "--examples" {
		return true
	}
	return strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Contains(arg, "E")
}

func isExamplesRequest(args []string) bool {
	for _, a := range args {
		if isExamplesFlag(a) {
			return true
		}
	}
	return len(args) > 0 && args[0] == "examples"
}

// handleExamplesCLI turns pod's spellings into clihelp's "help examples", which
// collects the tree, groups by command, colours the lines and honours the pager.
//
// This used to be about a hundred and fifty lines here: the collection walk, a
// brute-force command resolver, a theme, and the rendering. clihelp grew the
// view in v0.3.35 and learned the per-command form in v0.3.36, so all of that
// is gone and the two cannot drift apart.
func handleExamplesCLI(w io.Writer, app *clihelp.App, args []string) bool {
	if !isExamplesRequest(args) {
		return false
	}
	help := []string{"help", "examples"}
	for _, a := range args {
		if isExamplesFlag(a) || a == "examples" || a == "help" || strings.HasPrefix(a, "-") {
			continue
		}
		help = append(help, a)
	}
	if w != nil && w != os.Stdout {
		app.Stdout = w
	}
	_ = app.Execute(help)
	return true
}
