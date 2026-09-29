package transcribe

import (
	"testing"

	"pod/pkg/types"
)

func TestStripBidiControlsKeepsTheWordsAndDropsTheMarks(t *testing.T) {
	const marked = "\u202b\u05d4\u05d9\u05d5\u05dd \u05d9\u05d5\u05dd\u200f \u05e8\u05d1\u05d9\u05e2\u05d9\u202c"
	const clean = "\u05d4\u05d9\u05d5\u05dd \u05d9\u05d5\u05dd \u05e8\u05d1\u05d9\u05e2\u05d9"
	td := &types.TranscriptionData{
		Text: marked,
		Segments: []types.TranscriptionSegment{{
			Text:  marked,
			Words: []types.TranscriptionWord{{Word: "\u202b\u05d4\u05d9\u05d5\u05dd"}, {Word: "plain"}},
		}},
	}
	stripBidiControls(td)
	if td.Text != clean || td.Segments[0].Text != clean || td.Segments[0].Words[0].Word != "\u05d4\u05d9\u05d5\u05dd" || td.Segments[0].Words[1].Word != "plain" {
		t.Fatalf("got %+v", td)
	}
}
