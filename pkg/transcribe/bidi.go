package transcribe

import (
	"strings"

	"pod/pkg/types"
)

// isBidiControl reports the invisible characters that set text direction:
// the left/right marks, the embedding and override controls, and the isolates.
func isBidiControl(r rune) bool {
	return r == 0x200E || r == 0x200F || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

func withoutBidiControls(s string) string {
	if !strings.ContainsFunc(s, isBidiControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isBidiControl(r) {
			return -1
		}
		return r
	}, s)
}

// stripBidiControls removes direction controls from a transcript. A Hebrew
// model (ivrit-ai) wraps some phrases in U+202B, which nothing asked for: they
// are invisible, but they split a word in two for anything that compares or
// searches text, and they make a phrase paste backwards. The words and their
// order carry the meaning; direction is the display's business.
func stripBidiControls(td *types.TranscriptionData) {
	td.Text = withoutBidiControls(td.Text)
	for i := range td.Segments {
		seg := &td.Segments[i]
		seg.Text = withoutBidiControls(seg.Text)
		for j := range seg.Words {
			seg.Words[j].Word = withoutBidiControls(seg.Words[j].Word)
		}
	}
}
