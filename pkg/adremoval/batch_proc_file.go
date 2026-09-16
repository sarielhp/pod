package adremoval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/audio"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/detect"
	"pod/pkg/format"
	"pod/pkg/gemini"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

// markTranscriptionStarted records that this episode is being transcribed,
// along with what the source audio was before anything was cut from it.
func markTranscriptionStarted(mainMP3File, sourceAudioFile string, totalDuration float64, verbose bool) {
	err := pipeline.UpdateEpisodeStatus(mainMP3File, func(st *types.EpisodeStatusFile) {
		st.Status = types.StateTranscribingLocally
		st.Original.DurationSec = totalDuration
		if fi, err := os.Stat(sourceAudioFile); err == nil {
			st.Original.SizeBytes = fi.Size()
		}
	})
	if err != nil && verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update episode status: %v\n", err)
	}
}

// applyPreviewLimit truncates the audio when only the first few minutes were
// asked for, returning the duration actually being transcribed.
func applyPreviewLimit(sourceAudioFile *string, totalDuration float64, opts types.ProcOptions) float64 {
	if opts.TranscribeMin == "" {
		return totalDuration
	}
	limited, err := pipeline.HandleTranscribeMin(sourceAudioFile, totalDuration, opts.TranscribeMin)
	if err != nil && !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Warning: failed to truncate preview audio: %v\n", err)
	}
	return limited
}

// discardTruncatedPreview removes the temporary audio a preview limit made.
func discardTruncatedPreview(sourceAudioFile string) {
	if strings.HasSuffix(sourceAudioFile, ".truncated.wav") {
		os.Remove(sourceAudioFile)
	}
}

func processSingleAudioFile(idx, totalFiles, processedCount int, inputFile string, opts types.ProcOptions, config types.Config, action string, batchStartTime time.Time, selectedProfile types.LLMProfile) (hasError bool, processed bool, stop bool) {
	fileStartTime := time.Now()

	if strings.HasSuffix(inputFile, ".json") {
		pipeline.ProcessJSONFile(inputFile, opts)
		return false, false, false
	}

	mainMP3File, precutFile, sourceAudioFile := pipeline.ResolveAudioFiles(inputFile, opts.Verbose)
	baseName := util.StripExt(mainMP3File)
	jsonFile := opts.TranscriptPath
	if jsonFile == "" {
		jsonFile = baseName + ".transcript.json"
	}
	outputFile := pipeline.ResolveOutputFile(mainMP3File, opts.Output, totalFiles)

	fileLock, ok, shouldStop := checkSkipOrLockAudioFile(mainMP3File, inputFile, idx, totalFiles, processedCount, opts)
	if !ok || shouldStop {
		return false, false, shouldStop
	}
	defer fileLock.Release()
	processed = true

	totalDuration := audio.GetAudioDuration(sourceAudioFile)
	markTranscriptionStarted(mainMP3File, sourceAudioFile, totalDuration, opts.Verbose)
	totalDuration = applyPreviewLimit(&sourceAudioFile, totalDuration, opts)
	if opts.Recut {
		err := pipeline.HandleRecut(mainMP3File, sourceAudioFile, precutFile, outputFile, baseName, totalDuration, selectedProfile, config, opts, fileStartTime)
		return err != nil, processed, false
	}

	if needsTranscription := !util.FileExists(jsonFile) || opts.ForceTranscribe; needsTranscription {
		var success, handled bool
		switch {
		case canRunSpeculativeRace(config, opts):
			success, handled = handleSpeculativeStep(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile, totalDuration, config, opts, selectedProfile, fileStartTime)
		case isGeminiEngine(config, opts):
			success, handled = handleGeminiStepWithFallback(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile, totalDuration, config, opts, selectedProfile, fileStartTime)
		}
		if handled {
			discardTruncatedPreview(sourceAudioFile)
			return !success, processed, false
		}
	}

	transData, t0Step1, ok, hasErr := runLocalTranscriptionStep(sourceAudioFile, jsonFile, mainMP3File, totalDuration, config, opts, selectedProfile, fileStartTime)
	if hasErr || !ok {
		return hasErr, processed, false
	}

	cutSuccess := runLocalAdDetectionAndCutStep(transData, sourceAudioFile, mainMP3File, precutFile, outputFile, totalDuration, config, opts, selectedProfile, fileStartTime, t0Step1)
	if strings.HasSuffix(sourceAudioFile, ".truncated.wav") {
		os.Remove(sourceAudioFile)
	}
	return !cutSuccess, processed, false
}

