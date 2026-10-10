package adremoval

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/audio"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/pipeline"
	"pod/pkg/types"
)

func TestGeminiFinalizationCutsRecordedBoilerplate(t *testing.T) {
	t.Parallel()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
	cases := []struct {
		name     string
		phrases  []config.BoilerplatePhrase
		ads      []types.AdSegment
		wantCut  float64
		wantCuts int
	}{
		{"boilerplate without Gemini ads", []config.BoilerplatePhrase{{Text: bpRead, Episodes: 9}}, nil, 30, 1},
		{"overlapping Gemini ad", []config.BoilerplatePhrase{{Text: bpRead, Episodes: 9}}, []types.AdSegment{{Start: 60, End: 80, Reason: "ad"}}, 40, 1},
		{"disabled boilerplate", []config.BoilerplatePhrase{{Text: bpRead, Episodes: 9, Disabled: true}}, nil, 0, 0},
		{"no recorded boilerplate", nil, nil, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := config.SavePodcastConfig(dir, config.PodcastConfig{Boilerplate: tc.phrases}); err != nil {
				t.Fatal(err)
			}
			mp3 := filepath.Join(dir, "ep.mp3")
			silentWav(t, mp3, 110)
			original, err := os.ReadFile(mp3)
			if err != nil {
				t.Fatal(err)
			}
			td := &types.TranscriptionData{Text: bpRead + " " + bpUnique("open", 40), Segments: []types.TranscriptionSegment{
				{Start: 0, End: 40, Text: bpUnique("open", 40)},
				{Start: 40, End: 70, Text: bpRead},
				{Start: 70, End: 110, Text: bpUnique("close", 40)},
			}}
			job := episodeJob{
				sourceAudioFile: mp3,
				mainMP3File:     mp3,
				precutFile:      mp3 + ".precut",
				outputFile:      mp3,
				jsonFile:        filepath.Join(dir, "ep.transcript.json"),
				totalDuration:   110,
				opts:            types.ProcOptions{Quiet: true},
				fileStartTime:   time.Now(),
			}
			if err := pipeline.SaveJSONTranscript(mp3, td, job.jsonFile, true, nil); err != nil {
				t.Fatal(err)
			}
			if !finalizeGeminiEpisode(job, td, tc.ads, time.Now()) {
				t.Fatal("Gemini finalization failed")
			}
			if got := audio.GetAudioDuration(mp3); math.Abs(got-(110-tc.wantCut)) > 1.5 {
				t.Errorf("cleaned duration = %.1f, want %.1f", got, 110-tc.wantCut)
			}
			data, err := os.ReadFile(filepath.Join(dir, "ep.cuts.json"))
			if err != nil {
				t.Fatal(err)
			}
			var cuts types.CutsData
			if err := json.Unmarshal(data, &cuts); err != nil {
				t.Fatal(err)
			}
			if len(cuts.MergedCutIntervals) != tc.wantCuts || math.Abs(cuts.TotalCutDurationSec-tc.wantCut) > 1 {
				t.Errorf("unexpected cuts: %+v", cuts)
			}
			if !episode.IsEpisodeClean(mp3) {
				t.Error("completed episode should be marked clean")
			}
			originalPath := mp3
			if tc.wantCut > 0 {
				originalPath = job.precutFile
			}
			preserved, err := os.ReadFile(originalPath)
			if err != nil || !bytes.Equal(preserved, original) {
				t.Errorf("original audio was not preserved: %v", err)
			}
		})
	}
}
