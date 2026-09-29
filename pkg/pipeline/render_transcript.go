package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"pod/pkg/episode"
	"pod/pkg/format"
)

// RenderTranscript writes other forms of a transcript from its JSON alone: "md"
// for reading, "srt" for subtitles, "txt" for plain text. The JSON is the record
// and these are only views of it, so they can be made again at any time, with no
// model and no server, and always agree with the names and text it holds. Each is
// written beside the JSON under its name, x.speakers.json giving x.speakers.md.
func RenderTranscript(path string, formats []string) ([]string, error) {
	path, err := TranscriptPathFor(path)
	if err != nil {
		return nil, err
	}
	td, err := LoadTranscriptFile(path)
	if err != nil {
		return nil, err
	}
	duration := TranscriptDuration(td)
	base := strings.TrimSuffix(path, ".json")
	var written []string
	for _, f := range formats {
		var out string
		switch strings.ToLower(strings.TrimSpace(f)) {
		case "md":
			out, err = format.ConvertToReadable(td, transcriptTitle(path), duration, base+".md", true)
		case "srt":
			out, err = format.ConvertJSONToSRT(path, td, base+".srt", true)
		case "txt":
			out, err = format.ConvertJSONToTXT(path, td, duration, base+".txt", true)
		case "json":
			continue
		default:
			return written, fmt.Errorf("unknown format %q (use md, srt or txt)", f)
		}
		if err != nil {
			return written, fmt.Errorf("write %s: %w", f, err)
		}
		written = append(written, out)
	}
	return written, nil
}

// transcriptTitle heads a readable transcript made from a JSON: the episode's
// title when the library recorded one, else the name it is filed under.
func transcriptTitle(path string) string {
	media := mediaPathOf(path)
	if id := episode.LoadIdentity(media); id != nil && id.Title != "" {
		return id.Title
	}
	return filepath.Base(strings.TrimSuffix(media, ".mp3"))
}