func canRunSpeculativeRace(cfg types.Config, opts types.ProcOptions) bool {
	if !cfg.IsSpeculativeTranscriptionEnabled() || opts.WhisperEngine != "" {
		return false
	}
	racers := pipeline.ResolveSpeculativeRacers(cfg, opts, cfg.WhisperLanguage)
	if len(racers) < 2 {
		if isOpen, until, reason := gemini.IsCircuitBreakerOpen(); isOpen && !opts.Quiet {
			for _, s := range cfg.GetCompetingServices() {
				if strings.EqualFold(s, "gemini") || strings.EqualFold(s, "google") {
					fmt.Printf("   %s\n", util.BoldYellow(fmt.Sprintf("Gemini in cooldown until %s (%s); skipping speculative race and using direct transcription.", until.Format("15:04:05"), reason)))
					break
				}
			}
		}
		return false
	}
	return true
}

func handleSpeculativeStep(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile string, totalDuration float64, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, fileStartTime time.Time) (bool, bool) {
	t0Step1 := time.Now()
	speedFactor := cfg.WhisperSpeedFactor
	if speedFactor <= 0 {
		speedFactor = 7.0
	}
	id3Tags := audio.ExtractID3Tags(sourceAudioFile)
	isHebrew := transcribe.IsHebrewAudio(sourceAudioFile, id3Tags, cfg.WhisperLanguage)
	whisperPrompt := cfg.WhisperPrompt
	if whisperPrompt == "" {
		whisperPrompt = pipeline.ExtractMetadataPrompt(sourceAudioFile, id3Tags, selectedProfile, opts)
	}
	dockerContainer := cfg.WhisperDockerContainer
	if dockerContainer == "" {
		dockerContainer = transcribe.DetectWhisperDockerContainer(cfg.WhisperURL)
	}

	td, ads, geminiWon, err := pipeline.RunSpeculativeParallelRace(context.Background(), sourceAudioFile, cfg, opts, totalDuration, speedFactor, whisperPrompt, cfg.WhisperLanguage, dockerContainer, isHebrew)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nSpeculative transcription error: %v\n\n", err)
		return false, false
	}

	if opts.SaveTranscript {
		pipeline.SaveJSONTranscript(mainMP3File, td, jsonFile, opts.Quiet, id3Tags)
	}

	if handleExportOrPreviewReturns(td, totalDuration, fileStartTime, sourceAudioFile, jsonFile, opts) {
		return true, true
	}

	if geminiWon {
		return finalizeGeminiRaceWinner(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile, totalDuration, td, ads, selectedProfile, cfg, opts, fileStartTime, t0Step1)
	}

	detectAndSanitizeTranscriptLanguage(td, cfg.WhisperLanguage, true, opts.Quiet)
	if !validateTranscriptSanity(td, totalDuration, opts.Quiet) {
		return false, true
	}

	cutSuccess := runLocalAdDetectionAndCutStep(td, sourceAudioFile, mainMP3File, precutFile, outputFile, totalDuration, cfg, opts, selectedProfile, fileStartTime, t0Step1)
	return cutSuccess, true
}

func finalizeGeminiRaceWinner(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile string, totalDuration float64, td *types.TranscriptionData, ads []types.AdSegment, selectedProfile types.LLMProfile, cfg types.Config, opts types.ProcOptions, fileStartTime, t0Step1 time.Time) (bool, bool) {
	if len(ads) > 0 {
		ads = format.MergeIntervals(ads)
	}
	t0Step2 := time.Now()
	if err := updateTranscriptAdDetectionStatus(jsonFile, true, "completed", "gemini-flash", "", len(ads)); err != nil && opts.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update transcript ad status: %v\n", err)
	}
	if err := updateStatusAdDetection(mainMP3File, true, "completed", "gemini-flash", ""); err != nil && opts.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update episode status: %v\n", err)
	}
	if len(ads) == 0 {
		handleNoAdsDetected(mainMP3File, sourceAudioFile, outputFile, totalDuration, selectedProfile, opts, fileStartTime, t0Step1, t0Step2)
		return true, true
	}
	cutsResult := format.SaveCutsJSON(mainMP3File, totalDuration, ads, &selectedProfile, opts.Quiet)
	t0Step3 := time.Now()
	cutSuccess := executeLocalAudioCutting(sourceAudioFile, mainMP3File, precutFile, outputFile, cutsResult.KeepSegments, ads, totalDuration, cfg, opts, selectedProfile, fileStartTime, t0Step1, t0Step2, t0Step3)
	return cutSuccess, true
}

