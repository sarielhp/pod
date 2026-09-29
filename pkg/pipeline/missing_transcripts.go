package pipeline

import (
	"os"
	"sort"

	"pod/pkg/util"
)

// MissingTranscript is a downloaded episode that has no transcript yet.
type MissingTranscript struct {
	// Audio is the episode's audio file, the name its outputs are based on.
	Audio string
	// Source is the audio to transcribe. For an episode already cut it is the
	// uncut original, so the transcript's timestamps agree with the cuts and with
	// any later recut; otherwise it is Audio itself.
	Source string
	// Transcript is where the transcript belongs.
	Transcript string
	Bytes      int64
}

// FindMissingTranscripts lists the episodes in the given podcast directories
// that have no transcript, newest first. A transcript file that is empty counts
// as missing, since one interrupted mid-write is worthless.
func FindMissingTranscripts(dirs []string) []MissingTranscript {
	var missing []MissingTranscript
	seen := map[string]bool{}
	for _, dir := range dirs {
		for _, audio := range util.FindMP3Files(dir) {
			if seen[audio] {
				continue
			}
			seen[audio] = true
			if m, ok := missingTranscriptFor(audio); ok {
				missing = append(missing, m)
			}
		}
	}
	sortNewestFirst(missing)
	return missing
}

func missingTranscriptFor(audio string) (MissingTranscript, bool) {
	transcript := util.StripExt(audio) + ".transcript.json"
	if fi, err := os.Stat(transcript); err == nil && fi.Size() > 0 {
		return MissingTranscript{}, false
	}
	m := MissingTranscript{Audio: audio, Source: audio, Transcript: transcript}
	if precut := audio + ".precut"; util.FileExists(precut) {
		m.Source = precut
	}
	if fi, err := os.Stat(m.Source); err == nil {
		m.Bytes = fi.Size()
	}
	return m, true
}

func sortNewestFirst(items []MissingTranscript) {
	mtime := make(map[string]int64, len(items))
	for _, m := range items {
		if fi, err := os.Stat(m.Audio); err == nil {
			mtime[m.Audio] = fi.ModTime().UnixNano()
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if mtime[items[i].Audio] != mtime[items[j].Audio] {
			return mtime[items[i].Audio] > mtime[items[j].Audio]
		}
		return items[i].Audio > items[j].Audio
	})
}
