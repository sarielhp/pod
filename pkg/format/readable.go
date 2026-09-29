package format

import (
	"fmt"
	"os"
	"strings"

	"pod/pkg/types"
	"pod/pkg/util"
)

const (
	paragraphPauseSec = 1.5
	paragraphMaxChars = 700
)

type speakerTurn struct {
	speaker    string
	start      float64
	paragraphs []string
}

// FormatReadable renders a transcript as Markdown for reading: one block per
// turn of speech, headed by who is speaking and when, with the turn broken into
// paragraphs at pauses. A transcript with no speaker labels reads as a single
// unlabelled run of paragraphs, still broken at pauses.
func FormatReadable(data *types.TranscriptionData, title string, duration float64) string {
	if data == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "%s\n\n", readableSummary(data, duration))
	for _, turn := range speakerTurns(data.Segments) {
		if turn.speaker != "" {
			fmt.Fprintf(&b, "**%s** · %s\n\n", turn.speaker, FormatClock(turn.start))
		} else {
			fmt.Fprintf(&b, "*%s*\n\n", FormatClock(turn.start))
		}
		for _, p := range turn.paragraphs {
			fmt.Fprintf(&b, "%s\n\n", p)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func readableSummary(data *types.TranscriptionData, duration float64) string {
	parts := []string{FormatClock(duration)}
	if data.Language != "" {
		parts = append(parts, strings.ToUpper(data.Language))
	}
	if len(data.Speakers) > 0 {
		parts = append(parts, fmt.Sprintf("%d speakers: %s", len(data.Speakers), strings.Join(data.Speakers, ", ")))
	}
	return "_" + strings.Join(parts, " · ") + "_"
}

// speakerTurns groups consecutive segments by speaker. A segment with no label
// continues the turn before it, since the engine gave it nobody else. Within a
// turn a paragraph ends at a pause or when it has grown long enough to want one.
func speakerTurns(segs []types.TranscriptionSegment) []speakerTurn {
	var turns []speakerTurn
	var current strings.Builder
	var lastEnd float64
	flush := func() {
		if text := strings.TrimSpace(current.String()); text != "" && len(turns) > 0 {
			turns[len(turns)-1].paragraphs = append(turns[len(turns)-1].paragraphs, text)
		}
		current.Reset()
	}
	for _, seg := range segs {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		newTurn := len(turns) == 0 || (seg.Speaker != "" && seg.Speaker != turns[len(turns)-1].speaker)
		if newTurn {
			flush()
			turns = append(turns, speakerTurn{speaker: seg.Speaker, start: seg.Start})
		} else if seg.Start-lastEnd >= paragraphPauseSec || current.Len() >= paragraphMaxChars {
			flush()
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(text)
		lastEnd = seg.End
	}
	flush()
	return turns
}

// ConvertToReadable writes the Markdown transcript to path.
func ConvertToReadable(data *types.TranscriptionData, title string, duration float64, path string, quiet bool) (string, error) {
	if data == nil {
		return "", fmt.Errorf("no transcript to write")
	}
	if err := util.WriteFileAtomic(path, []byte(FormatReadable(data, title, duration)), 0644); err != nil {
		return "", err
	}
	if !quiet {
		fmt.Fprintf(os.Stdout, "Saved readable transcript (.md) to: '%s'\n", path)
	}
	return path, nil
}