func handleGeminiStepWithFallback(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile string, totalDuration float64, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, fileStartTime time.Time) (bool, bool) {
	if runGeminiPipelineStep(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile, totalDuration, cfg, opts, selectedProfile, fileStartTime) {
		return true, true
	}
	if !opts.Quiet {
		fmt.Println()
		fmt.Println("\n" + util.BoldYellow("Transcription: Gemini processing failed. Falling back to Whisper...") + "\n")
	}
	fallbackCfg := config.PrepareWhisperFallbackConfig(cfg)
	fallbackOpts := opts
	fallbackOpts.WhisperEngine = string(fallbackCfg.WhisperEngine)
	transData, t0Step1, ok, hasErr := runLocalTranscriptionStep(sourceAudioFile, jsonFile, mainMP3File, totalDuration, fallbackCfg, fallbackOpts, selectedProfile, fileStartTime)
	if hasErr || !ok {
		return false, true
	}
	cutSuccess := runLocalAdDetectionAndCutStep(transData, sourceAudioFile, mainMP3File, precutFile, outputFile, totalDuration, fallbackCfg, fallbackOpts, selectedProfile, fileStartTime, t0Step1)
	return cutSuccess, true
}

func runLocalTranscriptionStep(sourceAudioFile, jsonFile, mainMP3File string, totalDuration float64, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, fileStartTime time.Time) (*types.TranscriptionData, time.Time, bool, bool) {
	speedFactor := cfg.WhisperSpeedFactor
	if speedFactor <= 0 {
		speedFactor = 7.0
	}
	t0Step1 := time.Now()
	isNewlyTranscribed := false
	id3Tags := map[string]string{}

	transcriptionData, err := pipeline.LoadOrTranscribe(sourceAudioFile, jsonFile, cfg, opts, selectedProfile, totalDuration, speedFactor, cfg.WhisperLanguage, cfg.WhisperPrompt, id3Tags, &isNewlyTranscribed, &t0Step1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n\n", err)
		return nil, t0Step1, false, true
	}

	detectAndSanitizeTranscriptLanguage(transcriptionData, cfg.WhisperLanguage, isNewlyTranscribed, opts.Quiet)
	if !validateTranscriptSanity(transcriptionData, totalDuration, opts.Quiet) {
		return nil, t0Step1, false, true
	}

	if isNewlyTranscribed && opts.SaveTranscript {
		pipeline.SaveJSONTranscript(mainMP3File, transcriptionData, jsonFile, opts.Quiet, id3Tags)
	}

	if handleExportOrPreviewReturns(transcriptionData, totalDuration, fileStartTime, sourceAudioFile, jsonFile, opts) {
		return nil, t0Step1, false, false
	}
	return transcriptionData, t0Step1, true, false
}

