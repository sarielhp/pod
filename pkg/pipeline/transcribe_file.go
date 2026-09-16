package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/audio"
	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

// TranscribeRequest is a one-off transcription of a single file.
//
// The file may be audio or video in any container ffmpeg can read. It is not
// required to be part of a podcast library, and nothing about the library is
// touched: no status file, no queue entry, no short ID. A lecture recording on
// a desktop is not an episode.
type TranscribeRequest struct {
	// Path is the audio or video file to transcribe.
	Path string

	// OutputDir is where the transcript files are written. Empty writes them
	// beside the input, which is the convention the rest of the tool follows.
	OutputDir string

	// Formats selects the outputs: "json", "srt", "txt". Empty means json.
	Formats []string

	// MaxMinutes transcribes only the first N minutes. Zero does the whole file.
	MaxMinutes float64

	// KeepAudio writes the extracted 16 kHz mono audio next to the transcript
	// instead of discarding it with the rest of the working files.
	KeepAudio bool
}

// TranscribeResult reports what a run produced.
type TranscribeResult struct {
	Data     *types.TranscriptionData
	Written  []string
	AudioOut string
	Duration float64
}

// TranscribeFile extracts the audio, transcribes it, and writes the transcript.
//
// The input is always converted to the 16 kHz mono WAV that every supported
// engine wants, in one ffmpeg pass. That is both the audio extraction for a
// video and the downsample for transcription: going via an intermediate
// compressed file would add a lossy generation for no benefit, because ffmpeg
// decodes the source either way.
func TranscribeFile(req TranscribeRequest, cfg types.Config, opts types.ProcOptions, rep progress.Reporter) (TranscribeResult, error) {
	r := progress.Or(rep)
	var res TranscribeResult

	info, err := os.Stat(req.Path)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", req.Path, err)
	}
	if info.IsDir() {
		return res, fmt.Errorf("%s is a directory", req.Path)
	}

	res.Duration = audio.GetAudioDuration(req.Path)
	if res.Duration <= 0 {
		return res, fmt.Errorf("%s has no readable audio track", filepath.Base(req.Path))
	}

	wavPath, cleanup, err := extractTranscriptionAudio(req, res.Duration, r)
	if err != nil {
		return res, err
	}
	defer cleanup()

	r.Infof("Transcribing %s (%s)...", filepath.Base(req.Path), format.FormatClock(res.Duration))
	td, err := runWhisperTranscription(wavPath, cfg, opts, res.Duration, 1.0, "", "", "")
	if err != nil {
		return res, fmt.Errorf("transcribe %s: %w", filepath.Base(req.Path), err)
	}
	res.Data = td

	if req.KeepAudio {
		res.AudioOut = transcriptOutputPath(req, ".audio.wav")
		if err := util.CopyFileErr(wavPath, res.AudioOut); err != nil {
			r.Warnf("could not keep the extracted audio: %v", err)
			res.AudioOut = ""
		}
	}

	written, err := writeTranscriptFormats(req, td, res.Duration, opts.Quiet)
	res.Written = written
	return res, err
}

// extractTranscriptionAudio converts the input to the 16 kHz mono WAV the
// engines expect, honouring MaxMinutes. The WAV goes in the .work directory
// beside the source, as every other temporary file in this tool does, and the
// whole directory is removed afterwards — a one-off transcription should leave
// nothing behind but the transcript.
func extractTranscriptionAudio(req TranscribeRequest, duration float64, r progress.Reporter) (string, func(), error) {
	workDir := util.WorkDirFor(req.Path)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", func() {}, fmt.Errorf("create work directory: %w", err)
	}
	wavPath := filepath.Join(workDir, filepath.Base(req.Path)+".wav")
	if err := util.VerifyTempFile(wavPath); err != nil {
		return "", func() {}, err
	}
	// The engines create their own scratch space under the WAV's directory, so
	// removing the tree is what actually cleans up, not removing the one file.
	cleanup := func() {
		_ = os.RemoveAll(workDir)
		_ = os.Remove(filepath.Dir(workDir))
	}

	limit := req.MaxMinutes * 60
	if limit > 0 && limit < duration {
		r.Infof("Extracting the first %s of audio...", format.FormatClock(limit))
		if !audio.TruncateAudio(req.Path, wavPath, limit) {
			cleanup()
			return "", func() {}, fmt.Errorf("could not extract audio from %s", filepath.Base(req.Path))
		}
		return wavPath, cleanup, nil
	}

	r.Infof("Extracting audio...")
	if !audio.ConvertToWAV(req.Path, wavPath) {
		cleanup()
		return "", func() {}, fmt.Errorf("could not extract audio from %s", filepath.Base(req.Path))
	}
	return wavPath, cleanup, nil
}

func writeTranscriptFormats(req TranscribeRequest, td *types.TranscriptionData, duration float64, quiet bool) ([]string, error) {
	formats := req.Formats
	if len(formats) == 0 {
		formats = []string{"json"}
	}
	var written []string
	for _, f := range formats {
		switch strings.ToLower(strings.TrimSpace(f)) {
		case "json":
			out := transcriptOutputPath(req, ".transcript.json")
			if err := format.SaveJSONTranscript(req.Path, td, out, quiet, nil); err != nil {
				return written, fmt.Errorf("write %s: %w", filepath.Base(out), err)
			}
			written = append(written, out)
		case "srt":
			out, err := format.ConvertJSONToSRT(req.Path, td, transcriptOutputPath(req, ".srt"), quiet)
			if err != nil {
				return written, fmt.Errorf("write srt: %w", err)
			}
			written = append(written, out)
		case "txt":
			out, err := format.ConvertJSONToTXT(req.Path, td, duration, transcriptOutputPath(req, ".txt"), quiet)
			if err != nil {
				return written, fmt.Errorf("write txt: %w", err)
			}
			written = append(written, out)
		default:
			return written, fmt.Errorf("unknown format %q (use json, srt or txt)", f)
		}
	}
	return written, nil
}

// transcriptOutputPath puts an output beside the input, or in OutputDir when
// one was given. The source extension is replaced rather than appended, so a
// video yields "lecture.transcript.json" and not "lecture.mkv.transcript.json".
func transcriptOutputPath(req TranscribeRequest, suffix string) string {
	base := util.StripExt(filepath.Base(req.Path)) + suffix
	dir := req.OutputDir
	if dir == "" {
		dir = filepath.Dir(req.Path)
	}
	return filepath.Join(dir, base)
}
