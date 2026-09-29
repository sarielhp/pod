package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"pod/pkg/types"
	"pod/pkg/util"
)

// TruthFile is the labelled ad segments of one episode, kept beside its
// transcript so detection can be scored against them. It is plain JSON on
// purpose: the labels usually start as one strong model's answer and are then
// corrected by hand.
type TruthFile struct {
	Version   int               `json:"version"`
	LabeledBy string            `json:"labeled_by,omitempty"`
	Segments  []types.AdSegment `json:"segments"`
}

// TruthPathFor is where the labels for a transcript live.
func TruthPathFor(transcriptPath string) string {
	return strings.TrimSuffix(transcriptPath, ".transcript.json") + ".ads.truth.json"
}

// LoadTruth reads the labels for a transcript. The boolean is false when the
// episode has none, which is not an error: most episodes are unlabelled.
func LoadTruth(transcriptPath string) (TruthFile, bool, error) {
	path := TruthPathFor(transcriptPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return TruthFile{}, false, nil
	}
	if err != nil {
		return TruthFile{}, false, err
	}
	var truth TruthFile
	if err := json.Unmarshal(data, &truth); err != nil {
		return TruthFile{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	return truth, true, nil
}

// SaveTruth writes the labels for a transcript and returns the path.
func SaveTruth(transcriptPath, labeledBy string, segments []types.AdSegment) (string, error) {
	path := TruthPathFor(transcriptPath)
	if segments == nil {
		segments = []types.AdSegment{}
	}
	data, err := json.MarshalIndent(TruthFile{Version: 1, LabeledBy: labeledBy, Segments: segments}, "", "  ")
	if err != nil {
		return "", err
	}
	return path, util.WriteFileAtomic(path, append(data, '\n'), 0o644)
}
