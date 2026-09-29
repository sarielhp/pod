package format

import (
	"strings"
	"testing"

	"pod/pkg/types"
)

func speech(start, end float64, speaker, text string) types.TranscriptionSegment {
	return types.TranscriptionSegment{Start: start, End: end, Speaker: speaker, Text: text}
}

func TestReadableGroupsSegmentsIntoSpeakerTurns(t *testing.T) {
	data := &types.TranscriptionData{
		Language: "en",
		Speakers: []string{"SPEAKER_00", "SPEAKER_01"},
		Segments: []types.TranscriptionSegment{
			speech(0, 2, "SPEAKER_00", "Welcome back."),
			speech(2.2, 4, "SPEAKER_00", "Today we have a guest."),
			speech(4.5, 6, "SPEAKER_01", "Thanks for having me."),
			speech(6.1, 8, "", "It is a pleasure."),
			speech(12, 14, "SPEAKER_01", "After a long pause."),
		},
	}
	out := FormatReadable(data, "Show", 20)
	for _, want := range []string{
		"# Show",
		"2 speakers: SPEAKER_00, SPEAKER_01",
		"**SPEAKER_00** · 00:00\n\nWelcome back. Today we have a guest.",
		"**SPEAKER_01** · 00:05\n\nThanks for having me. It is a pleasure.\n\nAfter a long pause.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "**SPEAKER_01**") != 1 {
		t.Errorf("an unlabelled segment must continue the turn before it:\n%s", out)
	}
}

func TestReadableWithoutSpeakersStillBreaksAtPauses(t *testing.T) {
	data := &types.TranscriptionData{Segments: []types.TranscriptionSegment{
		speech(0, 2, "", "One."), speech(2.1, 3, "", "Two."), speech(9, 10, "", "Three."),
	}}
	out := FormatReadable(data, "T", 10)
	if !strings.Contains(out, "One. Two.\n\nThree.") || strings.Contains(out, "**") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestSubtitlesLeadWithTheSpeaker(t *testing.T) {
	data := &types.TranscriptionData{Segments: []types.TranscriptionSegment{speech(0, 1, "SPEAKER_00", " Hi ")}}
	if got := formatSRT(data); !strings.Contains(got, "\nSPEAKER_00: Hi\n") {
		t.Fatalf("srt = %q", got)
	}
	if got := formatTXT(data, 1, "x"); !strings.Contains(got, "] SPEAKER_00: Hi\n") {
		t.Fatalf("txt = %q", got)
	}
}
