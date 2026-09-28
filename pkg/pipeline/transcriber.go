package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pod/pkg/config"
	"pod/pkg/gemini"
	"pod/pkg/progress"
	"pod/pkg/transcribe"
	"pod/pkg/types"
)

// Transcriber transcribes audio into structured text.
type Transcriber interface {
	Transcribe(ctx context.Context, wav string, duration float64) (*types.TranscriptionData, error)
}

// NamedTranscriber pairs a name with a transcriber for speculative racing.
type NamedTranscriber struct {
	Name        string
	Transcriber Transcriber
}

type fallbackTranscriber struct {
	primary Transcriber
	backup  Transcriber
	rep     progress.Reporter
}

// NewFallbackTranscriber creates a transcriber that tries primary first and backup upon error.
func NewFallbackTranscriber(primary, backup Transcriber, rep progress.Reporter) Transcriber {
	return &fallbackTranscriber{
		primary: primary,
		backup:  backup,
		rep:     progress.Or(rep),
	}
}

func (f *fallbackTranscriber) Transcribe(ctx context.Context, wav string, duration float64) (*types.TranscriptionData, error) {
	td, err := f.primary.Transcribe(ctx, wav, duration)
	if err == nil && td != nil {
		return td, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.rep != nil {
		f.rep.Warnf("Primary transcriber failed (%v); falling back to backup", err)
	}
	return f.backup.Transcribe(ctx, wav, duration)
}

type racingTranscriber struct {
	racers []NamedTranscriber
	rep    progress.Reporter
}

// NewRacingTranscriber creates a transcriber that races multiple candidates in parallel.
func NewRacingTranscriber(racers []NamedTranscriber, rep progress.Reporter) Transcriber {
	return &racingTranscriber{
		racers: racers,
		rep:    progress.Or(rep),
	}
}

type raceOutcome struct {
	name string
	td   *types.TranscriptionData
	err  error
}

func (r *racingTranscriber) Transcribe(ctx context.Context, wav string, duration float64) (*types.TranscriptionData, error) {
	if len(r.racers) == 0 {
		return nil, errors.New("no transcribers configured for racing")
	}
	if len(r.racers) == 1 {
		return r.racers[0].Transcriber.Transcribe(ctx, wav, duration)
	}

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	names := make([]string, len(r.racers))
	for i, racer := range r.racers {
		names[i] = racer.Name
	}
	r.rep.Infof("Speculative competition: racing %s", strings.Join(names, " vs "))

	resultCh := make(chan raceOutcome, len(r.racers))
	for _, racer := range r.racers {
		go func(target NamedTranscriber) {
			td, err := target.Transcriber.Transcribe(raceCtx, wav, duration)
			resultCh <- raceOutcome{name: target.Name, td: td, err: err}
		}(racer)
	}

	var errList []string
	for i := 0; i < len(r.racers); i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case outcome := <-resultCh:
			if outcome.err == nil && outcome.td != nil {
				cancel()
				r.rep.Infof("Speculative race winner: %s", outcome.name)
				return outcome.td, nil
			}
			if outcome.err != nil {
				errList = append(errList, fmt.Sprintf("%s: %v", outcome.name, outcome.err))
			}
		}
	}

	return nil, fmt.Errorf("all transcribers failed: %s", strings.Join(errList, "; "))
}

type geminiTranscriber struct {
	cfg      types.Config
	chunkDur float64
	rep      progress.Reporter
}

// NewGeminiTranscriber creates a transcriber backed by Gemini Flash audio
// processing, reporting progress to rep. A nil Reporter is silent.
func NewGeminiTranscriber(cfg types.Config, chunkDurSec float64, rep progress.Reporter) Transcriber {
	return &geminiTranscriber{
		cfg:      cfg,
		chunkDur: chunkDurSec,
		rep:      progress.Or(rep),
	}
}

