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
	"pod/pkg/config"
	"pod/pkg/detect"
	"pod/pkg/format"
	"pod/pkg/gemini"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

func ResolveAudioFiles(inputFile string, verbose bool) (mainMP3File, precutFile, sourceAudioFile string) {
	mainMP3File, precutFile = inputFile, inputFile+".precut"
	if strings.HasSuffix(inputFile, ".precut") {
		mainMP3File, precutFile = strings.TrimSuffix(inputFile, ".precut"), inputFile
	}

	switch {
	case util.FileExists(precutFile):
		sourceAudioFile = precutFile
	case util.FileExists(mainMP3File):
		sourceAudioFile = mainMP3File
	default:
		sourceAudioFile = inputFile
	}
	return mainMP3File, precutFile, sourceAudioFile
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
	_, err := CutFile(CutRequest{
		Path:     sourceAudioFile,
		CutsPath: cutsFile,
		Output:   outputFile,
		DryRun:   opts.DryRun,
	}, cfg, opts, nil)
	return err
}

func LoadOrTranscribe(sourceAudioFile, jsonFile string, cfg types.Config, opts types.ProcOptions, selectedProfile types.LLMProfile, totalDuration, speedFactor float64, whisperLanguage, whisperPrompt string, id3TagsOut map[string]string, isNewlyTranscribed *bool, t0Step1 *time.Time) (*types.TranscriptionData, error) {
	if util.FileExists(jsonFile) && !opts.ForceTranscribe {
		data, err := os.ReadFile(jsonFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read transcript file: %w", err)
		}
		var td types.TranscriptionData
		if err := json.Unmarshal(data, &td); err != nil {
			return nil, fmt.Errorf("failed to parse transcript JSON: %w", err)
		}
		return &td, nil
	}

	// The metadata prompt is built first, and only then is transcription
	// announced. Announcing before it meant the banner claimed transcription
	// had begun while pod was still waiting on an LLM call — no whisper
	// process, no GPU activity, and an idle socket, for however long that
	// call took.
	if whisperPrompt == "" {
		whisperPrompt = ExtractMetadataPrompt(sourceAudioFile, id3TagsOut, selectedProfile, opts)
	}

	transcribe.AnnounceStart(totalDuration, opts.Quiet)

	dockerContainer := cfg.WhisperDockerContainer
	if dockerContainer == "" {
		dockerContainer = transcribe.DetectWhisperDockerContainer(cfg.WhisperURL)
	}

	transcriptionData, err := runWhisperTranscription(sourceAudioFile, cfg, opts, totalDuration, speedFactor, whisperPrompt, whisperLanguage, dockerContainer)
	if err != nil {
		return nil, err
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
		return ""
	}

	return detect.ExtractKeywordsLLM(tagText, selectedProfile, selectedProfile.APIKey, opts.Quiet)
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
		wp = transcribe.ResolveWhisperProfileForLanguage(*cfg, lang)
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
		return
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
