package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pod/pkg/audio"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/detect"
	"pod/pkg/format"
	"pod/pkg/gemini"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

func ResolveAudioFiles(inputFile string, verbose bool) (mainMP3File, precutFile, sourceAudioFile string) {
	if strings.HasSuffix(inputFile, ".precut") {
		precutFile = inputFile
		mainMP3File = strings.TrimSuffix(inputFile, ".precut")
	} else {
		mainMP3File = inputFile
		precutFile = inputFile + ".precut"
	}

	if util.FileExists(precutFile) {
		sourceAudioFile = precutFile
		if verbose {
			fmt.Printf("Found existing pre-cut audio source: '%s'\n", precutFile)
		}
	} else if util.FileExists(mainMP3File) {
		sourceAudioFile = mainMP3File
	} else {
		sourceAudioFile = inputFile
	}
	return
}

func ResolveOutputFile(mainMP3File string, output string, totalFiles int) string {
	if totalFiles > 1 && output != "" {
		if info, err := os.Stat(output); err == nil && info.IsDir() {
			return filepath.Join(output, filepath.Base(mainMP3File))
		}
	}
	if output != "" {
		return output
	}
	return mainMP3File
}

func HandleTranscribeMin(sourceAudioFile *string, totalDuration float64, transcribeMin string) (float64, error) {
	val, err := strconv.ParseFloat(transcribeMin, 64)
	if err != nil || val <= 0 {
		return totalDuration, nil
	}
	durSec := val * 60.0
	if durSec >= totalDuration {
		return totalDuration, nil
	}
	workDir := util.WorkDirFor(*sourceAudioFile)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return totalDuration, err
	}
	truncPath := filepath.Join(workDir, filepath.Base(*sourceAudioFile)+".truncated.wav")
	if err := util.VerifyTempFile(truncPath); err != nil {
		return totalDuration, err
	}
	if !audio.TruncateAudio(*sourceAudioFile, truncPath, durSec) {
		return totalDuration, fmt.Errorf("failed to truncate audio to %v min", transcribeMin)
	}
	*sourceAudioFile = truncPath
	return durSec, nil
}

func HandleRecut(mainMP3File, sourceAudioFile, precutFile, outputFile, baseName string, totalDuration float64, selectedProfile types.LLMProfile, cfg types.Config, opts types.ProcOptions, fileStartTime time.Time) error {
	cutsFile := baseName + ".cuts.json"
	if !util.FileExists(cutsFile) {
		err := fmt.Errorf("cut metadata JSON file '%s' not found for recutting", cutsFile)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	keepSegments, _, ok := loadRecutKeepSegments(cutsFile, mainMP3File, totalDuration, selectedProfile, opts)
	if !ok || len(keepSegments) == 0 {
		return fmt.Errorf("no keep segments loaded from '%s'", cutsFile)
	}

	workDir := util.WorkDirFor(outputFile)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating work directory '%s': %v\n", workDir, err)
		return err
	}
	tempOutputFile := filepath.Join(workDir, filepath.Base(outputFile)+".tmp"+filepath.Ext(outputFile))
	if err := util.VerifyTempFile(tempOutputFile); err != nil {
		return err
	}

	return executeRecutAudio(sourceAudioFile, precutFile, outputFile, tempOutputFile, mainMP3File, workDir, keepSegments, totalDuration, cfg, opts, fileStartTime)
}

