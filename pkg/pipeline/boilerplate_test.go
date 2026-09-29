package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/config"
	"pod/pkg/detect"
	"pod/pkg/types"
)

const standingRead = "this show is brought to you by acme widgets the finest widgets money can buy visit acme dot com slash pod and use code pod for ten percent off your first order today"

func uniqueText(tag string, episode, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("%s%de%d", tag, episode, i)
	}
	return strings.Join(words, " ")
}

func writeEpisode(t *testing.T, dir string, i int, withRead bool) {
	t.Helper()
	td := types.TranscriptionData{Segments: []types.TranscriptionSegment{
		{Start: 0, End: 40, Text: uniqueText("open", i, 40)},
	}}
	if withRead {
		td.Segments = append(td.Segments, types.TranscriptionSegment{Start: 40, End: 70, Text: standingRead})
	}
	td.Segments = append(td.Segments, types.TranscriptionSegment{Start: 70, End: 110, Text: uniqueText("close", i, 40)})
	data, _ := json.Marshal(td)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("ep%02d.transcript.json", i)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func showWith(t *testing.T, episodes, withRead int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < episodes; i++ {
		writeEpisode(t, dir, i, i < withRead)
	}
	return dir
}

func TestAnalyzeRecordsPhrasesRecurringEnoughAndNothingElse(t *testing.T) {
	dir := showWith(t, 12, 8)
	res, err := AnalyzePodcast(dir, AnalyzeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Phrases) != 1 || res.Phrases[0].Episodes != 8 || !strings.HasPrefix(res.Phrases[0].Text, "this show is brought") {
		t.Fatalf("want the one standing read, in 8 episodes, got %+v", res.Phrases)
	}
	saved := config.LoadPodcastConfig(dir, config.PodcastConfig{})
	if len(saved.Boilerplate) != 1 {
		t.Errorf("phrases should be saved in podcast.json, got %+v", saved.Boilerplate)
	}

	few := showWith(t, 12, 4)
	res, err = AnalyzePodcast(few, AnalyzeOptions{})
	if err != nil || len(res.Phrases) != 0 {
		t.Errorf("a read in only 4 episodes must not be recorded: %+v (err %v)", res.Phrases, err)
	}
}

func TestAnalyzeDryRunAndSmallShows(t *testing.T) {
	dir := showWith(t, 12, 8)
	if _, err := AnalyzePodcast(dir, AnalyzeOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, config.PodcastConfigFileName)); err == nil {
		t.Error("a dry run must not write podcast.json")
	}
	small, err := AnalyzePodcast(showWith(t, 5, 5), AnalyzeOptions{})
	if err != nil || small.Skipped == "" {
		t.Errorf("a show with too few transcripts should be skipped, got %+v (err %v)", small, err)
	}
}

func TestReanalysisKeepsDisabledAndManualEntries(t *testing.T) {
	t.Parallel()
	opts := detect.RepeatOptions{}
	existing := []config.BoilerplatePhrase{
		{Text: standingRead, Disabled: true},
		{Text: "a hand written phrase that names something specific to skip every single week", Manual: true},
		{Text: "an automatic entry the analysis no longer finds anywhere in the whole catalogue today", Episodes: 9},
	}
	found := []detect.Phrase{{Text: strings.Replace(standingRead, "finest", "very finest", 1), Episodes: 10, Position: 0.5}}

	merged, added, dropped := mergePhrases(existing, found, opts)
	if added != 0 || dropped != 1 || len(merged) != 2 {
		t.Fatalf("want 0 added, 1 dropped, 2 kept; got %d added, %d dropped, %+v", added, dropped, merged)
	}
	if !merged[0].Disabled || merged[0].Episodes != 10 || !merged[1].Manual {
		t.Errorf("the reworded read should inherit disabled and take the fresh count, got %+v", merged[0])
	}
}

