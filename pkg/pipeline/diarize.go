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
	td, model, err := sendForSpeakers(sourceAudioFile, wp, opts, duration, lang, rep)
	if err != nil || lang != "" || td.Language == "" {
		return td, err
	}
	// The language was left to the server. If it found one that has a model of
	// its own (Hebrew, say), the first pass used the general model and read the
	// text noticeably worse, so it is done again with the right one.
	if better := transcribe.ModelForLanguage(wp, td.Language); better != model {
		rep.Infof("Detected language %s; transcribing again with %s...", td.Language, better)
		again, _, againErr := sendForSpeakers(sourceAudioFile, wp, opts, duration, td.Language, rep)
		if againErr != nil {
			rep.Warnf("the second pass failed (%v); keeping the first", againErr)
			return td, nil
		}
		td = again
	}
	return td, nil
}

// sendForSpeakers makes one request to the diarizing server, with the model that
// suits lang, and returns the transcript with the model used.
func sendForSpeakers(sourceAudioFile string, wp types.WhisperProfile, opts types.ProcOptions, duration float64, lang string, rep progress.Reporter) (*types.TranscriptionData, string, error) {
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
		return nil, model, err
	}
	transcribe.StampBackend(td, wp.Engine, model)
	return td, model, nil
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
