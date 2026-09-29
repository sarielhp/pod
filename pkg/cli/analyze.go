package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/pipeline"
	"pod/pkg/util"
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
			clihelp.Bool(&opts.ShowBoilerplate, "--show-bp", false, "Show the boilerplate already recorded for the podcast, in full, without analysing"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show what would be recorded without writing podcast.json"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "List every phrase"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod analyze --show-bp hard_fork", Description: "Read the boilerplate recorded for a podcast, in full"},
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
		run := analyzeOne
		if cli.ShowBoilerplate {
			run = showBoilerplate
		}
		if err := run(cli, dir); err != nil {
			util.FprintError(errFor(cli), "%s: %v\n", dir, err)
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

const (
	boilerplateWrapWidth = 96
	// wordsPerSecond is a conversational speaking rate, used to estimate the
	// length of a phrase recorded before its duration was kept.
	wordsPerSecond = 2.5
)

// showBoilerplate prints the boilerplate recorded in a podcast's podcast.json,
// in full, word-wrapped and separated by rules. It reads only: nothing is
// analysed or written.
func showBoilerplate(cli CLIOptions, dir string) error {
	cfg, err := config.LoadPodcastConfigErr(dir, config.PodcastConfig{})
	if err != nil {
		return err
	}
	w := outFor(cli)
	folder := filepath.Base(filepath.Clean(dir))
	if len(cfg.Boilerplate) == 0 {
		fmt.Fprintf(w, "%s: no boilerplate recorded. Run 'pod analyze %s' to learn it.\n", listTitle(folder), folder)
		return nil
	}
	heavy := strings.Repeat("━", boilerplateWrapWidth+4)
	light := strings.Repeat("┈", boilerplateWrapWidth+4)
	fmt.Fprintf(w, "%s: %d boilerplate phrase(s)\n", listTitle(folder), len(cfg.Boilerplate))
	for i, p := range cfg.Boilerplate {
		fmt.Fprintf(w, "%s\n%2d  %d eps · %s · %d words · %s%s\n%s\n", heavy, i+1, p.Episodes, orDash(p.Position), len(strings.Fields(p.Text)), phraseLength(p), phraseFlags(p), light)
		for _, line := range wrapText(p.Text, boilerplateWrapWidth) {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
	fmt.Fprintln(w, heavy)
	return nil
}

// phraseSeconds is how long a phrase runs: the measured duration when it was
// kept, otherwise an estimate from its word count. The boolean is true for an
// estimate.
func phraseSeconds(p config.BoilerplatePhrase) (float64, bool) {
	if p.Seconds > 0 {
		return p.Seconds, false
	}
	return float64(len(strings.Fields(p.Text))) / wordsPerSecond, true
}

func phraseLength(p config.BoilerplatePhrase) string {
	secs, estimated := phraseSeconds(p)
	if estimated {
		return "~" + format.FormatClock(secs)
	}
	return format.FormatClock(secs)
}

func phraseFlags(p config.BoilerplatePhrase) string {
	var flags []string
	if p.Disabled {
		flags = append(flags, "disabled")
	}
	if p.Manual {
		flags = append(flags, "manual")
	}
	if len(flags) == 0 {
		return ""
	}
	return "  [" + strings.Join(flags, ", ") + "]"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
