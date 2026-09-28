package adremoval

import (
	"errors"
	"fmt"
	"os"
	"time"

	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/transcribe"
	"pod/pkg/types"
)

var (
	errSkipped      = errors.New("episode skipped")
	errLimitReached = errors.New("processing limit reached")

	ErrSkipped      = errSkipped
	ErrLimitReached = errLimitReached
)

// Report describes the outcome of an ad-removal processing run.
type Report struct {
	Processed bool
	Total     int
	Failures  int
}

// ProcessFiles removes ads from an already-resolved set of audio file targets.
// Interpreting a command line (e.g. resolving directories or filtering by
// podcast policy) belongs to the caller.
func ProcessFiles(targets []string, opts types.ProcOptions, cfg types.Config, rep ...progress.Reporter) (Report, error) {
	opts.Normalize()
	var r progress.Reporter
	if len(rep) > 0 {
		r = rep[0]
	}
	r = progress.Or(r)

	if len(targets) == 0 {
		if !opts.Quiet {
			fmt.Println("No files or directories with audio found to process.")
		}
		return Report{}, nil
	}

	if opts.DryRun {
		handleProcDryRun(targets, opts, cfg, r)
		return Report{Total: len(targets)}, nil
	}

	return executeLocalBatchProcessing(targets, opts, cfg, r)
}

func executeLocalBatchProcessing(expandedArgs []string, opts types.ProcOptions, cfg types.Config, r progress.Reporter) (Report, error) {
	wp := config.GetActiveWhisperProfile(&cfg)
	if opts.WhisperEngine != "" {
		wp.Engine = types.WhisperEngine(opts.WhisperEngine)
	}
	if wp.Engine != types.WhisperEngineLocal && wp.Engine != types.WhisperEngineGemini {
		transcribe.WakeServer(cfg.WhisperURL, cfg.WhisperWakeCommand, opts.Quiet)
	}

	selectedProfile, _ := config.SelectLLMProfile(&cfg, opts.UseLLM)
	batchStartTime := time.Now()

	totalFiles := len(expandedArgs)
	processedCount := 0
	failures := 0

	for idx, inputFile := range expandedArgs {
		report, err := processSingleAudioFile(idx, len(expandedArgs), processedCount, inputFile, opts, cfg, batchStartTime, selectedProfile, r)
		if errors.Is(err, errLimitReached) {
			break
		}
		if errors.Is(err, errSkipped) {
			continue
		}
		if err != nil {
			failures++
			continue
		}
		if report.Processed {
			processedCount++
		}
	}

	if (processedCount > 1 || totalFiles > 1) && !opts.Quiet {
		batchDuration := time.Since(batchStartTime)
		fmt.Printf("\nBatch Completed! Processed %d file(s) in %s.\n", processedCount, format.FormatClock(batchDuration.Seconds()))
	}

	os.Stdout.Sync()
	os.Stderr.Sync()
	rep := Report{
		Total:     totalFiles,
		Processed: processedCount > 0,
		Failures:  failures,
	}
	if failures > 0 {
		return rep, fmt.Errorf("%d episode(s) failed or could not be processed", failures)
	}
	return rep, nil
}
