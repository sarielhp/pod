package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"pod/pkg/pipeline"
	"pod/pkg/util"
)

// parseSpeakerAssignments reads "SPEAKER_00=Elad,SPEAKER_01=Dana".
func parseSpeakerAssignments(list string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(list, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		label, name, ok := strings.Cut(part, "=")
		label = strings.TrimSpace(label)
		if !ok || label == "" {
			return nil, fmt.Errorf("cannot read %q: write it as SPEAKER_00=Name", strings.TrimSpace(part))
		}
		out[label] = strings.TrimSpace(name)
	}
	return out, nil
}

// splitTranscriptArgs separates transcripts already made from the media files
// still to be transcribed.
func splitTranscriptArgs(args []string) (transcripts, media []string) {
	for _, a := range uniquePaths(args) {
		if isTranscriptArg(a) {
			transcripts = append(transcripts, a)
		} else {
			media = append(media, a)
		}
	}
	return transcripts, media
}

// nameExistingTranscripts redoes only the speaker names of transcripts that are
// already made, so a wrong guess is put right without transcribing again.
func nameExistingTranscripts(cfg Config, cli CLIOptions, paths []string) error {
	set, err := parseSpeakerAssignments(cli.SpeakersSet)
	if err != nil {
		return err
	}
	if cli.SpeakersClear && len(set) > 0 {
		return fmt.Errorf("--clear-names and --names say opposite things; use one")
	}
	if !cli.SpeakersClear && !cli.Speakers {
		return fmt.Errorf("%s is already transcribed; add --speakers to name its speakers, --names to name them yourself, or --clear-names to remove the names", filepath.Base(paths[0]))
	}
	var failures []string
	for _, path := range paths {
		res, err := pipeline.NameSpeakers(pipeline.NameSpeakersRequest{
			Path: path, Set: set, Clear: cli.SpeakersClear, Profile: cli.UseLLM,
			DryRun: cli.DryRun, Markdown: true,
		}, cfg, reporter(cli))
		if err != nil {
			util.FprintError(errFor(cli), "%s: %v\n", filepath.Base(path), err)
			failures = append(failures, filepath.Base(path))
			continue
		}
		printSpeakerNames(outFor(cli), cli, res)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d transcript(s) could not be named: %s", len(failures), len(paths), strings.Join(failures, ", "))
	}
	return nil
}

func printSpeakerNames(w io.Writer, cli CLIOptions, res pipeline.NameSpeakersResult) {
	verb := "named"
	if cli.DryRun {
		verb = "would be named"
	}
	fmt.Fprintf(w, "%s: %d of %d speaker(s) %s\n", filepath.Base(res.TranscriptPath), len(res.Names), len(res.Speakers), verb)
	for _, label := range res.Speakers {
		name := res.Names[label]
		if name == "" {
			name = "(no name)"
		}
		fmt.Fprintf(w, "  %-11s %s\n", label, name)
	}
	if !cli.DryRun {
		for _, p := range res.Written {
			fmt.Fprintf(w, "  wrote %s\n", p)
		}
	}
}
