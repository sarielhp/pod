package pipeline

import (
	"strings"
	"testing"

	"pod/pkg/types"
)

func TestFormatTranscript(t *testing.T) {
	t.Parallel()
	td := &types.TranscriptionData{
		Segments: []types.TranscriptionSegment{
			{Start: 0.0, End: 5.0, Text: "Hello"},
			{Start: 5.0, End: 10.0, Text: "World"},
		},
	}
	formatted := FormatTranscript(td, 10.0)
	if !strings.Contains(formatted, "[0.0s -> 5.0s] Hello") {
		t.Errorf("unexpected formatted transcript: %q", formatted)
	}
}
