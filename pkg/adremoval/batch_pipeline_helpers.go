package adremoval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/gemini"
	"pod/pkg/pipeline"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

func validateTranscriptSanity(data *types.TranscriptionData, totalDuration float64, quiet bool) bool {
	if totalDuration <= 0 || data == nil {
		return true
	}
	segments := data.Segments
	fullText := data.Text
	if fullText == "" && len(segments) > 0 {
		var b strings.Builder
		for _, s := range segments {
			b.WriteString(s.Text)
			b.WriteByte(' ')
		}
		fullText = strings.TrimSpace(b.String())
	}
	wordCount := len(strings.Fields(fullText))
	minExpectedWords := int(totalDuration / 60.0 * 15.0)
	if minExpectedWords < 20 {
		minExpectedWords = 20
	}
	lastSegmentEnd := 0.0
	for _, seg := range segments {
		if seg.End > lastSegmentEnd {
			lastSegmentEnd = seg.End
		}
	}
	minRequiredCoverage := totalDuration * 0.85
	var failedReasons []string
	if wordCount < minExpectedWords {
		failedReasons = append(failedReasons, fmt.Sprintf("Word count too low (%d words found, expected at least %d words for %s audio)", wordCount, minExpectedWords, format.FormatClock(totalDuration)))
	}
	if totalDuration >= 30.0 && lastSegmentEnd < minRequiredCoverage {
		failedReasons = append(failedReasons, fmt.Sprintf("Transcript ended prematurely at %s (expected coverage up to at least %s)", format.FormatClock(lastSegmentEnd), format.FormatClock(minRequiredCoverage)))
	}
	if len(failedReasons) > 0 {
		if !quiet {
			fmt.Println("\n" + util.RepeatStr("WARNING ", 5))
			fmt.Println("TRANSCRIPT SANITY CHECK FAILED!")
			fmt.Println(util.RepeatStr("WARNING ", 5))
			for _, reason := range failedReasons {
				fmt.Printf("  - %s\n", reason)
			}
			fmt.Println("  - The Whisper transcription appears incomplete or corrupted.")
			fmt.Println("  - Aborting ad detection and audio cutting for safety.")
			fmt.Println(util.RepeatStr("WARNING ", 5) + "\n")
		}
		return false
	}
	return true
}

func detectScriptLanguage(text string) string {
	if text == "" {
		return ""
	}
	hebrew, arabic, cyrillic, greek, totalLetters := 0, 0, 0, 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			totalLetters++
			switch {
			case r >= 0x0590 && r <= 0x05FF:
				hebrew++
			case (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F):
				arabic++
			case r >= 0x0400 && r <= 0x04FF:
				cyrillic++
			case r >= 0x0370 && r <= 0x03FF:
				greek++
			}
		}
	}
	if totalLetters == 0 {
		return ""
	}
	threshold := float64(totalLetters) * 0.10
	switch {
	case float64(hebrew) >= threshold:
		return "he"
	case float64(arabic) >= threshold:
		return "ar"
	case float64(cyrillic) >= threshold:
		return "ru"
	case float64(greek) >= threshold:
		return "el"
	default:
		return ""
	}
}

func printTimingSummary(verbose bool, originalDuration, newDuration, actualCut float64, pctCut float64, numAds int, step1, step2, step3 time.Duration, total time.Duration) {
	fmt.Println("\nTIMING SUMMARY:")
	fmt.Printf("   - Original Length:     %s (%.1fs)\n", format.FormatMinutes(originalDuration), originalDuration)
	fmt.Printf("   - Time Cut:            %s (%.1fs)\n", format.FormatTime(actualCut), actualCut)
	fmt.Printf("   - New Episode Length:  %s (%.1fs)\n", format.FormatMinutes(newDuration), newDuration)
	if verbose {
		fmt.Printf("   - Running Times:\n")
		fmt.Printf("       - Step 1 (Transcription): %s\n", format.FormatClock(step1.Seconds()))
		fmt.Printf("       - Step 2 (Ad Detection):  %s\n", format.FormatClock(step2.Seconds()))
		fmt.Printf("       - Step 3 (Audio Cut):     %s\n", format.FormatClock(step3.Seconds()))
		fmt.Printf("       - Total File Processing:  %s\n", format.FormatClock(total.Seconds()))
	} else {
		fmt.Printf("   - Total Running Time:     %s\n", format.FormatClock(total.Seconds()))
	}
}

