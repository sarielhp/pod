package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/sarielhp/clihelp"

	"pod/pkg/pipeline"
)

func buildAnalyzeCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "analyze",
		Hidden:      true,
		Description: "Learn a show's boilerplate (intros, credits, standing promos) and record it in its podcast.json",
		UsageLine:   "pod analyze [options] <podcast|directory>...",
		Parameters: []clihelp.Param{
			{Name: "<podcast|directory>...", Description: "Podcast by name, index or ID, a podcast directory, or 'all'"},
		},
		Args: clihelp.MinimumNArgs(1),
		Options: []clihelp.Option{
			clihelp.Int(&opts.AnalyzeMinEpisodes, "--min-episodes <n>", pipeline.DefaultBoilerplateEpisodes, "Record text found in at least this many episodes"),
			clihelp.Int(&opts.RepeatsMinWords, "--min-words <n>", 20, "Shortest recurring passage to record, in words"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show what would be recorded without writing podcast.json"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "List every phrase"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod analyze --dry-run 'Fresh Air'", Description: "See which passages would be recorded as boilerplate"},
			{Line: "pod analyze all", Description: "Record boilerplate for every podcast with enough episodes"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "analyze"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func runAnalyzeCommand(cfg Config, cli CLIOptions) error {
	dirs, err := analyzeTargets(cfg, cli)
	if err != nil {
		return err
	}
	var failures int
	for _, dir := range dirs {
		if err := analyzeOne(cli, dir); err != nil {
			fmt.Fprintf(errFor(cli), "%s: %v\n", dir, err)
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d of %d podcast(s) could not be analysed", failures, len(dirs))
	}
	return nil
}

func analyzeTargets(cfg Config, cli CLIOptions) ([]string, error) {
	var dirs []string
	lib := library(cfg, cli, nil)
	for _, arg := range uniquePaths(cli.Args) {
		if fi, err := os.Stat(arg); err == nil && fi.IsDir() {
			dirs = append(dirs, arg)
			continue
		}
		group, err := lib.ResolveGroup(arg)
		if err != nil {
			return nil, err
		}
		for _, entry := range group.Entries {
			dirs = append(dirs, entry.Dir)
		}
	}
	return dirs, nil
}

func analyzeOne(cli CLIOptions, dir string) error {
	res, err := pipeline.AnalyzePodcast(dir, pipeline.AnalyzeOptions{
		MinEpisodes: cli.AnalyzeMinEpisodes,
		MinWords:    cli.RepeatsMinWords,
		DryRun:      cli.DryRun,
	})
	if err != nil {
		return err
	}
	w := progressFor(cli)
	if res.Skipped != "" {
		fmt.Fprintf(w, "%s: skipped, %s\n", dir, res.Skipped)
		return nil
	}
	verb := "recorded"
	if cli.DryRun {
		verb = "would record"
	}
	fmt.Fprintf(w, "%s: %d transcripts, %s %d boilerplate phrase(s) (%d new, %d dropped)\n",
		dir, res.Transcripts, verb, len(res.Phrases), res.Added, res.Dropped)
	if cli.DryRun || cli.Verbose {
		listPhrases(outFor(cli), res)
	}
	return nil
}

func listPhrases(w io.Writer, res pipeline.AnalyzeResult) {
	for _, p := range res.Phrases {
		flag := ""
		if p.Disabled {
			flag = " [disabled]"
		}
		fmt.Fprintf(w, "  %3d eps  %-5s %s%s\n", p.Episodes, p.Position, clip(p.Text, 140), flag)
	}
}
