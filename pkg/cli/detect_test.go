package cli

import (
	"bytes"
	"strings"
	"testing"

	"pod/pkg/pipeline"
	"pod/pkg/progress"
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
