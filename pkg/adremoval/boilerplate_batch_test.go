package adremoval

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"pod/pkg/audio"
	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/types"
	"pod/pkg/util"
)

// silentWav writes seconds of silence as a WAV file, which ffmpeg reads whatever
// the file is named, so a test can have real audio without a fixture on disk.
func silentWav(t *testing.T, path string, seconds int) {
	t.Helper()
	const rate = 8000
	data := make([]byte, seconds*rate*2)
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], rate)
	binary.LittleEndian.PutUint32(h[28:], rate*2)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	if err := os.WriteFile(path, append(h, data...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// batchEpisode makes a processed 110-second episode with the standing read at 40-70s.
// firstCut is the range of an earlier ad cut, or nil for an episode with no cuts file.
func batchEpisode(t *testing.T, dir, stem string, firstCut *types.AdSegment) string {
	t.Helper()
	audioPath := filepath.Join(dir, stem+".mp3")
	silentWav(t, audioPath, 110)
	silentWav(t, audioPath+".precut", 110)
	td := types.TranscriptionData{Text: bpRead + " " + bpUnique("open", 40), Segments: []types.TranscriptionSegment{
		{Start: 0, End: 40, Text: bpUnique("open", 40)},
		{Start: 40, End: 70, Text: bpRead},
		{Start: 70, End: 110, Text: bpUnique("close", 40)},
	}}
	data, _ := json.Marshal(td)
	if err := os.WriteFile(filepath.Join(dir, stem+".transcript.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if firstCut != nil {
		format.SaveCutsJSON(audioPath, 110, []types.AdSegment{*firstCut}, &types.LLMProfile{Name: "Model", Model: "m"}, true)
	}
	return audioPath
}

func TestBoilerplateBatchCountsWhyEpisodesAreLeftAloneAndRecutsTheRest(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	if err := config.SavePodcastConfig(dir, config.PodcastConfig{Boilerplate: []config.BoilerplatePhrase{{Text: bpRead, Episodes: 9}}}); err != nil {
		t.Fatal(err)
	}
	needs := batchEpisode(t, dir, "needs", &types.AdSegment{Start: 5, End: 15, Reason: "ad"})
	covered := batchEpisode(t, dir, "covered", &types.AdSegment{Start: 39.5, End: 70.5, Reason: "ad"})
	batchEpisode(t, dir, "unprocessed", nil)
	targets := []string{covered, needs, filepath.Join(dir, "unprocessed.mp3")}

	dry := RecutBoilerplate(targets, types.ProcOptions{DryRun: true}, types.Config{}, nil)
	if dry.Recut != 1 || dry.Skipped["its cuts already cover the boilerplate"] != 1 || dry.Skipped["it has no cuts file"] != 1 {
		t.Fatalf("unexpected dry-run report %+v", dry)
	}
	if got := audio.GetAudioDuration(needs); math.Abs(got-110) > 1 {
		t.Fatalf("a dry run must not touch the audio, got %.0fs", got)
	}

	real := RecutBoilerplate(targets, types.ProcOptions{}, types.Config{}, nil)
	if real.Recut != 1 || len(real.Failures) != 0 || len(real.Changed) != 1 || real.Changed[0] != needs {
		t.Fatalf("unexpected report %+v", real)
	}
	// 110s minus the earlier ad (10s) and the 30s read.
	if got := audio.GetAudioDuration(needs); math.Abs(got-70) > 1.5 {
		t.Errorf("the recut should leave about 70s of audio, got %.1fs", got)
	}
	if got := audio.GetAudioDuration(needs + ".precut"); math.Abs(got-110) > 1 {
		t.Errorf("the uncut original must be left alone, got %.1fs", got)
	}
	if !util.FileExists(filepath.Join(dir, "needs.cuts.json")) {
		t.Error("the cuts file should still be there")
	}

	again := RecutBoilerplate(targets, types.ProcOptions{}, types.Config{}, nil)
	if again.Recut != 0 || again.Skipped["its cuts already cover the boilerplate"] != 2 {
		t.Errorf("a second run must find nothing to do: %+v", again)
	}
}

func TestBoilerplateBatchLimitStopsAfterThatManyRecuts(t *testing.T) {
	dir := t.TempDir()
	if err := config.SavePodcastConfig(dir, config.PodcastConfig{Boilerplate: []config.BoilerplatePhrase{{Text: bpRead, Episodes: 9}}}); err != nil {
		t.Fatal(err)
	}
	var targets []string
	for _, stem := range []string{"a", "b", "c"} {
		targets = append(targets, batchEpisode(t, dir, stem, &types.AdSegment{Start: 5, End: 15}))
	}
	rep := RecutBoilerplate(targets, types.ProcOptions{DryRun: true, Count: 2}, types.Config{}, nil)
	if rep.Recut != 2 {
		t.Errorf("want the limit of 2 honoured, got %+v", rep)
	}
}
