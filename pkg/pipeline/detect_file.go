package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/config"
	"pod/pkg/detect"
	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

// DetectRequest is a one-off ad detection over an existing transcript.
//
// It is the second stage of the pipeline made available on its own, in the
// same spirit as TranscribeFile: no audio is read, no library state is
// touched, and the transcript is not modified. That matters because ad
// detection is otherwise only reachable by running a whole transcribe →
// detect → cut cycle, which makes comparing two LLM profiles over the same
// input, or re-running after a prompt change, disproportionately expensive.
type DetectRequest struct {
	// Path is a transcript JSON, or any file whose transcript sits beside it
	// under the usual ".transcript.json" name.
	Path string

	// Profile selects the LLM profile by id, name or unique substring. Empty
	// uses the configured active profile.
	Profile string

	// NoMerge reports the segments exactly as the model returned them instead
	// of merging adjacent intervals. Merging is what the pipeline does before
	// cutting, so the merged form is the default; the raw form is what you
	// want when judging the model rather than the cut.
	NoMerge bool

	// WriteCuts saves a .cuts.json beside the transcript, as a pipeline run
	// would. Off by default: reading should not rewrite the library.
	WriteCuts bool
}

// DetectResult reports what a detection run found.
type DetectResult struct {
	TranscriptPath string
	Profile        types.LLMProfile
	Segments       []types.AdSegment
	RawCount       int
	Duration       float64
	CutsPath       string

	// Usage is what the detection consumed, over every request it made.
	Usage detect.LLMUsage
}

// DetectFile runs ad detection over an existing transcript.
func DetectFile(req DetectRequest, cfg types.Config, opts types.ProcOptions, rep progress.Reporter) (DetectResult, error) {
	r := progress.Or(rep)
	var res DetectResult

	path, err := TranscriptPathFor(req.Path)
	if err != nil {
		return res, err
	}
	res.TranscriptPath = path

	td, err := LoadTranscriptFile(path)
	if err != nil {
		return res, err
	}
	res.Duration = TranscriptDuration(td)
	if res.Duration <= 0 {
		return res, fmt.Errorf("%s has no timed segments to detect over", filepath.Base(path))
	}

	profile, err := config.SelectLLMProfile(&cfg, req.Profile)
	if err != nil {
		return res, err
	}
	res.Profile = profile
	apiKey := config.ResolveLLMAPIKey(profile, &cfg)

	r.Infof("Detecting ads in %s (%s) with %s...",
		filepath.Base(path), format.FormatClock(res.Duration), profile.Name)

	detector := detect.NewLLMAdDetector(profile, apiKey, detect.DefaultLLMTimeout)
	segments, usage, err := detector.DetectAdsUsage(context.Background(), FormatTranscript(td, res.Duration))
	res.Usage = usage
	if err != nil {
		return res, fmt.Errorf("ad detection failed: %w", err)
	}
	if usage.TotalTokens > 0 {
		r.Detailf("tokens: %d in (%d cached), %d out",
			usage.PromptTokens, usage.CachedTokens(), usage.CompletionTokens)
	}

	res.RawCount = len(segments)
	if !req.NoMerge && len(segments) > 0 {
		segments = format.MergeIntervals(segments)
		if merged := res.RawCount - len(segments); merged > 0 {
			r.Detailf("merged %d adjacent interval(s)", merged)
		}
	}
	res.Segments = segments

	if req.WriteCuts {
		res.CutsPath = format.SaveCutsJSON(path, res.Duration, segments, &profile, opts.Quiet).CutsFile
	}
	return res, nil
}

// TranscriptPathFor resolves the transcript belonging to a path, which may be
// the transcript itself or the media file it was made from.
func TranscriptPathFor(path string) (string, error) {
	if strings.HasSuffix(path, ".transcript.json") {
		if !util.FileExists(path) {
			return "", fmt.Errorf("no such transcript: %s", path)
		}
		return path, nil
	}
	candidate := util.StripExt(path) + ".transcript.json"
	if util.FileExists(candidate) {
		return candidate, nil
	}
	if util.FileExists(path) {
		return "", fmt.Errorf("no transcript beside %s (expected %s)",
			filepath.Base(path), filepath.Base(candidate))
	}
	return "", fmt.Errorf("no such file: %s", path)
}

// LoadTranscriptFile reads a saved transcript.
func LoadTranscriptFile(path string) (*types.TranscriptionData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	var td types.TranscriptionData
	if err := json.Unmarshal(data, &td); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &td, nil
}

// TranscriptDuration is how far the transcript reaches, taken from the last
// segment that ends. A transcript records no duration of its own.
func TranscriptDuration(td *types.TranscriptionData) float64 {
	if td == nil {
		return 0
	}
	last := 0.0
	for _, s := range td.Segments {
		if s.End > last {
			last = s.End
		}
	}
	return last
}
