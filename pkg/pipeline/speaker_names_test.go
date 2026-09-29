package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/types"
)

func diarizedTranscript(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Some Show")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"text":      "x",
		"speakers":  []string{"SPEAKER_00", "SPEAKER_01", "SPEAKER_02"},
		"diarized":  true,
		"id3_title": "kept",
		"segments": []map[string]any{
			{"start": 0, "end": 5, "speaker": "SPEAKER_00", "text": "Welcome, I am Elad Simchayoff and this is the show, with a long enough sentence."},
			{"start": 5, "end": 9, "speaker": "SPEAKER_01", "text": "Thanks Elad, I am Dana Levy and I am glad to be here on the show today."},
			{"start": 9, "end": 12, "speaker": "SPEAKER_02", "text": "Elad again but split into a second label by the software here."},
		},
	}
	data, _ := json.Marshal(doc)
	path := filepath.Join(dir, "ep.transcript.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNamingByHandMergesLabelsAndKeepsTheRestOfTheFile(t *testing.T) {
	path := diarizedTranscript(t)
	res, err := NameSpeakers(NameSpeakersRequest{
		Path: path, Markdown: true,
		Set: map[string]string{"SPEAKER_00": "Elad", "SPEAKER_02": "Elad", "SPEAKER_01": "Dana Levy"},
	}, types.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 2 {
		t.Fatalf("written %v", res.Written)
	}
	raw, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["id3_title"] != "kept" || doc["speaker_names"] == nil || doc["diarized"] != true {
		t.Fatalf("the file lost fields: %v", doc)
	}
	md, _ := os.ReadFile(strings.TrimSuffix(path, ".transcript.json") + ".transcript.md")
	if !strings.Contains(string(md), "2 speakers: Elad, Dana Levy") || strings.Contains(string(md), "SPEAKER_") {
		t.Fatalf("readable transcript:\n%s", md)
	}
	if strings.Count(string(md), "**Dana Levy**") != 1 {
		t.Fatalf("readable transcript:\n%s", md)
	}
}

func TestNamingByHandRejectsALabelTheTranscriptLacks(t *testing.T) {
	_, err := NameSpeakers(NameSpeakersRequest{Path: diarizedTranscript(t), Set: map[string]string{"SPEAKER_09": "X"}}, types.Config{}, nil)
	if err == nil || !strings.Contains(err.Error(), "SPEAKER_00, SPEAKER_01, SPEAKER_02") {
		t.Fatalf("err = %v", err)
	}
}

func TestClearRemovesTheNamesAndAnEmptyNameRemovesOne(t *testing.T) {
	path := diarizedTranscript(t)
	set := map[string]string{"SPEAKER_00": "Elad", "SPEAKER_01": "Dana"}
	if _, err := NameSpeakers(NameSpeakersRequest{Path: path, Set: set}, types.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := NameSpeakers(NameSpeakersRequest{Path: path, Set: map[string]string{"SPEAKER_01": ""}}, types.Config{}, nil)
	if err != nil || len(res.Names) != 1 || res.Names["SPEAKER_00"] != "Elad" {
		t.Fatalf("names %v, err %v", res.Names, err)
	}
	if _, err := NameSpeakers(NameSpeakersRequest{Path: path, Clear: true}, types.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "speaker_names") {
		t.Fatalf("names left in %s", raw)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	path := diarizedTranscript(t)
	before, _ := os.ReadFile(path)
	if _, err := NameSpeakers(NameSpeakersRequest{Path: path, DryRun: true, Set: map[string]string{"SPEAKER_00": "Elad"}}, types.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("a dry run changed the transcript")
	}
}

func TestParseSpeakerNamesDropsUnknownLabelsNullsAndPadding(t *testing.T) {
	labels := []string{"SPEAKER_00", "SPEAKER_01", "SPEAKER_02"}
	reply := "Sure!\n```json\n{\"SPEAKER_00\": \"  Elad   Simchayoff \", \"SPEAKER_01\": null, \"SPEAKER_07\": \"Ghost\", \"SPEAKER_02\": \"Host\"}\n```"
	got, err := parseSpeakerNames(reply, labels)
	if err != nil || len(got) != 2 || got["SPEAKER_00"] != "Elad Simchayoff" || got["SPEAKER_02"] != "Host" {
		t.Fatalf("got %v, err %v", got, err)
	}
	if _, err := parseSpeakerNames("I cannot tell.", labels); err == nil {
		t.Fatal("a reply with no object must be an error")
	}
}

func TestNamingAsksTheModelWithTheEvidenceAndRecordsItsAnswer(t *testing.T) {
	var prompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		prompt = string(body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"choices": [{"message": {"content": "{\"SPEAKER_00\": \"Elad Simchayoff\", \"SPEAKER_01\": \"Dana Levy\", \"SPEAKER_02\": \"Elad Simchayoff\"}"}}]}`)
	}))
	defer srv.Close()
	var cfg types.Config
	cfg.Profiles = []types.LLMProfile{{ID: 1, Name: "fake", URL: srv.URL, Model: "m"}}
	path := diarizedTranscript(t)
	res, err := NameSpeakers(NameSpeakersRequest{Path: path}, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Podcast: Some Show", "SPEAKER_00: 00:05", "I am Elad Simchayoff", "Never guess a name"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if res.Names["SPEAKER_01"] != "Dana Levy" || res.Names["SPEAKER_02"] != "Elad Simchayoff" {
		t.Fatalf("names %v", res.Names)
	}
}