func loadRecutKeepSegments(cutsFile, mainMP3File string, totalDuration float64, selectedProfile types.LLMProfile, opts types.ProcOptions) ([][2]float64, types.CutsData, bool) {
	if !opts.Quiet {
		fmt.Printf("Recutting audio using existing cut metadata: '%s'\n", cutsFile)
	}

	data, err := os.ReadFile(cutsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading cuts file: %v\n", err)
		return nil, types.CutsData{}, false
	}
	var cutsData types.CutsData
	if err := json.Unmarshal(data, &cutsData); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing cuts file: %v\n", err)
		return nil, types.CutsData{}, false
	}

	var existingAds []types.AdSegment
	for _, c := range cutsData.CutIntervals {
		existingAds = append(existingAds, types.AdSegment{Start: c.StartSec, End: c.EndSec, Reason: c.Reason})
	}
	if len(existingAds) > 0 {
		existingAds = format.MergeIntervals(existingAds)
	}

	cutsResult := format.SaveCutsJSON(mainMP3File, totalDuration, existingAds, &selectedProfile, opts.Quiet)
	keepSegments := cutsResult.KeepSegments
	if len(keepSegments) == 0 {
		if !opts.Quiet {
			fmt.Println("No keep segments found in cut metadata.")
		}
		return nil, cutsData, false
	}

	if opts.Verbose && !opts.Quiet && len(cutsData.MergedCutIntervals) > 0 {
		fmt.Println("\nCUT INTERVALS TO REMOVE:")
		for _, m := range cutsData.MergedCutIntervals {
			fmt.Printf("  - [%s -> %s] (%.1fs)\n", format.FormatTime(m.Start), format.FormatTime(m.End), m.End-m.Start)
		}
		fmt.Println()
	}
	return keepSegments, cutsData, true
}

func executeRecutAudio(sourceAudioFile, precutFile, outputFile, tempOutputFile, mainMP3File, workDir string, keepSegments [][2]float64, totalDuration float64, cfg types.Config, opts types.ProcOptions, fileStartTime time.Time) error {
	t0Recut := time.Now()
	if !opts.Quiet {
		fmt.Printf("Cutting ads with ffmpeg (%d non-ad clips)...\n", len(keepSegments))
	}

	if !audio.KeepFractionIsPlausible(sourceAudioFile, keepSegments) {
		return fmt.Errorf("keep fraction not plausible for '%s'", sourceAudioFile)
	}

	if err := audio.DefaultProcessor.Cut(context.Background(), sourceAudioFile, keepSegments, tempOutputFile); err != nil {
		_ = os.Remove(tempOutputFile)
		_ = os.RemoveAll(workDir)
		return fmt.Errorf("failed to cut audio for '%s': %w", mainMP3File, err)
	}

	if !opts.Quiet && opts.Verbose {
		fmt.Printf("Audio Recutting finished in %s\n", format.FormatClock(time.Since(t0Recut).Seconds()))
	}

	if err := util.SafeMove(tempOutputFile, outputFile); err != nil {
		_ = os.Remove(tempOutputFile)
		_ = os.RemoveAll(workDir)
		return fmt.Errorf("failed to install cut audio '%s': %w", outputFile, err)
	}
	_ = os.RemoveAll(workDir)
	finishRecutStatusAndSummary(mainMP3File, precutFile, outputFile, totalDuration, cfg, opts, fileStartTime)
	return nil
}

func finishRecutStatusAndSummary(mainMP3File, precutFile, outputFile string, totalDuration float64, cfg types.Config, opts types.ProcOptions, fileStartTime time.Time) {
	newDuration := audio.GetAudioDuration(outputFile)
	actualCut := totalDuration - newDuration
	pctCut := 0.0
	if totalDuration > 0 {
		pctCut = actualCut / totalDuration * 100
	}

	if err := UpdateEpisodeStatus(mainMP3File, func(st *types.EpisodeStatusFile) {
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
	}); err != nil && !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Warning: failed to update status for '%s': %v\n", mainMP3File, err)
	}

	if err := backend.SyncEpisodeDuration(&cfg, outputFile, newDuration); err != nil && !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Warning: failed to sync duration for '%s': %v\n", outputFile, err)
	}

	if !opts.Quiet {
		fmt.Println()
		fmt.Println("DURATION & TIME SAVED SUMMARY (RECUT):")
		fmt.Printf("  - Original Episode Length: %s (%.1fs)\n", format.FormatMinutes(totalDuration), totalDuration)
		fmt.Printf("  - Total Ad Time Cut:       %s (%.1fs)\n", format.FormatTime(actualCut), actualCut)
		fmt.Printf("  - New Episode Length:      %s (%.1fs)\n", format.FormatMinutes(newDuration), newDuration)
		fmt.Printf("  - Reduction:               %.1f%% of episode trimmed\n", pctCut)
		fmt.Printf("  - Total Recut Time:        %s\n", format.FormatClock(time.Since(fileStartTime).Seconds()))
		fmt.Printf("Success! Recut ad-free episode saved to: '%s'\n", outputFile)
	}
}

