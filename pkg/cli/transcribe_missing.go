package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"pod/pkg/audio"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/pipeline"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

// runTranscribeMissing transcribes every downloaded episode that has no
// transcript yet, newest first. Nothing else about an episode is touched: no
// status, no queue entry, no cut. Rerunning picks up whatever is left, so an
// interrupted run costs only the episode it was in.
func runTranscribeMissing(cfg Config, cli CLIOptions) error {
	dirs, err := missingTargets(cfg, cli)
	if err != nil {
		return err
	}
	missing := pipeline.FindMissingTranscripts(dirs)
	if cli.Count > 0 && len(missing) > cli.Count {
		missing = missing[:cli.Count]
	}
	out := outFor(cli)
	if len(missing) == 0 {
		fmt.Fprintln(out, "Every downloaded episode already has a transcript.")
		return nil
	}
	if cli.DryRun {
		printMissingTranscripts(cli, missing)
		return nil
	}
	opts := cli.ProcOptions
	opts.Normalize()
	wakeWhisper(cfg, opts)
	return transcribeEach(cfg, cli, opts, missing)
}

// missingTargets is every podcast when none is named, else the named podcasts
// and directories.
func missingTargets(cfg Config, cli CLIOptions) ([]string, error) {
	if len(cli.Args) > 0 {
		return analyzeTargets(cfg, cli)
	}
	lib := library(cfg, cli, nil)
	if lib.Config().PodcastsDir == "" {
		return nil, fmt.Errorf("podcasts_dir is not configured")
	}
	var dirs []string
	for _, entry := range lib.Podcasts() {
		dirs = append(dirs, entry.Dir)
	}
	return dirs, nil
}

// wakeWhisper starts the transcription host once, as ad removal does, when the
// engine is a remote one that may be asleep.
func wakeWhisper(cfg Config, opts ProcOptions) {
	wp := config.GetActiveWhisperProfile(&cfg)
	if opts.WhisperEngine != "" {
		wp.Engine = types.WhisperEngine(opts.WhisperEngine)
	}
	if wp.Engine != types.WhisperEngineLocal && wp.Engine != types.WhisperEngineGemini {
		transcribe.WakeServer(cfg.WhisperURL, cfg.WhisperWakeCommand, opts.Quiet)
	}
}

func printMissingTranscripts(cli CLIOptions, missing []pipeline.MissingTranscript) {
	out := outFor(cli)
	var bytes int64
	var seconds float64
	fromOriginal := 0
	for _, m := range missing {
		bytes += m.Bytes
		seconds += audio.GetAudioDuration(m.Source)
		if m.Source != m.Audio {
			fromOriginal++
		}
		fmt.Fprintf(out, "  %s\n", m.Audio)
	}
	fmt.Fprintf(out, "\n%d episode(s) without a transcript, %s of audio (%s on disk).\n", len(missing), format.FormatClock(seconds), humanBytes(bytes))
	if fromOriginal > 0 {
		fmt.Fprintf(out, "%d of them are already cut and will be transcribed from their uncut original.\n", fromOriginal)
	}
	fmt.Fprintln(out, "[dry-run] Nothing was transcribed.")
}

func transcribeEach(cfg Config, cli CLIOptions, opts ProcOptions, missing []pipeline.MissingTranscript) error {
	w := progressFor(cli)
	start := time.Now()
	var doneBytes, totalBytes int64
	for _, m := range missing {
		totalBytes += m.Bytes
	}
	var failures, skipped []string
	for i, m := range missing {
		fmt.Fprintf(w, "\n[%d/%d] %s%s\n", i+1, len(missing), filepath.Base(filepath.Dir(m.Audio))+" / ", filepath.Base(m.Audio))
		switch err := transcribeMissingOne(m, cfg, cli, opts); {
		case err == errEpisodeBusy:
			skipped = append(skipped, filepath.Base(m.Audio))
		case err != nil:
			util.FprintError(errFor(cli), "%v\n", err)
			failures = append(failures, filepath.Base(m.Audio))
		}
		doneBytes += m.Bytes
		fmt.Fprintln(w, progressLine(start, doneBytes, totalBytes))
	}
	return summarizeTranscribeRun(cli, len(missing), skipped, failures)
}

// errEpisodeBusy marks an episode left for later because another process has it.
var errEpisodeBusy = fmt.Errorf("episode is busy")

func transcribeMissingOne(m pipeline.MissingTranscript, cfg Config, cli CLIOptions, opts ProcOptions) error {
	if episode.IsEpisodeInRemoteFlight(m.Audio) {
		return errEpisodeBusy
	}
	lock, err := util.AcquireFileLock(m.Audio)
	if err != nil {
		return err
	}
	if lock == nil {
		return errEpisodeBusy
	}
	defer lock.Release()
	_, err = pipeline.TranscribeFile(pipeline.TranscribeRequest{
		Path:       m.Source,
		OutputBase: util.StripExt(m.Audio),
		Formats:    []string{"json"},
	}, cfg, opts, reporter(cli))
	return err
}

// progressLine reports elapsed time and, once enough has been done to judge the
// pace, roughly how long is left. Audio size stands in for audio length, since
// transcription time follows the length of the recording.
func progressLine(start time.Time, done, total int64) string {
	elapsed := time.Since(start)
	line := fmt.Sprintf("  elapsed %s", elapsed.Round(time.Second))
	if done > 0 && done < total {
		left := time.Duration(float64(elapsed) * float64(total-done) / float64(done))
		line += fmt.Sprintf(", about %s left", left.Round(time.Minute))
	}
	return line
}

func summarizeTranscribeRun(cli CLIOptions, total int, skipped, failures []string) error {
	w := progressFor(cli)
	fmt.Fprintf(w, "\nTranscribed %d of %d episode(s).\n", total-len(skipped)-len(failures), total)
	if len(skipped) > 0 {
		fmt.Fprintf(w, "%d left for later because another pod process had them; run again to pick them up.\n", len(skipped))
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d episode(s) could not be transcribed: %v", len(failures), total, failures)
	}
	return nil
}
