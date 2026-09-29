package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/detect"
	"pod/pkg/format"
	"pod/pkg/pipeline"
)

func buildRepeatsCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "repeats",
		Hidden:      true,
		Description: "Find text that repeats across a show's episodes: ads, promos, intros, credits",
		UsageLine:   "pod repeats [options] <directory|transcript...>",
		Parameters: []clihelp.Param{
			{Name: "<directory|transcript...>", Description: "A podcast directory, or transcript files, whose episodes are compared with each other"},
		},
		Args: clihelp.MinimumNArgs(1),
		Options: []clihelp.Option{
			clihelp.Int(&opts.RepeatsMinWords, "--min-words <n>", 20, "Shortest repeated span to report, in words"),
			clihelp.Int(&opts.RepeatsMinEpisodes, "--min-episodes <n>", 1, "Report only spans found in at least this many episodes, counting the one shown"),
			clihelp.String(&opts.RepeatsEpisode, "--episode <text>", "", "List the spans, with their text, of episodes whose name contains this"),
			clihelp.Bool(&opts.RepeatsCatalog, "--catalog", false, "List the texts that recur most across the show"),
			clihelp.Bool(&opts.RepeatsCurve, "--curve", false, "Score against labelled episodes as more episodes are compared"),
			clihelp.Bool(&opts.JSON, "--json", false, "Emit the results as JSON"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod repeats /media/podcasts/clean/Fresh_Air", Description: "How much of each episode repeats elsewhere in the show"},
			{Line: "pod repeats --catalog --min-episodes 5 Fresh_Air/", Description: "The reads and boilerplate that recur in 5+ episodes"},
			{Line: "pod repeats --curve Dan_Snows_History_Hit/", Description: "Recall and precision against labelled truth as the episode count grows"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "repeats"
			opts.Args = ctx.Args
			return nil
		},
	}
}

// RepeatSpan is one repeated stretch of an episode, in the JSON output.
type RepeatSpan struct {
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	StartHMS string  `json:"start_hms"`
	Episodes int     `json:"episodes"`
	Text     string  `json:"text"`
}

// RepeatEpisodeResult is one episode's repeats, in the JSON output.
type RepeatEpisodeResult struct {
	Episode     string        `json:"episode"`
	Duration    float64       `json:"duration"`
	RepeatedSec float64       `json:"repeated_sec"`
	Spans       []RepeatSpan  `json:"spans"`
	Score       *detect.Score `json:"score,omitempty"`
}

type repeatShow struct {
	corpus *detect.Corpus
	paths  []string
}

func runRepeatsCommand(cli CLIOptions) error {
	paths := transcriptsIn(cli.Args)
	if len(paths) < 2 {
		return fmt.Errorf("need at least two transcripts to compare, found %d", len(paths))
	}
	opts := detect.RepeatOptions{Shingle: 8, Gap: 6, MinWords: cli.RepeatsMinWords}
	show, err := loadRepeatShow(paths, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(progressFor(cli), "Compared %d episodes.\n", show.corpus.Len())

	switch {
	case cli.RepeatsCurve:
		return printRepeatCurve(cli, show)
	case cli.RepeatsCatalog:
		return printRepeatCatalog(cli, show)
	}
	results := repeatResults(show, cli.RepeatsMinEpisodes)
	if cli.JSON {
		return encodeJSON(cli, results)
	}
	printRepeatResults(cli, results)
	return nil
}

// transcriptsIn expands directories to the transcripts they hold.
func transcriptsIn(args []string) []string {
	var out []string
	for _, arg := range uniquePaths(args) {
		if fi, err := os.Stat(arg); err == nil && fi.IsDir() {
			matches, _ := filepath.Glob(filepath.Join(arg, "*.transcript.json"))
			sort.Strings(matches)
			out = append(out, matches...)
			continue
		}
		out = append(out, arg)
	}
	return out
}

func loadRepeatShow(paths []string, opts detect.RepeatOptions) (*repeatShow, error) {
	show := &repeatShow{corpus: detect.NewCorpus(opts)}
	for _, path := range paths {
		td, err := pipeline.LoadTranscriptFile(path)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(path), ".transcript.json")
		show.corpus.Add(name, detect.WordsOf(td))
		show.paths = append(show.paths, path)
	}
	return show, nil
}

func repeatResults(show *repeatShow, minEpisodes int) []RepeatEpisodeResult {
	results := make([]RepeatEpisodeResult, 0, show.corpus.Len())
	for id := 0; id < show.corpus.Len(); id++ {
		results = append(results, episodeRepeats(show, id, minEpisodes))
	}
	return results
}

func episodeRepeats(show *repeatShow, id, minEpisodes int) RepeatEpisodeResult {
	words := show.corpus.Words(id)
	res := RepeatEpisodeResult{Episode: show.corpus.Name(id)}
	if len(words) > 0 {
		res.Duration = words[len(words)-1].End
	}
	var kept []detect.Repeat
	for _, r := range show.corpus.Repeats(id, nil) {
		if r.Episodes+1 < minEpisodes {
			continue
		}
		kept = append(kept, r)
		res.RepeatedSec += r.End - r.Start
		res.Spans = append(res.Spans, RepeatSpan{
			Start: r.Start, End: r.End, StartHMS: format.FormatClock(r.Start),
			Episodes: r.Episodes, Text: r.Text,
		})
	}
	if truth, ok, err := pipeline.LoadTruth(show.paths[id]); ok && err == nil {
		score := detect.ScoreSegments(detect.RepeatSegments(kept), truth.Segments)
		res.Score = &score
	}
	return res
}

func encodeJSON(cli CLIOptions, v any) error {
	enc := json.NewEncoder(outFor(cli))
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