func LoadOrTranscribe(sourceAudioFile, jsonFile string, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, totalDuration, speedFactor float64, whisperLanguage, whisperPrompt string, id3TagsOut map[string]string, isNewlyTranscribed *bool, t0Step1 *time.Time) (*types.TranscriptionData, error) {
	if util.FileExists(jsonFile) && !opts.ForceTranscribe {
		if !opts.Quiet {
			fmt.Printf("Found existing transcript JSON file: '%s'. Reusing transcript...\n", jsonFile)
		}
		data, err := os.ReadFile(jsonFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read transcript file: %w", err)
		}
		var td types.TranscriptionData
		if err := json.Unmarshal(data, &td); err != nil {
			return nil, fmt.Errorf("failed to parse transcript JSON: %w", err)
		}
		if !opts.Quiet && opts.Verbose {
			fmt.Printf("\nStep 1/3 (Transcript Loaded) finished in %s\n", format.FormatClock(time.Since(*t0Step1).Seconds()))
		}
		return &td, nil
	}

	transcribe.AnnounceStart(totalDuration, opts.Quiet)

	if whisperPrompt == "" {
		whisperPrompt = ExtractMetadataPrompt(sourceAudioFile, id3TagsOut, selectedProfile, opts)
	}

	dockerContainer := cfg.WhisperDockerContainer
	if dockerContainer == "" {
		dockerContainer = transcribe.DetectWhisperDockerContainer(cfg.WhisperURL)
		if opts.Verbose && dockerContainer != "" {
			fmt.Printf("   Auto-detected whisper Docker container: '%s'\n", dockerContainer)
		}
	}

	transcriptionData, err := runWhisperTranscription(sourceAudioFile, cfg, opts, totalDuration, speedFactor, whisperPrompt, whisperLanguage, dockerContainer)
	if err != nil {
		return nil, err
	}

	if !opts.Quiet && opts.Verbose {
		fmt.Printf("Step 1/3 (Transcription) finished in %s\n", format.FormatClock(time.Since(*t0Step1).Seconds()))
	}
	*isNewlyTranscribed = true
	return transcriptionData, nil
}

func ExtractMetadataPrompt(sourceAudioFile string, id3TagsOut map[string]string, selectedProfile types.LLMProfile, opts types.ProcOptions) string {
	id3Tags := audio.ExtractID3Tags(sourceAudioFile)
	for k, v := range id3Tags {
		id3TagsOut[k] = v
	}
	var tagTexts []string
	for _, key := range []string{"title", "artist", "album", "genre", "comment", "description", "synopsis", "purl", "encodedby", "copyright"} {
		if val, ok := id3TagsOut[key]; ok && val != "" {
			tagTexts = append(tagTexts, val)
		}
	}
	tagText := strings.Join(tagTexts, "\n")
	if tagText == "" {
		if !opts.Quiet {
			fmt.Println("   No ID3 metadata found in file for keyword extraction.")
		}
		return ""
	}

	if !opts.Quiet {
		if opts.Verbose {
			keys := make([]string, 0, len(id3Tags))
			for k := range id3Tags {
				keys = append(keys, k)
			}
			fmt.Printf("   Extracted ID3 metadata: %s\n", strings.Join(keys, ", "))
		}
		fmt.Println("   Extracting keywords from metadata to improve transcription accuracy...")
	}
	extracted := detect.ExtractKeywordsLLM(tagText, selectedProfile, selectedProfile.APIKey, opts.Quiet)
	if extracted != "" && opts.Verbose {
		fmt.Printf("   Using keywords: %s\n", extracted)
	}
	return extracted
}

