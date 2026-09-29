package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"

	"pod/pkg/util"
)

// doAllStep is one stage of `pod server do-all`.
type doAllStep struct {
	name string
	run  func(Config, CLIOptions) error
}

func buildServerDoAllSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "do-all",
		Description: "Fetch, transcribe, analyze, remove ads and republish the feeds, in one run",
		UsageLine:   "pod server do-all [podcast] [options]",
		Parameters: []clihelp.Param{
			{Name: "[podcast]", Description: "Limit every stage to one podcast, by name, index or ID"},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <name>", "", "Limit every stage to one podcast"),
			clihelp.Int(&opts.Count, "-n, --count <number>", 1, "Latest N episodes per show to fetch"),
			clihelp.Bool(&opts.DryRun, "-d, --dry-run", false, "Report what each stage would do without changing anything"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
			clihelp.Bool(&opts.IncludeHourly, "--hourly", false, "Include hourly news bulletins, skipped by default"),
			clihelp.String(&opts.UseLLM, "--profile <id/name>", "", "Select LLM profile ID or name"),
		},
		Examples: []clihelp.Example{
			{Line: "pod server do-all", Description: "Bring the whole library up to date"},
			{Line: "pod server do-all 'Fresh Air'", Description: "The same for one podcast"},
			{Line: "pod server do-all --dry-run", Description: "See what each stage would do"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "do-all"
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 && opts.Podcast == "" {
				opts.Podcast = ctx.Args[0]
			}
			return nil
		},
	}
}

// handleServerDoAll runs the whole maintenance cycle in the order each stage
// needs the one before it: new episodes are downloaded and queued, the older
// downloads that never got a transcript are transcribed, the transcripts are
// analysed for boilerplate, ad removal then runs on the queue and recuts the
// finished episodes with any boilerplate learned since, and last the feeds are
// republished so the site describes the audio as it now is.
//
// A stage that fails does not stop the ones after it: an unreachable feed should
// not keep the transcripts from being analysed. The failures are reported at the
// end, and the command exits non-zero if there were any.
func handleServerDoAll(cfg Config, cli CLIOptions) error {
	steps := doAllSteps()
	var failed []string
	start := time.Now()
	for i, step := range steps {
		if !cli.Quiet {
			fmt.Fprintf(outFor(cli), "\n== %d/%d %s ==\n", i+1, len(steps), step.name)
		}
		if err := step.run(cfg, doAllScope(cli)); err != nil {
			util.FprintError(errFor(cli), "%s: %v\n", step.name, err)
			failed = append(failed, step.name)
		}
	}
	if !cli.Quiet {
		fmt.Fprintf(outFor(cli), "\nFinished in %s.\n", time.Since(start).Round(time.Second))
	}
	if len(failed) > 0 {
		return errors.New(plural(len(failed), "stage") + " failed: " + strings.Join(failed, ", "))
	}
	return nil
}

func doAllSteps() []doAllStep {
	return []doAllStep{
		{"fetch new episodes", func(cfg Config, cli CLIOptions) error {
			cli.NoClean = true
			return runFetchCommand(cfg, cli)
		}},
		{"transcribe episodes missing a transcript", func(cfg Config, cli CLIOptions) error {
			cli.Count = 0
			return runTranscribeMissing(cfg, cli)
		}},
		{"analyze boilerplate", func(cfg Config, cli CLIOptions) error {
			cli.Count = 0
			return runAnalyzeCommand(cfg, cli)
		}},
		{"remove ads from queued episodes", func(cfg Config, cli CLIOptions) error {
			cli.Count = 0
			return handleQueueRun(library(cfg, cli, nil), cfg, cli, cli.Podcast)
		}},
		{"cut boilerplate from finished episodes", func(cfg Config, cli CLIOptions) error {
			cli.Count = 0
			cli.Recut = true
			return runBoilerplateRecut(cfg, cli)
		}},
		{"republish feeds", func(cfg Config, cli CLIOptions) error {
			if cli.DryRun {
				return nil
			}
			return handleServerFeed(cfg, cli)
		}},
	}
}

// doAllScope is the options each stage receives: the podcast the run is limited
// to, if any, as the argument every stage already understands.
func doAllScope(cli CLIOptions) CLIOptions {
	cli.Args = nil
	if cli.Podcast != "" {
		cli.Args = []string{cli.Podcast}
	}
	return cli
}
