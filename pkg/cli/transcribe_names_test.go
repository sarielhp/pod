package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSpeakerAssignments(t *testing.T) {
	got, err := parseSpeakerAssignments(" SPEAKER_00=Elad , SPEAKER_01=Dana Levy,SPEAKER_02= ")
	want := map[string]string{"SPEAKER_00": "Elad", "SPEAKER_01": "Dana Levy", "SPEAKER_02": ""}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, err %v", got, err)
	}
	if _, err := parseSpeakerAssignments("SPEAKER_00 Elad"); err == nil {
		t.Fatal("a missing = must be an error")
	}
}

func TestSplitTranscriptArgsSeparatesMadeTranscriptsFromMedia(t *testing.T) {
	transcripts, media := splitTranscriptArgs([]string{"a.transcript.json", "b.mp3", "a.transcript.json", "c.mkv"})
	if !reflect.DeepEqual(transcripts, []string{"a.transcript.json"}) || !reflect.DeepEqual(media, []string{"b.mp3", "c.mkv"}) {
		t.Fatalf("transcripts %v, media %v", transcripts, media)
	}
}

func TestATranscriptNeedsAnInstructionToBeRenamed(t *testing.T) {
	err := nameExistingTranscripts(Config{}, CLIOptions{}, []string{"ep.transcript.json"})
	if err == nil || !strings.Contains(err.Error(), "--speakers") {
		t.Fatalf("err = %v", err)
	}
	var cli CLIOptions
	cli.SpeakersClear = true
	cli.SpeakersSet = "SPEAKER_00=x"
	if err := nameExistingTranscripts(Config{}, cli, []string{"ep.transcript.json"}); err == nil {
		t.Fatal("--clear-names with --names must be refused")
	}
}