func resolveWhisperRoutingProfile(cfg *types.Config, sourceAudioFile string, opts types.ProcOptions, whisperLang *string) types.WhisperProfile {
	wp := config.GetActiveWhisperProfile(cfg)
	isHebrew := transcribe.IsHebrewAudio(sourceAudioFile, nil, *whisperLang)
	if isHebrew && *whisperLang == "" {
		*whisperLang = "he"
	}
	if opts.WhisperEngine != "" {
		wp.Engine = types.WhisperEngine(opts.WhisperEngine)
	} else if wp.Engine != types.WhisperEngineGemini {
		lang := transcribe.WhisperTargetLanguage(*cfg, isHebrew, *whisperLang)
		routed := transcribe.ResolveWhisperProfileForLanguage(*cfg, lang)
		if routed.ID != wp.ID && !opts.Quiet {
			fmt.Printf("   Language %s: routing to %s (%s)\n", strings.ToUpper(lang), routed.Name, config.WhisperEngineBadge(routed.Engine))
		}
		wp = routed
		if wp.URL != "" {
			cfg.WhisperURL = wp.URL
		}
	}
	if opts.WhisperModel != "" {
		wp.Model = opts.WhisperModel
		// Gemini takes its model from the config rather than the profile, so
		// without this --whisper-model is silently ignored whenever the engine
		// is Gemini. It matters because the free tier's request quota is per
		// model: when one model is exhausted, naming another is the difference
		// between a transcript and a fallback to whisper.
		if wp.Engine == types.WhisperEngineGemini {
			cfg.GeminiModel = opts.WhisperModel
		}
	}
	return wp
}

// handleGeminiWhisperFallback runs the Gemini transcription and, when it
// fails, the local whisper fallback. It returns the transcript together with
// the profile and config that produced it, so the caller can record the
// backend that did the work rather than the one that was asked for. A nil
// transcript means neither path produced one and the caller should go on to
// the whisper server, using the returned fallback profile.
func handleGeminiWhisperFallback(ctx context.Context, sourceAudioFile string, cfg types.Config, opts types.ProcOptions, whisperPrompt, whisperLang string, geminiWp types.WhisperProfile) (*types.TranscriptionData, types.WhisperProfile, types.Config) {
	td, _, err := gemini.ProcessWithGeminiConfig(ctx, sourceAudioFile, cfg, cfg.GetGeminiChunkSec())
	if err == nil {
		// Prefer the model Gemini reports as having answered: a chain may have
		// moved past the configured one, and the configured one may be an
		// alias that names no particular model.
		geminiWp.Model = cfg.GetGeminiModel()
		if td != nil && td.Model != "" {
			geminiWp.Model = td.Model
		}
		return td, geminiWp, cfg
	}
	if !opts.Quiet {
		fmt.Printf("\n%s\n   %s\n   ➔ %s\n\n",
			util.BoldYellow("Transcription: Gemini failed:"),
			util.BoldYellow(strings.ReplaceAll(err.Error(), "\n", "\n   ")),
			util.Bold("Falling back to Whisper..."),
		)
	}
	fallbackCfg := config.PrepareWhisperFallbackConfig(cfg)
	fallbackWp := config.GetActiveWhisperProfile(&fallbackCfg)
	if fallbackWp.Engine == types.WhisperEngineLocal {
		res, runErr := transcribe.RunWhisperCLITranscription(sourceAudioFile, fallbackWp, opts.Quiet, opts.Verbose, whisperPrompt, whisperLang)
		if runErr != nil {
			res = nil
		}
		return res, fallbackWp, fallbackCfg
	}
	return nil, fallbackWp, fallbackCfg
}

