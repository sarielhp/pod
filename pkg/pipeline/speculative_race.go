package pipeline

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pod/pkg/config"
	"pod/pkg/gemini"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

type SpeculativeRacer struct {
	Name     string
	IsGemini bool
	Profile  types.WhisperProfile
}

type SpeculativeCandidateResult struct {
	ServiceName string
	TD          *types.TranscriptionData
	Ads         []types.AdSegment
	IsGemini    bool
	Err         error
}

func runLocalCandidateTranscription(ctx context.Context, audioPath string, wp types.WhisperProfile, cfg types.Config, opts types.ProcOptions, totalDuration, speedFactor float64, whisperPrompt, whisperLang, dockerContainer string) (td *types.TranscriptionData, err error) {
	defer func() { transcribe.StampBackend(td, wp.Engine, wp.Model) }()
	if wp.Engine == types.WhisperEngineLocal {
		return transcribe.RunWhisperCLITranscriptionContext(ctx, audioPath, wp, opts.Quiet, opts.Verbose, whisperPrompt, whisperLang)
	}
	transcribe.AnnounceWhisperServer(wp.URL, wp.Engine, dockerContainer, opts.Quiet)
	chunkDuration := cfg.ChunkDurationSec
	useChunks := opts.UseChunks || (chunkDuration > 0 && totalDuration > float64(chunkDuration)*1.5)
	if useChunks {
		return transcribe.TranscribeChunksContext(
			ctx,
			audioPath, wp.URL, opts.Quiet, opts.Verbose,
			totalDuration, speedFactor, chunkDuration,
			dockerContainer, whisperPrompt, whisperLang,
		)
	}
	return transcribe.TranscribeWhisperContext(
		ctx, audioPath, wp.URL, opts.Quiet, opts.Verbose,
		totalDuration, speedFactor, dockerContainer,
		whisperPrompt, whisperLang, nil,
	)
}

func ResolveSpeculativeRacers(cfg types.Config, opts types.ProcOptions, lang string) []SpeculativeRacer {
	if !cfg.IsSpeculativeTranscriptionEnabled() {
		return nil
	}
	services := cfg.GetCompetingServices()
	var racers []SpeculativeRacer
	seen := make(map[string]bool)

	for _, raw := range services {
		name := strings.ToLower(strings.TrimSpace(raw))
		switch name {
		case "gemini", "google":
			if !seen["gemini"] && canIncludeGeminiRacer(cfg, opts) {
				seen["gemini"] = true
				racers = append(racers, SpeculativeRacer{
					Name:     "Gemini",
					IsGemini: true,
				})
			}
		case "whisper", "default":
			wp := transcribe.ResolveWhisperProfileForLanguage(cfg, lang)
			key := fmt.Sprintf("whisper-%d-%s", wp.ID, wp.Name)
			if !seen[key] && transcribe.WhisperProfileUsable(wp) {
				seen[key] = true
				racers = append(racers, SpeculativeRacer{
					Name:    wp.Name,
					Profile: wp,
				})
			}
		default:
			if wp, ok := matchWhisperProfileForRacer(cfg, name, lang); ok {
				key := fmt.Sprintf("whisper-%d-%s", wp.ID, wp.Name)
				if !seen[key] {
					seen[key] = true
					racers = append(racers, SpeculativeRacer{
						Name:    wp.Name,
						Profile: wp,
					})
				}
			}
		}
	}
	return racers
}

func canIncludeGeminiRacer(cfg types.Config, opts types.ProcOptions) bool {
	if !cfg.IsGeminiAPIKeyEnabled() || config.ResolveGeminiAPIKey(&cfg) == "" {
		return false
	}
	if opts.WhisperEngine != "" && opts.WhisperEngine != string(types.WhisperEngineGemini) {
		return false
	}
	if isOpen, _, _ := gemini.IsCircuitBreakerOpen(); isOpen {
		return false
	}
	return true
}

func matchWhisperProfileForRacer(cfg types.Config, target, lang string) (types.WhisperProfile, bool) {
	targetLower := strings.ToLower(strings.TrimSpace(target))
	for _, p := range cfg.WhisperProfiles {
		wp := config.NormalizeWhisperProfile(p)
		if !transcribe.WhisperProfileSupportsLanguage(wp, lang) || !transcribe.WhisperProfileUsable(wp) {
			continue
		}
		if strconv.Itoa(wp.ID) == targetLower ||
			strings.EqualFold(string(wp.Engine), targetLower) ||
			strings.EqualFold(wp.Name, target) ||
			strings.Contains(strings.ToLower(wp.Name), targetLower) ||
			(wp.CliBinary != "" && strings.EqualFold(wp.CliBinary, targetLower)) {
			return wp, true
		}
	}
	return types.WhisperProfile{}, false
}

