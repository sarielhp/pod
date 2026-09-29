package pipeline

import (
	"context"
	"fmt"

	"pod/pkg/progress"
	"pod/pkg/transcribe"
	"pod/pkg/types"
)

// transcribeWithSpeakers sends the audio to a diarizing server (WhisperX), which
// returns the transcript with a speaker on every segment. Such a server is slower
// than plain Whisper and is never chosen by speed routing, so it is used only
// when speakers are asked for. The whole file goes in one request, because
// speaker labels are consistent only within one call: labels from separately
// transcribed chunks would not match up.
func transcribeWithSpeakers(sourceAudioFile string, cfg types.Config, opts types.ProcOptions, duration float64, whisperLang string, rep progress.Reporter) (*types.TranscriptionData, error) {
	rep = progress.Or(rep)
	lang := speakersLanguage(sourceAudioFile, cfg, opts, whisperLang)
	wp, ok := transcribe.ResolveDiarizingProfile(cfg, lang)
	if !ok {
		return nil, fmt.Errorf("no Whisper profile can label speakers%s: add one with \"diarize\": true in the config", forLanguage(lang))
	}
	model := transcribe.ModelForLanguage(wp, lang)
	fields := map[string]string{"diarize": "true"}
	if model != "" {
		fields["model"] = model
	}
	rep.Infof("Labelling speakers with %s (%s)...", wp.URL, describeModel(model, lang))
	td, err := transcribe.TranscribeWhisperRequest(context.Background(), transcribe.WhisperRequest{
		AudioPath: sourceAudioFile, URL: wp.URL, Quiet: opts.Quiet, Verbose: opts.Verbose,
		TotalDuration: duration, SpeedFactor: 1.0, Language: lang, Fields: fields,
	})
	if err != nil {
		return nil, err
	}
	transcribe.StampBackend(td, wp.Engine, model)
	return td, nil
}

// speakersLanguage is the language to ask for: the one named on the command
// line, else Hebrew when the file's name says so, else the configured language,
// else empty so the server detects it.
func speakersLanguage(sourceAudioFile string, cfg types.Config, opts types.ProcOptions, whisperLang string) string {
	switch {
	case opts.Language != "":
		return opts.Language
	case whisperLang != "":
		return whisperLang
	case transcribe.IsHebrewAudio(sourceAudioFile, nil, ""):
		return "he"
	case cfg.WhisperLanguage != "" && cfg.WhisperLanguage != "auto":
		return cfg.WhisperLanguage
	}
	return ""
}

func forLanguage(lang string) string {
	if lang == "" {
		return ""
	}
	return " for language " + lang
}

func describeModel(model, lang string) string {
	if model == "" {
		model = "server default model"
	}
	if lang == "" {
		return model + ", language auto-detected"
	}
	return model + ", language " + lang
}
