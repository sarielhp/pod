package cli

import (
	"fmt"
	"os"
	"strings"

	"pod/pkg/util"
)

// transcribeTarget is one thing to transcribe. A plain file is its own source.
// A library episode is transcribed from its uncut original when there is one, so
// that times line up with the episode's cuts, and its outputs are named after
// the episode, not after that source.
type transcribeTarget struct {
	source     string
	outputBase string
	lockPath   string
	episodeID  string
}

func (t transcribeTarget) isEpisode() bool { return t.outputBase != "" }

// resolveTranscribeArgs turns the arguments into targets. An argument that is a
// path on disk is a file to transcribe, or a directory to scan for them; anything
// else is looked up as an episode ID or a name in the library, as the other
// commands do.
func resolveTranscribeArgs(cfg Config, args []string) ([]transcribeTarget, error) {
	var files []string
	var episodes []transcribeTarget
	for _, arg := range args {
		if _, err := os.Stat(arg); err == nil {
			files = append(files, arg)
			continue
		}
		t, err := episodeTranscribeTarget(cfg, arg)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, t)
	}
	var out []transcribeTarget
	if len(files) > 0 {
		paths, err := expandTranscribeTargets(files)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			out = append(out, transcribeTarget{source: p, lockPath: p})
		}
	}
	return append(out, episodes...), nil
}

func episodeTranscribeTarget(cfg Config, query string) (transcribeTarget, error) {
	res, err := resolveQueueTarget(cfg.PodcastsDir, query)
	if err != nil {
		return transcribeTarget{}, fmt.Errorf("%s is not a file, and %w", query, err)
	}
	if !res.IsEpisode() {
		return transcribeTarget{}, fmt.Errorf("%s names a podcast; give an episode ID or an audio file", query)
	}
	audio := res.Episode.Path
	source := audio
	if precut := audio + ".precut"; util.FileExists(precut) {
		source = precut
	}
	if !util.FileExists(source) {
		return transcribeTarget{}, fmt.Errorf("[%s] %s has no audio left to transcribe", res.Episode.ShortID, res.Episode.Title)
	}
	return transcribeTarget{source: source, outputBase: util.StripExt(audio), lockPath: audio, episodeID: res.Episode.ShortID}, nil
}

// refuseToReplaceTranscript stops a plain transcription of an episode that
// already has its transcript, since that file is what ad removal and boilerplate
// analysis read. Speaker versions go to their own files and are never refused.
func refuseToReplaceTranscript(t transcribeTarget, speakers bool) error {
	if speakers || !t.isEpisode() {
		return nil
	}
	existing := t.outputBase + ".transcript.json"
	if fi, err := os.Stat(existing); err == nil && fi.Size() > 0 {
		return fmt.Errorf("[%s] already has a transcript (%s); add --speakers for a version with speakers, written beside it", t.episodeID, existing)
	}
	return nil
}

func isTranscriptArg(arg string) bool {
	return strings.HasSuffix(arg, ".transcript.json") || strings.HasSuffix(arg, ".speakers.json")
}