func transcribeWhisperServerWithChunkFallback(sourceAudioFile string, cfg types.Config, opts types.ProcOptions, wp types.WhisperProfile, totalDuration, speedFactor float64, dockerContainer, whisperPrompt, whisperLang string) (*types.TranscriptionData, error) {
	transcribe.AnnounceWhisperServer(cfg.WhisperURL, wp.Engine, dockerContainer, opts.Quiet)
	chunkDuration := cfg.ChunkDurationSec
	useChunks := opts.UseChunks || (chunkDuration > 0 && totalDuration > float64(chunkDuration)*1.5)

	if useChunks {
		if !opts.Quiet {
			numChunks := int(totalDuration / float64(chunkDuration))
			if numChunks < 1 {
				numChunks = 1
			}
			fmt.Printf("   Audio is %s long - splitting into %d chunks of %s for reliability...\n",
				format.FormatMinutes(totalDuration), numChunks, format.FormatMinutes(float64(chunkDuration)))
		}
		return transcribe.TranscribeChunks(
			sourceAudioFile, cfg.WhisperURL, opts.Quiet, opts.Verbose,
			totalDuration, speedFactor, chunkDuration,
			dockerContainer, whisperPrompt, whisperLang,
		)
	}

	data, err := transcribe.TranscribeWhisper(
		sourceAudioFile, cfg.WhisperURL, opts.Quiet, opts.Verbose,
		totalDuration, speedFactor, dockerContainer,
		whisperPrompt, whisperLang, nil,
	)
	if err != nil && strings.Contains(err.Error(), "failed to") && totalDuration > 300 {
		if !opts.Quiet {
			fmt.Println("\n" + util.BoldYellow("Transcription: full-file Whisper request failed; retrying on the same server in chunks...") + "\n")
		}
		chunkDur := cfg.ChunkDurationSec
		if chunkDur <= 0 {
			chunkDur = 900
		}
		return transcribe.TranscribeChunks(
			sourceAudioFile, cfg.WhisperURL, opts.Quiet, opts.Verbose,
			totalDuration, speedFactor, chunkDur,
			dockerContainer, whisperPrompt, whisperLang,
		)
	}
	return data, err
}

func runWhisperTranscription(sourceAudioFile string, cfg types.Config, opts types.ProcOptions, totalDuration, speedFactor float64, whisperPrompt, whisperLang, dockerContainer string) (td *types.TranscriptionData, err error) {
	wp := resolveWhisperRoutingProfile(&cfg, sourceAudioFile, opts, &whisperLang)
	defer func() { transcribe.StampBackend(td, wp.Engine, wp.Model) }()

	if wp.Engine == types.WhisperEngineLocal {
		return transcribe.RunWhisperCLITranscription(sourceAudioFile, wp, opts.Quiet, opts.Verbose, whisperPrompt, whisperLang)
	}
	if wp.Engine == types.WhisperEngineGemini {
		// usedWp is the profile that actually produced res, which is not
		// necessarily the Gemini one: a failed Gemini call falls back to
		// whisper. Adopting it before returning is what keeps the deferred
		// StampBackend honest — otherwise a fallback transcript is labelled
		// "Gemini", and with a flaky API key that is the common case.
		res, usedWp, usedCfg := handleGeminiWhisperFallback(context.Background(), sourceAudioFile, cfg, opts, whisperPrompt, whisperLang, wp)
		wp, cfg = usedWp, usedCfg
		if res != nil {
			return res, nil
		}
	}

	return transcribeWhisperServerWithChunkFallback(sourceAudioFile, cfg, opts, wp, totalDuration, speedFactor, dockerContainer, whisperPrompt, whisperLang)
}

func FormatTranscript(data *types.TranscriptionData, totalDuration float64) string {
	segments := data.Segments
	if len(segments) == 0 && data.Text != "" {
		return fmt.Sprintf("[0.0s -> %.1fs] %s", totalDuration, data.Text)
	}

	var lines []string
	for _, seg := range segments {
		lines = append(lines, fmt.Sprintf("[%.1fs -> %.1fs] %s", seg.Start, seg.End, seg.Text))
	}
	return strings.Join(lines, "\n")
}

func ProcessJSONFile(inputFile string, opts types.ProcOptions) {
	if !util.FileExists(inputFile) {
		fmt.Fprintf(os.Stderr, "Error: Transcript JSON file '%s' not found.\n", inputFile)
		return
	}

	if !opts.Quiet {
		fmt.Printf("Processing transcript JSON file: '%s'\n", inputFile)
	}

	if !opts.ExportSRT && !opts.ExportTXT {
		opts.ExportSRT = true
		opts.ExportTXT = true
	}

	if opts.ExportSRT {
		format.ConvertJSONToSRT(inputFile, nil, opts.TranscriptPath, opts.Quiet)
	}
	if opts.ExportTXT {
		format.ConvertJSONToTXT(inputFile, nil, 0, opts.TranscriptPath, opts.Quiet)
	}
}

func SaveJSONTranscript(mainFile string, data *types.TranscriptionData, jsonFile string, quiet bool, id3Tags map[string]string) error {
	return format.SaveJSONTranscript(mainFile, data, jsonFile, quiet, id3Tags)
}
