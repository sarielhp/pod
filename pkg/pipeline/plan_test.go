package pipeline

import (
	"reflect"
	"testing"

	"pod/pkg/types"
)

func TestPlan(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		artifacts Artifacts
		force     ForceOptions
		want      []Stage
	}{
		{
			name:      "fresh episode needs all stages",
			artifacts: Artifacts{},
			force:     ForceOptions{},
			want:      []Stage{StageTranscribe, StageDetect, StageCut},
		},
		{
			name: "clean episode with no force needs nothing",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       true,
				IsClean:       true,
			},
			force: ForceOptions{},
			want:  nil,
		},
		{
			name: "clean episode with force transcribe runs all",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       true,
				IsClean:       true,
			},
			force: ForceOptions{ForceTranscribe: true},
			want:  []Stage{StageTranscribe, StageDetect, StageCut},
		},
		{
			name: "clean episode with force runs all",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       true,
				IsClean:       true,
			},
			force: ForceOptions{Force: true},
			want:  []Stage{StageTranscribe, StageDetect, StageCut},
		},
		{
			name: "clean episode with force detect runs detect and cut",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       true,
				IsClean:       true,
			},
			force: ForceOptions{ForceDetect: true},
			want:  []Stage{StageDetect, StageCut},
		},
		{
			name: "transcript exists but missing cuts runs detect and cut",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       false,
				IsClean:       false,
			},
			force: ForceOptions{},
			want:  []Stage{StageDetect, StageCut},
		},
		{
			name: "transcript and cuts exist but unclean runs cut",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       true,
				IsClean:       false,
			},
			force: ForceOptions{},
			want:  []Stage{StageCut},
		},
		{
			name: "recut with cuts runs only cut",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       true,
				IsClean:       true,
			},
			force: ForceOptions{Recut: true},
			want:  []Stage{StageCut},
		},
		{
			name: "recut without cuts but with transcript runs detect and cut",
			artifacts: Artifacts{
				HasTranscript: true,
				HasCuts:       false,
				IsClean:       false,
			},
			force: ForceOptions{Recut: true},
			want:  []Stage{StageDetect, StageCut},
		},
		{
			name: "recut with nothing runs all",
			artifacts: Artifacts{
				HasTranscript: false,
				HasCuts:       false,
				IsClean:       false,
			},
			force: ForceOptions{Recut: true},
			want:  []Stage{StageTranscribe, StageDetect, StageCut},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Plan(types.EpisodeStatusFile{}, tc.artifacts, tc.force)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Plan() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStageString(t *testing.T) {
	t.Parallel()

	if got := StageTranscribe.String(); got != "transcribe" {
		t.Errorf("got %q, want transcribe", got)
	}
	if got := StageDetect.String(); got != "detect" {
		t.Errorf("got %q, want detect", got)
	}
	if got := StageCut.String(); got != "cut" {
		t.Errorf("got %q, want cut", got)
	}
	if got := Stage(99).String(); got != "unknown" {
		t.Errorf("got %q, want unknown", got)
	}
}
