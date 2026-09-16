package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/format"
	"pod/pkg/pipeline"
)

func buildDetectCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "detect",
		Description: "Detect ad segments in an existing transcript",
		UsageLine:   "pod detect [options] <path...>",
		Parameters: []clihelp.Param{
			{Name: "<path...>", Description: "Transcript JSON files, or media files whose transcript sits beside them"},
		},
		Args: clihelp.MinimumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.UseLLM, "--profile <id/name>", "", "LLM profile to detect with (default: the active one)"),
			clihelp.Bool(&opts.DetectRaw, "--raw", false, "Report segments as the model returned them, unmerged"),
			clihelp.Bool(&opts.DetectWriteCuts, "--write-cuts", false, "Save a .cuts.json beside the transcript"),
			clihelp.Int(&opts.DetectRepeat, "-n, --repeat <count>", 1, "Detect this many times and report how much the runs agree"),
			clihelp.Bool(&opts.JSON, "--json", false, "Emit the segments as JSON"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod detect lecture.transcript.json", Description: "Detect ads in a transcript you already have"},
			{Line: "pod detect --profile 3 ep.mp3", Description: "Use a specific LLM profile on the episode's transcript"},
			{Line: "pod detect --raw --json ep.mp3", Description: "See exactly what the model returned, before merging"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "detect"
			opts.Args = ctx.Args
			return nil
		},
	}
}

// DetectedSegment is one reported ad segment. It is the JSON shape of
// `pod detect --json`, kept separate from the internal type so the output
// format does not drift whenever the pipeline's own types change.
type DetectedSegment struct {
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	Duration float64 `json:"duration"`
	StartHMS string  `json:"start_hms"`
	EndHMS   string  `json:"end_hms"`
	Reason   string  `json:"reason,omitempty"`
}

// DetectStabilityResult reports how much repeated runs agreed. It is absent
// from the output of a single run, where there is nothing to compare.
type DetectStabilityResult struct {
	Runs          int       `json:"runs"`
	SegmentCounts []int     `json:"segment_counts"`
	AdTimes       []float64 `json:"ad_times"`
	MinAgreement  float64   `json:"min_agreement"`
	MeanAgreement float64   `json:"mean_agreement"`
	Identical     int       `json:"identical_runs"`
	Deterministic bool      `json:"deterministic"`
}

// DetectFileResult is the JSON shape of one analysed transcript.
type DetectFileResult struct {
	Transcript string            `json:"transcript"`
	Profile    string            `json:"profile"`
	Model      string            `json:"model"`
	Duration   float64           `json:"duration"`
	AdTime     float64           `json:"ad_time"`
	Segments   []DetectedSegment `json:"segments"`
	CutsFile   string            `json:"cuts_file,omitempty"`

	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`

	Stability *DetectStabilityResult `json:"stability,omitempty"`
}

func detectStability(s pipeline.DetectStability) *DetectStabilityResult {
	return &DetectStabilityResult{
		Runs:          s.Runs,
		SegmentCounts: s.SegmentCounts,
		AdTimes:       s.AdTimes,
		MinAgreement:  s.MinAgreement,
		MeanAgreement: s.MeanAgreement,
		Identical:     s.Identical,
		Deterministic: s.Deterministic(),
	}
}

func runDetectCommand(cfg Config, cli CLIOptions) error {
	opts := cli.ProcOptions
	opts.Normalize()

	var results []DetectFileResult
	var failures []string

	for _, path := range uniquePaths(cli.Args) {
		req := pipeline.DetectRequest{
			Path:      path,
			Profile:   cli.UseLLM,
			NoMerge:   cli.DetectRaw,
			WriteCuts: cli.DetectWriteCuts,
		}
		runs, stability, err := pipeline.DetectFileRepeated(req, cli.DetectRepeat, cfg, opts, reporter(cli))
		if err != nil {
			fmt.Fprintf(errFor(cli), "%v\n", err)
			failures = append(failures, path)
			if len(runs) == 0 {
				continue
			}
		}
		out := detectFileResult(runs[0])
		if stability.Runs > 1 {
			out.Stability = detectStability(stability)
		}
		results = append(results, out)
	}

	if cli.JSON {
		enc := json.NewEncoder(outFor(cli))
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			printDetectResult(cli, r)
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d of %d transcript(s) could not be analysed: %s",
			len(failures), len(cli.Args), strings.Join(failures, ", "))
	}
	return nil
}

func detectFileResult(res pipeline.DetectResult) DetectFileResult {
	out := DetectFileResult{
		Transcript: res.TranscriptPath,
		Profile:    res.Profile.Name,
		Model:      res.Profile.Model,
		Duration:   res.Duration,
		CutsFile:   res.CutsPath,

		PromptTokens:     res.Usage.PromptTokens,
		CachedTokens:     res.Usage.CachedTokens(),
		CompletionTokens: res.Usage.CompletionTokens,
	}
	for _, s := range res.Segments {
		d := s.End - s.Start
		out.AdTime += d
		out.Segments = append(out.Segments, DetectedSegment{
			Start:    s.Start,
			End:      s.End,
			Duration: d,
			StartHMS: format.FormatClock(s.Start),
			EndHMS:   format.FormatClock(s.End),
			Reason:   s.Reason,
		})
	}
	return out
}

func printDetectResult(cli CLIOptions, r DetectFileResult) {
	w := outFor(cli)
	if len(r.Segments) == 0 {
		fmt.Fprintf(w, "%s: no ad segments detected (%s, %s)\n",
			r.Transcript, format.FormatClock(r.Duration), r.Profile)
		return
	}
	fmt.Fprintf(w, "%s: %d ad segment(s), %s of %s (%s)\n",
		r.Transcript, len(r.Segments),
		format.FormatClock(r.AdTime), format.FormatClock(r.Duration), r.Profile)
	for _, s := range r.Segments {
		fmt.Fprintf(w, "  %s → %s  (%s)", s.StartHMS, s.EndHMS, format.FormatClock(s.Duration))
		if s.Reason != "" {
			fmt.Fprintf(w, "  %s", s.Reason)
		}
		fmt.Fprintln(w)
	}
	if r.CutsFile != "" {
		fmt.Fprintf(w, "  wrote %s\n", r.CutsFile)
	}
	printDetectStability(w, r.Stability)
}

func printDetectStability(w io.Writer, s *DetectStabilityResult) {
	if s == nil || s.Runs < 2 {
		return
	}
	counts := make([]string, 0, len(s.SegmentCounts))
	for _, c := range s.SegmentCounts {
		counts = append(counts, strconv.Itoa(c))
	}
	fmt.Fprintf(w, "  %d runs: segments %s\n", s.Runs, strings.Join(counts, "/"))
	if s.Deterministic {
		fmt.Fprintf(w, "  reproducible: every pair of runs agreed exactly\n")
		return
	}
	fmt.Fprintf(w, "  NOT reproducible: %d/%d runs matched the first; agreement min %.0f%%, mean %.0f%%\n",
		s.Identical, s.Runs, s.MinAgreement*100, s.MeanAgreement*100)
}
