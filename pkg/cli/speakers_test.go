package cli

import (
	"reflect"
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