func runLocalAdDetectionAndCutStep(transcriptionData *types.TranscriptionData, sourceAudioFile, mainMP3File, precutFile, outputFile string, totalDuration float64, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, fileStartTime, t0Step1 time.Time) bool {
	formattedTranscript := pipeline.FormatTranscript(transcriptionData, totalDuration)
	t0Step2 := time.Now()
	if !opts.Quiet {
		fmt.Println()
		fmt.Println(util.BoldYellow("Step 2/3: Detecting ad/sponsor segments..."))
	}
	jsonFile := opts.TranscriptPath
	if jsonFile == "" {
		jsonFile = util.StripExt(mainMP3File) + ".transcript.json"
	}
	detect.AnnounceAdDetection(selectedProfile, opts.Quiet)
	detector := detect.NewLLMAdDetector(selectedProfile, selectedProfile.APIKey, detect.DefaultLLMTimeout)
	adSegments, err := detector.DetectAds(context.Background(), formattedTranscript)
	if err != nil {
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "\nError during LLM ad detection: %v\n\n", err)
		}
		if err := updateTranscriptAdDetectionStatus(jsonFile, false, "failed", selectedProfile.Model, err.Error(), 0); err != nil && opts.Verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to update transcript ad status: %v\n", err)
		}
		if err := updateStatusAdDetection(mainMP3File, false, "failed", selectedProfile.Model, err.Error()); err != nil && opts.Verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to update episode status: %v\n", err)
		}
		if err := pipeline.UpdateEpisodeStatus(mainMP3File, func(st *types.EpisodeStatusFile) {
			st.Status = types.StateFailed
		}); err != nil && opts.Verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to update episode status: %v\n", err)
		}
		return false
	}
	if err := updateTranscriptAdDetectionStatus(jsonFile, true, "completed", selectedProfile.Model, "", len(adSegments)); err != nil && opts.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update transcript ad status: %v\n", err)
	}
	if err := updateStatusAdDetection(mainMP3File, true, "completed", selectedProfile.Model, ""); err != nil && opts.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update episode status: %v\n", err)
	}
	if len(adSegments) > 0 {
		adSegments = format.MergeIntervals(adSegments)
	}
	if len(adSegments) == 0 {
		handleNoAdsDetected(mainMP3File, sourceAudioFile, outputFile, totalDuration, selectedProfile, opts, fileStartTime, t0Step1, t0Step2)
		return true
	}

	cutsResult := format.SaveCutsJSON(mainMP3File, totalDuration, adSegments, &selectedProfile, opts.Quiet)
	t0Step3 := time.Now()
	return executeLocalAudioCutting(sourceAudioFile, mainMP3File, precutFile, outputFile, cutsResult.KeepSegments, adSegments, totalDuration, cfg, opts, selectedProfile, fileStartTime, t0Step1, t0Step2, t0Step3)
}

func checkSkipOrLockAudioFile(mainMP3File, inputFile string, idx, totalFiles, processedCount int, opts types.ProcOptions) (*util.FileLockWrapper, bool, bool) {
	shortName := util.DisplayName(filepath.Base(inputFile))
	if !opts.ForceTranscribe && !opts.ForceLLM && !opts.Recut && pipeline.IsEpisodeClean(mainMP3File) {
		if opts.Verbose && !opts.Quiet {
			fmt.Printf("skipping: %s\n", shortName)
		}
		return nil, false, false
	}
	if opts.Count > 0 && processedCount >= opts.Count {
		if !opts.Quiet {
			fmt.Printf("\nReached maximum episode processing limit (%d). Done.\n", opts.Count)
		}
		return nil, false, true
	}

	fileLock, err := util.AcquireFileLock(mainMP3File)
	if err != nil {
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "Cannot safely process %s: %v\n", shortName, err)
		}
		return nil, false, false
	}
	if fileLock == nil {
		if !opts.Quiet {
			fmt.Printf("⏭️  Skipping '%s' (currently being processed by another instance)\n", shortName)
		}
		return nil, false, false
	}

	if !opts.Quiet {
		printEpisodeHeader(inputFile, idx, totalFiles, processedCount, opts)
	}
	return fileLock, true, false
}

// printEpisodeHeader leads with the podcast and the episode, each on its own
// line, so the two names are readable before any processing detail.
func printEpisodeHeader(inputFile string, idx, totalFiles, processedCount int, opts types.ProcOptions) {
	fmt.Println()
	// Resolve first: a relative argument would otherwise name the podcast ".".
	resolved := inputFile
	if abs, err := filepath.Abs(inputFile); err == nil {
		resolved = abs
	}
	podcastDir := podcast.DetectPodcastDirForAudio(resolved)
	podcastName := filepath.Base(podcastDir)
	episodeName := podcast.EpisodeTitleFromPath(resolved)
	fmt.Printf("%s %s\n", util.BoldCyan("Podcast:"), util.Bold(util.DisplayName(podcastName)))
	fmt.Printf("%s %s\n", util.BoldCyan("Episode:"), util.Bold(util.DisplayName(episodeName)))
	switch {
	case opts.Count > 0:
		fmt.Printf("Processing (%d/%d limit): %s\n", processedCount, opts.Count, podcastDir)
	case totalFiles > 1:
		fmt.Printf("Processing (%d/%d): %s\n", idx+1, totalFiles, podcastDir)
	default:
		fmt.Printf("Processing: %s\n", podcastDir)
	}
}

