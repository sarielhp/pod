package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/pipeline"
	"pod/pkg/types"
)

const repeatedRead = "this episode is brought to you by acme widgets the finest widgets money can buy visit acme dot com slash pod and use code pod for ten percent off your first order today"

func uniqueWords(prefix string, episode, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("%s%de%d", prefix, episode, i)
	}
	return strings.Join(words, " ")
}

func writeShow(t *testing.T) string {
	t.Helper()
	return writeShowOf(t, 3, 3)
}

// writeShowOf writes episodes transcripts, the first withRead of which carry the
// same sponsor read among otherwise unique text.
func writeShowOf(t *testing.T, episodes, withRead int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < episodes; i++ {
		td := types.TranscriptionData{Segments: []types.TranscriptionSegment{
			{Start: 0, End: 30, Text: uniqueWords("open", i, 30)},
			{Start: 60, End: 90, Text: uniqueWords("close", i, 30)},
		}}
		if i < withRead {
			td.Segments = append(td.Segments[:1], append([]types.TranscriptionSegment{{Start: 30, End: 60, Text: repeatedRead}}, td.Segments[1:]...)...)
		}
		data, _ := json.Marshal(td)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("ep%d.transcript.json", i)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRepeatsReportsSharedReadAndScoresAgainstTruth(t *testing.T) {
	dir := writeShow(t)
	if _, err := pipeline.SaveTruth(filepath.Join(dir, "ep0.transcript.json"), "test", []types.AdSegment{{Start: 30, End: 60}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runRepeatsCommand(CLIOptions{Args: []string{dir}, Out: &out, ProcOptions: ProcOptions{Quiet: true}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "3 episodes") || !strings.Contains(out.String(), "P 100.0%  R 100.0%") {
		t.Errorf("unexpected report:\n%s", out.String())
	}
}

func TestRepeatsCatalogGroupsRecurringText(t *testing.T) {
	dir := writeShow(t)
	var out bytes.Buffer
	cli := CLIOptions{Args: []string{dir}, Out: &out, ProcOptions: ProcOptions{Quiet: true}}
	cli.RepeatsCatalog = true
	if err := runRepeatsCommand(cli); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1 distinct recurring texts") || !strings.Contains(out.String(), "3 eps") {
		t.Errorf("the one shared read should appear in all 3 episodes:\n%s", out.String())
	}
}

func TestRepeatsNeedsTwoTranscripts(t *testing.T) {
	dir := t.TempDir()
	if err := runRepeatsCommand(CLIOptions{Args: []string{dir}}); err == nil {
		t.Fatal("an empty directory cannot be compared")
	}
}