func (g *geminiTranscriber) Transcribe(ctx context.Context, wav string, duration float64) (*types.TranscriptionData, error) {
	chunkDur := g.chunkDur
	if chunkDur <= 0 {
		chunkDur = g.cfg.GeminiChunkSecCapped(g.cfg.ChunkDurationSec)
	}
	td, ads, err := gemini.ProcessWithGeminiConfig(ctx, wav, g.cfg, chunkDur, g.rep)
	if err != nil {
		return nil, err
	}
	if td != nil {
		transcribe.StampBackend(td, types.WhisperEngineGemini, g.cfg.GetGeminiModel())
		td.Cuts = ads
	}
	return td, nil
}

// WhisperConfig configures a Whisper-backed transcriber.
type WhisperConfig struct {
	Profile         types.WhisperProfile
	Config          types.Config
	Options         types.ProcOptions
	SpeedFactor     float64
	Prompt          string
	Language        string
	DockerContainer string
	Progress        progress.Reporter
}

type whisperTranscriber struct {
	cfg WhisperConfig
}

// NewWhisperTranscriber creates a transcriber backed by local CLI or remote Whisper server.
func NewWhisperTranscriber(cfg WhisperConfig) Transcriber {
	return &whisperTranscriber{cfg: cfg}
}

func (w *whisperTranscriber) Transcribe(ctx context.Context, wav string, duration float64) (*types.TranscriptionData, error) {
	speedFactor := w.cfg.SpeedFactor
	if speedFactor <= 0 {
		speedFactor = w.cfg.Config.WhisperSpeedFactor
	}
	if speedFactor <= 0 {
		speedFactor = 7.0
	}
	return runLocalCandidateTranscription(
		ctx,
		wav,
		w.cfg.Profile,
		w.cfg.Config,
		w.cfg.Options,
		duration,
		speedFactor,
		w.cfg.Prompt,
		w.cfg.Language,
		w.cfg.DockerContainer,
		w.cfg.Progress,
	)
}

// BuildTranscriber selects and composes the appropriate Transcriber for the configuration.
func BuildTranscriber(cfg types.Config, opts types.ProcOptions, prompt, lang, dockerContainer string, rep progress.Reporter) Transcriber {
	r := progress.Or(rep)
	if racers := ResolveSpeculativeRacers(cfg, opts, lang); len(racers) >= 2 {
		named := make([]NamedTranscriber, 0, len(racers))
		for _, racer := range racers {
			if racer.IsGemini {
				named = append(named, NamedTranscriber{
					Name:        "Gemini",
					Transcriber: NewGeminiTranscriber(cfg, 0, r),
				})
			} else {
				named = append(named, NamedTranscriber{
					Name: racer.Name,
					Transcriber: NewWhisperTranscriber(WhisperConfig{
						Profile:         racer.Profile,
						Config:          cfg,
						Options:         opts,
						Prompt:          prompt,
						Language:        lang,
						DockerContainer: dockerContainer,
						Progress:        r,
					}),
				})
			}
		}
		return NewRacingTranscriber(named, r)
	}

	activeProfile := config.GetActiveWhisperProfile(&cfg)
	if opts.WhisperEngine == string(types.WhisperEngineGemini) ||
		(opts.WhisperEngine == "" && activeProfile.Engine == types.WhisperEngineGemini) {
		primary := NewGeminiTranscriber(cfg, 0, r)
		fallbackCfg := config.PrepareWhisperFallbackConfig(cfg)
		fallbackProfile := config.GetActiveWhisperProfile(&fallbackCfg)
		backup := NewWhisperTranscriber(WhisperConfig{
			Profile:         fallbackProfile,
			Config:          fallbackCfg,
			Options:         opts,
			Prompt:          prompt,
			Language:        lang,
			DockerContainer: dockerContainer,
			Progress:        r,
		})
		return NewFallbackTranscriber(primary, backup, r)
	}

	return NewWhisperTranscriber(WhisperConfig{
		Profile:         activeProfile,
		Config:          cfg,
		Options:         opts,
		Prompt:          prompt,
		Language:        lang,
		DockerContainer: dockerContainer,
	})
}