func detectAndSanitizeTranscriptLanguage(transcriptionData *types.TranscriptionData, whisperLanguage string, isNewlyTranscribed, quiet bool) {
	detectedLang := transcriptionData.Language
	if detectedLang == "" && len(transcriptionData.Segments) > 0 {
		detectedLang = transcriptionData.Segments[0].Language
	}
	if !quiet && detectedLang != "" {
		langLabel := "(auto-detected)"
		if whisperLanguage != "" {
			langLabel = "(config override)"
		}
		fmt.Printf("   Detected language: %s %s\n", strings.ToUpper(detectedLang), langLabel)
	}

	if whisperLanguage == "" && isNewlyTranscribed {
		fullText := transcriptionData.Text
		if fullText == "" {
			for _, seg := range transcriptionData.Segments {
				fullText += seg.Text + " "
			}
		}
		scriptLang := detectScriptLanguage(fullText)
		if scriptLang != "" && scriptLang != detectedLang {
			transcriptionData.Language = scriptLang
			if !quiet {
				fmt.Printf("   Corrected language from %s to %s (detected from script)\n", strings.ToUpper(detectedLang), strings.ToUpper(scriptLang))
			}
		}
	}
}

func handleExportOrPreviewReturns(transcriptionData *types.TranscriptionData, totalDuration float64, fileStartTime time.Time, sourceAudioFile, jsonFile string, opts types.ProcOptions) bool {
	if opts.ExportSRT {
		if _, err := format.ConvertJSONToSRT(jsonFile, transcriptionData, opts.TranscriptPath, opts.Quiet); err != nil && !opts.Quiet {
			fmt.Fprintf(os.Stderr, "Error exporting SRT: %v\n", err)
		}
	}
	if opts.ExportTXT {
		if _, err := format.ConvertJSONToTXT(jsonFile, transcriptionData, totalDuration, opts.TranscriptPath, opts.Quiet); err != nil && !opts.Quiet {
			fmt.Fprintf(os.Stderr, "Error exporting TXT: %v\n", err)
		}
	}
	if opts.ExportSRT || opts.ExportTXT {
		if !opts.Quiet {
			fmt.Printf("Export completed in %s\n", format.FormatClock(time.Since(fileStartTime).Seconds()))
		}
		return true
	}
	if opts.TranscribeMin != "" {
		if !opts.Quiet {
			fmt.Printf("Preview transcription completed in %s\n   Transcript saved - original file was not modified.\n", format.FormatClock(time.Since(fileStartTime).Seconds()))
		}
		if strings.HasSuffix(sourceAudioFile, ".truncated.wav") {
			os.Remove(sourceAudioFile)
		}
		return true
	}
	return false
}

func installNoAdsOutput(source, output string) error {
	if source == output {
		return nil
	}
	workDir := util.WorkDirFor(output)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	tmp := filepath.Join(workDir, filepath.Base(output)+".tmp"+filepath.Ext(output))
	util.VerifyTempFile(tmp)

	if err := util.CopyFileErr(source, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := util.SafeMove(tmp, output); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.RemoveAll(workDir)
	return nil
}

func handleNoAdsDetected(mainMP3File, sourceAudioFile, outputFile string, totalDuration float64, selectedProfile types.LLMProfile, opts types.ProcOptions, fileStartTime, t0Step1, t0Step2 time.Time) {
	if sourceAudioFile != outputFile {
		if err := installNoAdsOutput(sourceAudioFile, outputFile); err != nil {
			if !opts.Quiet {
				fmt.Fprintf(os.Stderr, "Error installing output file: %v\n", err)
			}
			return
		}
	}
	format.SaveCutsJSON(mainMP3File, totalDuration, nil, &selectedProfile, opts.Quiet)
	if err := pipeline.UpdateEpisodeStatus(mainMP3File, func(st *types.EpisodeStatusFile) {
		st.Status = types.StateDone
		st.Cleaned = types.EpisodeAudioMeta{Filename: filepath.Base(outputFile), DurationSec: totalDuration}
		st.Ads = nil
	}); err != nil && !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Warning: failed to update episode status: %v\n", err)
	}
	if !opts.Quiet {
		fmt.Println("No ad segments detected by LLM!")
		printTimingSummary(opts.Verbose, totalDuration, totalDuration, 0, 0, 0, time.Since(t0Step1), time.Since(t0Step2), 0, time.Since(fileStartTime))
	}
	fmt.Printf("Result saved to: '%s'\n", outputFile)
}

