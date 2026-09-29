package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/pipeline"
	"pod/pkg/util"
)

func buildSpeakersCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "speakers",
		Hidden:      true,
		Description: "Give the speakers in a transcript names, by hand or by asking a model",
		UsageLine:   "pod speakers [options] <path...>",
		Parameters: []clihelp.Param{
			{Name: "<path...>", Description: "Transcripts made with `pod transcribe --speakers`, or the media files they belong to"},
		},
		Args: clihelp.MinimumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.SpeakersSet, "--set <list>", "", "Name speakers yourself, as SPEAKER_00=Name,SPEAKER_01=Name; no model is asked"),
			clihelp.Bool(&opts.SpeakersClear, "--clear", false, "Remove the names, back to SPEAKER_00 and so on"),
			clihelp.String(&opts.UseLLM, "--profile <id/name>", "", "LLM profile to ask (default: the active one)"),
			clihelp.String(&opts.DetectModel, "--model <id>", "", "Use this model on the profile's endpoint"),
			clihelp.String(&opts.DetectTimeout, "--timeout <duration>", "", "Wait this long for the model's reply, e.g. 300s"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show the names without writing them"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod speakers interview.transcript.json", Description: "Ask a model who is speaking, and record the names"},
			{Line: "pod speakers --dry-run ep.mp3", Description: "See the names it would choose without recording them"},
			{Line: "pod speakers --set 'SPEAKER_00=Elad,SPEAKER_03=Elad,SPEAKER_01=Dana Levy' ep.transcript.json", Description: "Name them yourself; two labels with one name are one person"},
			{Line: "pod speakers --clear ep.transcript.json", Description: "Forget the names"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "speakers"
			opts.Args = ctx.Args
			return nil
		},
	}
}

// parseSpeakerAssignments reads "SPEAKER_00=Elad,SPEAKER_01=Dana".
func parseSpeakerAssignments(list string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(list, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		label, name, ok := strings.Cut(part, "=")
		label = strings.TrimSpace(label)
		if !ok || label == "" {
			return nil, fmt.Errorf("cannot read %q: write it as SPEAKER_00=Name", strings.TrimSpace(part))
		}
		out[label] = strings.TrimSpace(name)
	}
	return out, nil
}

func runSpeakersCommand(cfg Config, cli CLIOptions) error {
	set, err := parseSpeakerAssignments(cli.SpeakersSet)
	if err != nil {
		return err
	}
	if cli.SpeakersClear && len(set) > 0 {
		return fmt.Errorf("--clear and --set say opposite things; use one")
	}
	var failures []string
	for _, path := range uniquePaths(cli.Args) {
		res, err := pipeline.NameSpeakers(pipeline.NameSpeakersRequest{
			Path: path, Set: set, Clear: cli.SpeakersClear,
			Profile: cli.UseLLM, Model: cli.DetectModel, Timeout: cli.DetectTimeout,
			DryRun: cli.DryRun, Markdown: true,
		}, cfg, reporter(cli))
		if err != nil {
			util.FprintError(errFor(cli), "%s: %v\n", filepath.Base(path), err)
			failures = append(failures, filepath.Base(path))
			continue
		}
		printSpeakerNames(outFor(cli), cli, res)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d transcript(s) could not be named: %s", len(failures), len(cli.Args), strings.Join(failures, ", "))
	}
	return nil
}

func printSpeakerNames(w io.Writer, cli CLIOptions, res pipeline.NameSpeakersResult) {
	verb := "named"
	if cli.DryRun {
		verb = "would be named"
	}
	fmt.Fprintf(w, "%s: %d of %d speaker(s) %s\n", filepath.Base(res.TranscriptPath), len(res.Names), len(res.Speakers), verb)
	for _, label := range res.Speakers {
		name := res.Names[label]
		if name == "" {
			name = "(no name)"
		}
		fmt.Fprintf(w, "  %-11s %s\n", label, name)
	}
	if !cli.DryRun {
		for _, p := range res.Written {
			fmt.Fprintf(w, "  wrote %s\n", p)
		}
	}
}