func TestBoilerplateCutsAndTranscriptWithoutThem(t *testing.T) {
	dir := showWith(t, 12, 8)
	if _, err := AnalyzePodcast(dir, AnalyzeOptions{}); err != nil {
		t.Fatal(err)
	}
	newEpisode := types.TranscriptionData{Segments: []types.TranscriptionSegment{
		{Start: 0, End: 30, Text: uniqueText("fresh", 99, 30)},
		{Start: 30, End: 60, Text: standingRead},
		{Start: 60, End: 90, Text: uniqueText("more", 99, 30)},
	}}
	cuts := BoilerplateCuts(dir, &newEpisode)
	if len(cuts) != 1 || cuts[0].Start < 25 || cuts[0].Start > 35 || cuts[0].Reason != "boilerplate" {
		t.Fatalf("want one cut around 30-60s, got %+v", cuts)
	}
	rest := WithoutSpans(&newEpisode, cuts)
	if len(rest.Segments) != 2 || len(newEpisode.Segments) != 3 {
		t.Errorf("the cut segment should be dropped from a copy only, got %d (original %d)", len(rest.Segments), len(newEpisode.Segments))
	}
}

func TestBoilerplateCutsHonoursDisabledAndMissingConfig(t *testing.T) {
	dir := showWith(t, 12, 8)
	td := &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 0, End: 30, Text: standingRead}}}
	if cuts := BoilerplateCuts(dir, td); len(cuts) != 0 {
		t.Errorf("a show that was never analysed has no boilerplate, got %+v", cuts)
	}
	cfg := config.PodcastConfig{Boilerplate: []config.BoilerplatePhrase{{Text: standingRead, Disabled: true}}}
	if err := config.SavePodcastConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if cuts := BoilerplateCuts(dir, td); len(cuts) != 0 {
		t.Errorf("a disabled phrase must not cut, got %+v", cuts)
	}
}

// overlappingVariants returns two distinct 40-word phrases where the second
// shares 70% of its word runs with the first, as variants of one read do.
func overlappingVariants() (string, string) {
	words := func(prefix string, from, to int) []string {
		var out []string
		for i := from; i < to; i++ {
			out = append(out, fmt.Sprintf("%s%d", prefix, i))
		}
		return out
	}
	a := strings.Join(words("w", 0, 40), " ")
	b := strings.Join(append(words("w", 10, 40), words("b", 0, 10)...), " ")
	return a, b
}

func TestRerunOverOverlappingVariantsChangesNothing(t *testing.T) {
	t.Parallel()
	a, b := overlappingVariants()
	if detect.PhraseOverlap(a, b, detect.RepeatOptions{}) < 0.5 {
		t.Fatal("test premise: the variants must resemble each other")
	}
	found := []detect.Phrase{{Text: a, Episodes: 30}, {Text: b, Episodes: 20}}
	existing := []config.BoilerplatePhrase{{Text: a, Episodes: 30}, {Text: b, Episodes: 20}}

	merged, added, dropped := mergePhrases(existing, found, detect.RepeatOptions{})
	if added != 0 || dropped != 0 || len(merged) != 2 {
		t.Fatalf("a rerun over unchanged data must add and drop nothing, got %d added, %d dropped, %+v", added, dropped, merged)
	}
}

func TestDisabledFlagStaysOnItsOwnVariant(t *testing.T) {
	t.Parallel()
	a, b := overlappingVariants()
	found := []detect.Phrase{{Text: a, Episodes: 30}, {Text: b, Episodes: 20}}
	existing := []config.BoilerplatePhrase{{Text: a, Episodes: 30}, {Text: b, Episodes: 20, Disabled: true}}

	merged, _, _ := mergePhrases(existing, found, detect.RepeatOptions{})
	if merged[0].Disabled || !merged[1].Disabled {
		t.Errorf("only the second variant was disabled, got %+v", merged)
	}
}

func TestAnalyzingTwiceIsIdempotent(t *testing.T) {
	dir := showWith(t, 12, 8)
	first, err := AnalyzePodcast(dir, AnalyzeOptions{})
	if err != nil || first.Added != 1 {
		t.Fatalf("first run: %+v (err %v)", first, err)
	}
	second, err := AnalyzePodcast(dir, AnalyzeOptions{})
	if err != nil || second.Added != 0 || second.Dropped != 0 || len(second.Phrases) != 1 {
		t.Errorf("second run should change nothing: %+v (err %v)", second, err)
	}
}