func executeLocalAudioCutting(sourceAudioFile, mainMP3File, precutFile, outputFile string, keepSegments [][2]float64, adSegments []types.AdSegment, totalDuration float64, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, fileStartTime, t0Step1, t0Step2, t0Step3 time.Time) bool {
	if err := pipeline.UpdateEpisodeStatus(mainMP3File, func(st *types.EpisodeStatusFile) {
		st.Status = types.StateCuttingLocally
	}); err != nil && opts.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update status to cutting: %v\n", err)
	}
	if !opts.Quiet {
		fmt.Println()
		fmt.Printf("Step 3/3: Cutting ads with ffmpeg (%d non-ad clips)...\n", len(keepSegments))
	}

	workDir := util.WorkDirFor(outputFile)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating work directory '%s': %v\n", workDir, err)
		return false
	}
	tempOutputFile := filepath.Join(workDir, filepath.Base(outputFile)+".tmp"+filepath.Ext(outputFile))
	if err := util.VerifyTempFile(tempOutputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid temp output file '%s': %v\n", tempOutputFile, err)
		return false
	}

	if err := audio.DefaultProcessor.Cut(context.Background(), sourceAudioFile, keepSegments, tempOutputFile); err != nil {
		_ = os.Remove(tempOutputFile)
		_ = os.RemoveAll(workDir)
		fmt.Fprintf(os.Stderr, "Failed to output ad-free audio for '%s': %v\n", mainMP3File, err)
		return false
	}

	if !installCutAudioAndPreserveOriginal(sourceAudioFile, mainMP3File, precutFile, outputFile, tempOutputFile, workDir, opts.Quiet) {
		return false
	}
	_ = os.RemoveAll(workDir)

	newDuration := audio.GetAudioDuration(outputFile)
	actualCut := totalDuration - newDuration
	pctCut := 0.0
	if totalDuration > 0 {
		pctCut = actualCut / totalDuration * 100
	}

	if err := updateEpisodeStatusAfterCut(mainMP3File, precutFile, outputFile, adSegments, newDuration, actualCut); err != nil && !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Warning: failed to update episode status: %v\n", err)
	}

	if !opts.Quiet {
		printFullSummary(opts.Verbose, totalDuration, newDuration, actualCut, pctCut, len(adSegments), time.Since(t0Step1), time.Since(t0Step2), time.Since(t0Step3), time.Since(fileStartTime))
		fmt.Printf("\nSuccess! Ad-free episode saved to: '%s'\n", outputFile)
	}
	if err := backend.SyncEpisodeDuration(&cfg, outputFile, newDuration); err != nil && !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Warning: failed to sync duration: %v\n", err)
	}
	return true
}

func installCutAudioAndPreserveOriginal(sourceAudioFile, mainMP3File, precutFile, outputFile, tempOutputFile, workDir string, quiet bool) bool {
	preserved := false
	if sourceAudioFile == mainMP3File && util.FileExists(mainMP3File) {
		if err := checkPrecutSymlink(precutFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return false
		}
		if err := os.Link(mainMP3File, precutFile); err != nil {
			if cpErr := util.CopyFileErr(mainMP3File, precutFile); cpErr != nil {
				fmt.Fprintf(os.Stderr, "Error: could not preserve the original: %v\n", cpErr)
				return false
			}
		}
		preserved = true
		if !quiet {
			fmt.Printf("Original file preserved at: '%s'\n", precutFile)
		}
	}

	if mvErr := util.SafeMove(tempOutputFile, outputFile); mvErr != nil {
		if preserved {
			_ = os.Remove(precutFile)
		}
		fmt.Fprintf(os.Stderr, "Error: could not install the cut audio: %v\n", mvErr)
		return false
	}
	return true
}

func updateEpisodeStatusAfterCut(mainMP3File, precutFile, outputFile string, adSegments []types.AdSegment, newDuration, actualCut float64) error {
	return pipeline.UpdateEpisodeStatus(mainMP3File, func(st *types.EpisodeStatusFile) {
		st.Status = types.StateDone
		if util.FileExists(precutFile) {
			st.Original.Filename = filepath.Base(precutFile)
			if fi, err := os.Stat(precutFile); err == nil {
				st.Original.SizeBytes = fi.Size()
			}
		}
		st.Cleaned.Filename = filepath.Base(outputFile)
		st.Cleaned.DurationSec = newDuration
		st.Cleaned.AdDurationSec = actualCut
		if fi, err := os.Stat(outputFile); err == nil {
			st.Cleaned.SizeBytes = fi.Size()
		}
		st.Ads = make([]types.EpisodeAdCut, 0, len(adSegments))
		for _, ad := range adSegments {
			st.Ads = append(st.Ads, types.EpisodeAdCut(ad))
		}
	})
}
