package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
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

func isExamplesFlag(arg string) bool {
	if arg == "-E" || arg == "--examples" {
		return true
	}
	if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Contains(arg, "E") {
		return true
	}
	return false
}

func isExamplesRequest(args []string) bool {
	for _, a := range args {
		if isExamplesFlag(a) {
			return true
		}
	}
	if len(args) > 0 && args[0] == "examples" {
		return true
	}
	if len(args) >= 2 && args[0] == "help" && args[1] == "examples" {
		return true
	}
	return false
}

func extractCommandTokens(args []string) []string {
	var tokens []string
	for _, a := range args {
		if isExamplesFlag(a) || a == "help" || a == "examples" || strings.HasPrefix(a, "-") {
			continue
		}
		tokens = append(tokens, a)
	}
	return tokens
}

func resolveTargetCommand(app *clihelp.App, tokens []string) (*clihelp.Command, []string) {
	for start := 0; start < len(tokens); start++ {
		for end := len(tokens); end > start; end-- {
			if cmd := app.LookupCommand(tokens[start:end]...); cmd != nil {
				return cmd, tokens[start:end]
			}
		}
	}
	return nil, nil
}

func collectExamples(app *clihelp.App, cmd *clihelp.Command) []clihelp.Example {
	if cmd != nil {
		if len(cmd.Examples) > 0 {
			return cmd.Examples
		}
		var subEx []clihelp.Example
		for _, sub := range cmd.Subcommands {
			subEx = append(subEx, collectExamples(app, &sub)...)
		}
		return subEx
	}
	if app != nil && len(app.Examples) > 0 {
		return app.Examples
	}
	var appEx []clihelp.Example
	for _, c := range app.Commands {
		appEx = append(appEx, collectExamples(app, &c)...)
	}
	return appEx
}

func cliExampleTheme() clihelp.Theme {
	return clihelp.Theme{
		Hdr:            color.New(color.FgYellow, color.Bold),
		Body:           color.New(color.FgWhite),
		Accent:         color.New(color.FgCyan, color.Bold),
		Subcommand:     color.New(color.FgGreen),
		Flag:           color.New(color.FgCyan),
		ExampleCmd:     color.New(color.FgGreen, color.Bold),
		ExampleFlag:    color.New(color.FgCyan),
		ExampleArg:     color.New(color.FgWhite),
		ExampleComment: color.New(color.FgHiBlack),
		ExampleDesc:    color.New(color.FgHiBlack),
	}
}

func printCommandExamples(w io.Writer, app *clihelp.App, cmd *clihelp.Command, path []string) {
	examples := collectExamples(app, cmd)
	if len(examples) == 0 {
		name := "pod"
		if len(path) > 0 {
			name = "pod " + strings.Join(path, " ")
		}
		fmt.Fprintf(w, "No examples available for %s.\nRun '%s --help' for usage.\n", name, name)
		return
	}

	th := cliExampleTheme()
	if th.Hdr != nil {
		th.Hdr.Fprintln(w, "Examples:")
	} else {
		fmt.Fprintln(w, "Examples:")
	}

	for i, ex := range examples {
		if i > 0 {
			fmt.Fprintln(w)
		}
		for _, l := range strings.Split(ex.Line, "\n") {
			colored := clihelp.ColorizeExampleLineWithApp(app, cmd, l, th)
			fmt.Fprintf(w, "  %s\n", colored)
		}
		if ex.Description != "" {
			if th.ExampleDesc != nil {
				th.ExampleDesc.Fprintf(w, "    %s\n", ex.Description)
			} else {
				fmt.Fprintf(w, "    %s\n", ex.Description)
			}
		}
	}
}

func handleExamplesCLI(w io.Writer, app *clihelp.App, args []string) bool {
	if !isExamplesRequest(args) {
		return false
	}
	if w == nil {
		w = os.Stdout
	}
	tokens := extractCommandTokens(args)
	cmd, path := resolveTargetCommand(app, tokens)
	printCommandExamples(w, app, cmd, path)
	return true
}