func RunSpeculativeParallelRace(parentCtx context.Context, audioPath string, cfg types.Config, opts types.ProcOptions, totalDuration, speedFactor float64, whisperPrompt, whisperLang, dockerContainer string, isHebrew bool) (*types.TranscriptionData, []types.AdSegment, bool, error) {
	transcribe.AnnounceStart(totalDuration, opts.Quiet)
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if _, hasDeadline := parentCtx.Deadline(); !hasDeadline {
		ctx, cancel = context.WithTimeout(parentCtx, 30*time.Minute)
	} else {
		ctx, cancel = context.WithCancel(parentCtx)
	}
	defer cancel()

	if isHebrew && whisperLang == "" {
		whisperLang = "he"
	}
	lang := transcribe.WhisperTargetLanguage(cfg, isHebrew, whisperLang)
	racers := ResolveSpeculativeRacers(cfg, opts, lang)
	if len(racers) < 2 {
		return nil, nil, false, fmt.Errorf("insufficient competing services for speculative race (found %d)", len(racers))
	}

	if !opts.Quiet {
		racerNames := make([]string, len(racers))
		for i, r := range racers {
			racerNames[i] = r.Name
		}
		fmt.Printf("   Speculative competition: racing %s\n", strings.Join(racerNames, " vs "))
	}

	resultCh := make(chan SpeculativeCandidateResult, len(racers))
	chunkDur := cfg.GeminiChunkSecCapped(cfg.ChunkDurationSec)

	for _, racer := range racers {
		if racer.IsGemini {
			go func() {
				td, ads, err := gemini.ProcessWithGeminiConfig(ctx, audioPath, cfg, chunkDur)
				if err == nil && td != nil {
					transcribe.StampBackend(td, types.WhisperEngineGemini, cfg.GetGeminiModel())
				}
				resultCh <- SpeculativeCandidateResult{
					ServiceName: "Gemini",
					TD:          td,
					Ads:         ads,
					IsGemini:    true,
					Err:         err,
				}
			}()
		} else {
			go func(r SpeculativeRacer) {
				td, err := runLocalCandidateTranscription(ctx, audioPath, r.Profile, cfg, opts, totalDuration, speedFactor, whisperPrompt, whisperLang, dockerContainer)
				resultCh <- SpeculativeCandidateResult{
					ServiceName: r.Name,
					TD:          td,
					IsGemini:    false,
					Err:         err,
				}
			}(racer)
		}
	}

	return awaitRaceResults(ctx, cancel, resultCh, len(racers), opts.Quiet)
}

func awaitRaceResults(ctx context.Context, cancel context.CancelFunc, resultCh <-chan SpeculativeCandidateResult, totalRacers int, quiet bool) (*types.TranscriptionData, []types.AdSegment, bool, error) {
	remaining := totalRacers
	var errors []string

	for remaining > 0 {
		select {
		case res := <-resultCh:
			if res.Err == nil {
				cancel()
				if !quiet {
					fmt.Println("\n" + util.BoldGreen(fmt.Sprintf("Transcription complete: using %s result.", res.ServiceName)))
				}
				return res.TD, res.Ads, res.IsGemini, nil
			}
			remaining--
			errors = append(errors, fmt.Sprintf("%s:\n   %v", res.ServiceName, res.Err))
			if !quiet {
				fmt.Printf("\n%s\n   %s\n",
					util.BoldYellow(fmt.Sprintf("Transcription: %s failed:", res.ServiceName)),
					util.BoldYellow(strings.ReplaceAll(res.Err.Error(), "\n", "\n   ")),
				)
				if remaining > 0 {
					fmt.Printf("   ➔ %s\n\n", util.Bold("Continuing with remaining services..."))
				}
			}
		case <-ctx.Done():
			return nil, nil, false, ctx.Err()
		}
	}
	return nil, nil, false, fmt.Errorf("all competing services failed:\n   %s", strings.Join(errors, "\n   "))
}