func printFullSummary(verbose bool, totalDuration, newDuration, actualCut float64, pctCut float64, numAds int, step1, step2, step3 time.Duration, total time.Duration) {
	fmt.Println()
	fmt.Println("DURATION & TIME SAVED SUMMARY:")
	fmt.Printf("  - Original Episode Length: %s (%.1fs)\n", format.FormatMinutes(totalDuration), totalDuration)
	fmt.Printf("  - Total Ad Time Cut:       %s (%.1fs across %d segment(s))\n", format.FormatTime(actualCut), actualCut, numAds)
	fmt.Printf("  - New Episode Length:      %s (%.1fs)\n", format.FormatMinutes(newDuration), newDuration)
	fmt.Printf("  - Reduction:               %.1f%% of episode trimmed\n", pctCut)
	if verbose {
		fmt.Printf("  - Running Times:\n")
		fmt.Printf("      - Step 1 (Transcription): %s\n", format.FormatClock(step1.Seconds()))
		fmt.Printf("      - Step 2 (Ad Detection):  %s\n", format.FormatClock(step2.Seconds()))
		fmt.Printf("      - Step 3 (Audio Cut):     %s\n", format.FormatClock(step3.Seconds()))
		fmt.Printf("      - Total File Processing:  %s\n", format.FormatClock(total.Seconds()))
	} else {
		fmt.Printf("  - Total Running Time:      %s\n", format.FormatClock(total.Seconds()))
	}
}

func checkPrecutSymlink(precutFile string) error {
	info, err := os.Lstat(precutFile)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("pre-cut backup file %q is a symlink, refusing to overwrite", precutFile)
	}
	return nil
}

func isGeminiEngine(cfg types.Config, opts types.ProcOptions) bool {
	if opts.WhisperEngine == string(types.WhisperEngineGemini) {
		return true
	}
	wp := config.GetActiveWhisperProfile(&cfg)
	return wp.Engine == types.WhisperEngineGemini
}

func updateStatusAdDetection(mainMP3File string, successful bool, status, model, errMsg string) error {
	return pipeline.UpdateEpisodeStatus(mainMP3File, func(st *types.EpisodeStatusFile) {
		st.AdDetectionSuccessful = &successful
		st.AdDetectionStatus = status
		st.AdDetectionModel = model
		st.AdDetectionError = errMsg
		if !successful {
			st.Status = types.StateNeedsAdR
		}
	})
}

func updateTranscriptAdDetectionStatus(jsonFile string, successful bool, status, model, errMsg string, adCount int) error {
	if !util.FileExists(jsonFile) {
		return nil
	}
	raw, err := os.ReadFile(jsonFile)
	if err != nil {
		return err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	data["ad_detection_successful"] = successful
	data["ad_detection_status"] = status
	if model != "" {
		data["ad_detection_model"] = model
	}
	if errMsg != "" {
		data["ad_detection_error"] = errMsg
	} else {
		delete(data, "ad_detection_error")
	}
	if successful {
		data["ad_segments_count"] = adCount
	}
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jsonFile, append(content, '\n'), 0644)
}

func runGeminiPipelineStep(sourceAudioFile, jsonFile, mainMP3File, precutFile, outputFile string, totalDuration float64, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, fileStartTime time.Time) bool {
	transcribe.AnnounceStart(totalDuration, opts.Quiet)
	ctx := context.Background()
	t0Step1 := time.Now()

	chunkDur := cfg.GeminiChunkSecCapped(cfg.ChunkDurationSec)
	td, ads, err := gemini.ProcessWithGeminiConfig(ctx, sourceAudioFile, cfg, chunkDur)
	transcribe.StampBackend(td, types.WhisperEngineGemini, cfg.GetGeminiModel())
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError processing with Gemini Flash: %v\n\n", err)
		return false
	}

	if opts.SaveTranscript {
		_ = pipeline.SaveJSONTranscript(mainMP3File, td, jsonFile, opts.Quiet, map[string]string{})
	}

	if handleExportOrPreviewReturns(td, totalDuration, fileStartTime, sourceAudioFile, jsonFile, opts) {
		return true
	}

	if len(ads) > 0 {
		ads = format.MergeIntervals(ads)
	}

	t0Step2 := time.Now()
	if err := updateTranscriptAdDetectionStatus(jsonFile, true, "completed", "gemini-flash", "", len(ads)); err != nil && opts.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update transcript ad status: %v\n", err)
	}
	if err := updateStatusAdDetection(mainMP3File, true, "completed", "gemini-flash", ""); err != nil && opts.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to update status ad detection: %v\n", err)
	}
	if len(ads) == 0 {
		handleNoAdsDetected(mainMP3File, sourceAudioFile, outputFile, totalDuration, selectedProfile, opts, fileStartTime, t0Step1, t0Step2)
		return true
	}

	cutsResult := format.SaveCutsJSON(mainMP3File, totalDuration, ads, &selectedProfile, opts.Quiet)
	t0Step3 := time.Now()
	return executeLocalAudioCutting(sourceAudioFile, mainMP3File, precutFile, outputFile, cutsResult.KeepSegments, ads, totalDuration, cfg, opts, selectedProfile, fileStartTime, t0Step1, t0Step2, t0Step3)
}
