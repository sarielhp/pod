package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/pipeline"
)

func buildTranscribeCommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "transcribe",
		Description: "Transcribe an audio or video file",
		UsageLine:   "pod transcribe [options] <path...>",
		Parameters: []clihelp.Param{
			{Name: "<path...>", Description: "Audio or video files, or directories to scan"},
		},
		Args: clihelp.MinimumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.ExportFormat, "--format <list>", "json", "Output formats: json, srt, txt (comma separated)"),
			clihelp.String(&opts.Output, "-o, --output <dir>", "", "Write transcripts to this directory"),
			clihelp.String(&opts.TranscribeMin, "-t, --tminutes <minutes>", "", "Transcribe only the first N minutes"),
			clihelp.Bool(&opts.KeepAudio, "--keep-audio", false, "Keep the extracted 16kHz audio beside the transcript"),
			clihelp.String(&opts.WhisperEngine, "--whisper-engine <engine>", "", "Engine: local, docker, remote, gemini"),
			clihelp.String(&opts.WhisperModel, "--whisper-model <model>", "", "Model name or alias (e.g. tiny.en, base)"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed output"),
		},
		Examples: []clihelp.Example{
			{Line: "pod transcribe lecture.mkv", Description: "Extract the audio track and transcribe it"},
			{Line: "pod transcribe --format srt,txt talk.mp4", Description: "Produce subtitles and plain text"},
			{Line: "pod transcribe -t 5 long-movie.mkv", Description: "Transcribe only the first five minutes"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "transcribe"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func runTranscribeCommand(cfg Config, cli CLIOptions) error {
	paths, err := expandTranscribeTargets(cli.Args)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no audio or video files found in %s", strings.Join(cli.Args, ", "))
	}

	opts := cli.ProcOptions
	opts.Normalize()

	var failures []string
	for _, path := range paths {
		res, err := pipeline.TranscribeFile(pipeline.TranscribeRequest{
			Path:       path,
			OutputDir:  cli.Output,
			Formats:    strings.Split(cli.ExportFormat, ","),
			MaxMinutes: transcribeMinutes(cli.TranscribeMin),
			KeepAudio:  cli.KeepAudio,
		}, cfg, opts, reporter(cli))
		if err != nil {
			fmt.Fprintf(errFor(cli), "%v\n", err)
			failures = append(failures, filepath.Base(path))
			continue
		}
		for _, w := range res.Written {
			fmt.Fprintf(outFor(cli), "%s\n", w)
		}
		if res.AudioOut != "" {
			fmt.Fprintf(outFor(cli), "%s\n", res.AudioOut)
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d of %d file(s) could not be transcribed: %s",
			len(failures), len(paths), strings.Join(failures, ", "))
	}
	return nil
}

// transcribeMinutes reads the --tminutes value. Anything unparseable or
// non-positive means "the whole file", matching HandleTranscribeMin.
func transcribeMinutes(v string) float64 {
	m, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || m <= 0 {
		return 0
	}
	return m
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		key, err := filepath.Abs(p)
		if err != nil {
			key = p
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// transcribableExts is what ffmpeg is asked to read when scanning a directory.
// A file named explicitly is always attempted, whatever its extension — ffmpeg
// decides, not this list.
var transcribableExts = map[string]bool{
	".mp3": true, ".m4a": true, ".m4b": true, ".aac": true, ".flac": true,
	".ogg": true, ".opus": true, ".wav": true, ".wma": true,
	".mp4": true, ".mkv": true, ".mov": true, ".avi": true, ".webm": true,
	".m4v": true, ".mpg": true, ".mpeg": true, ".ts": true,
}

func expandTranscribeTargets(args []string) ([]string, error) {
	var out []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", arg, err)
		}
		if !info.IsDir() {
			out = append(out, arg)
			continue
		}
		entries, err := os.ReadDir(arg)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", arg, err)
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if transcribableExts[strings.ToLower(filepath.Ext(e.Name()))] {
				out = append(out, filepath.Join(arg, e.Name()))
			}
		}
	}
	return uniquePaths(out), nil
}
