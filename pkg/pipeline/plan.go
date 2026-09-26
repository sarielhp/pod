package pipeline

import "pod/pkg/types"

// Stage identifies a step in the ad removal pipeline.
type Stage uint8

const (
	StageTranscribe Stage = iota
	StageDetect
	StageCut
)

func (s Stage) String() string {
	switch s {
	case StageTranscribe:
		return "transcribe"
	case StageDetect:
		return "detect"
	case StageCut:
		return "cut"
	default:
		return "unknown"
	}
}

// Artifacts captures the presence of pipeline artifacts on disk for an episode.
type Artifacts struct {
	HasTranscript bool
	HasCuts       bool
	IsClean       bool
}

// ForceOptions specifies which pipeline stages are forced.
type ForceOptions struct {
	Force           bool
	ForceTranscribe bool
	ForceDetect     bool
	Recut           bool
}

// Plan computes the sequence of stages required to process an episode.
func Plan(st types.EpisodeStatusFile, artifacts Artifacts, f ForceOptions) []Stage {
	if f.Recut {
		if artifacts.HasCuts {
			return []Stage{StageCut}
		}
		if artifacts.HasTranscript {
			return []Stage{StageDetect, StageCut}
		}
		return []Stage{StageTranscribe, StageDetect, StageCut}
	}

	if f.Force || f.ForceTranscribe || !artifacts.HasTranscript {
		return []Stage{StageTranscribe, StageDetect, StageCut}
	}

	if f.ForceDetect || !artifacts.HasCuts {
		return []Stage{StageDetect, StageCut}
	}

	if artifacts.IsClean {
		return nil
	}

	return []Stage{StageCut}
}
