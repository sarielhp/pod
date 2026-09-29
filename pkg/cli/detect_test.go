package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/pipeline"
	"pod/pkg/progress"
	"pod/pkg/types"
)

func TestDetectCommandReportsEmptyRunsInsteadOfPanicking(t *testing.T) {
	orig := detectFileRepeated
	detectFileRepeated = func(pipeline.DetectRequest, int, Config, ProcOptions, progress.Reporter) ([]pipeline.DetectResult, pipeline.DetectStability, error) {
		return nil, pipeline.DetectStability{}, nil
	}
	t.Cleanup(func() { detectFileRepeated = orig })

	var out, errOut bytes.Buffer
	cli := CLIOptions{Args: []string{"episode.transcript.json"}, Out: &out, Err: &errOut}
	err := runDetectCommand(Config{}, cli)
	if err == nil {
		t.Fatalf("expected an error when detection produced no result")
	}
	if !strings.Contains(err.Error(), "1 of 1") {
		t.Errorf("expected the path to be counted as a failure, got: %v", err)
	}
	if !strings.Contains(errOut.String(), "no result") {
		t.Errorf("expected a per-path message on stderr, got: %q", errOut.String())
	}
}

func stubDetection(t *testing.T, path string, segs ...types.AdSegment) {
	t.Helper()
	orig := detectFileRepeated
	detectFileRepeated = func(pipeline.DetectRequest, int, Config, ProcOptions, progress.Reporter) ([]pipeline.DetectResult, pipeline.DetectStability, error) {
		return []pipeline.DetectResult{{TranscriptPath: path, Duration: 600, Segments: segs, Profile: types.LLMProfile{Name: "stub"}}}, pipeline.DetectStability{}, nil
	}
	t.Cleanup(func() { detectFileRepeated = orig })
}

func TestDetectSaveTruthThenScore(t *testing.T) {
	transcript := filepath.Join(t.TempDir(), "ep.transcript.json")
	stubDetection(t, transcript, types.AdSegment{Start: 0, End: 30, Reason: "plug"})

	var out bytes.Buffer
	cli := CLIOptions{Args: []string{transcript}, Out: &out}
	cli.DetectSaveTruth = true
	if err := runDetectCommand(Config{}, cli); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pipeline.TruthPathFor(transcript)); err != nil {
		t.Fatalf("truth file not written: %v", err)
	}

	stubDetection(t, transcript, types.AdSegment{Start: 0, End: 30}, types.AdSegment{Start: 100, End: 130})
	out.Reset()
	cli.DetectSaveTruth = false
	if err := runDetectCommand(Config{}, cli); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"precision 50.0%", "recall 100.0%", "00:30 of programme wrongly cut"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestDetectUnlabelledEpisodeHasNoScore(t *testing.T) {
	transcript := filepath.Join(t.TempDir(), "ep.transcript.json")
	stubDetection(t, transcript, types.AdSegment{Start: 0, End: 30})
	var out bytes.Buffer
	if err := runDetectCommand(Config{}, CLIOptions{Args: []string{transcript}, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "vs truth") {
		t.Errorf("an unlabelled episode must not print a score:\n%s", out.String())
	}
}
