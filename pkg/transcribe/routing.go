package transcribe

import (
	"os/exec"
	"strings"

	"pod/pkg/config"
	"pod/pkg/types"
)

func ContainsHebrew(s string) bool {
	for _, r := range s {
		if (r >= 0x0590 && r <= 0x05FF) || (r >= 0xFB1D && r <= 0xFB4F) {
			return true
		}
	}
	return false
}

func IsHebrewAudio(audioPath string, id3Tags map[string]string, whisperLang string) bool {
	if strings.EqualFold(whisperLang, "he") || strings.EqualFold(whisperLang, "heb") || strings.EqualFold(whisperLang, "hebrew") {
		return true
	}
	if ContainsHebrew(audioPath) {
		return true
	}
	for _, v := range id3Tags {
		if ContainsHebrew(v) {
			return true
		}
	}
	return false
}

func WhisperProfileSupportsLanguage(wp types.WhisperProfile, lang string) bool {
	if lang == "" || lang == "auto" {
		return true
	}
	if len(wp.Languages) == 0 {
		if wp.Engine == types.WhisperEngineLocal && (strings.Contains(wp.Model, ".en") || wp.Model == "tiny.en") {
			return strings.EqualFold(lang, "en")
		}
		return true
	}
	for _, l := range wp.Languages {
		if l == "*" || strings.EqualFold(l, lang) {
			return true
		}
	}
	return false
}

// WhisperTargetLanguage decides which language the Whisper backend has to
// support: an explicitly requested language wins, Hebrew detection comes
// next, and anything else is assumed to be English.
func WhisperTargetLanguage(cfg types.Config, isHebrew bool, requested string) string {
	if requested != "" && !strings.EqualFold(requested, "auto") {
		return requested
	}
	if isHebrew {
		return "he"
	}
	if cfg.WhisperLanguage != "" && !strings.EqualFold(cfg.WhisperLanguage, "auto") {
		return cfg.WhisperLanguage
	}
	return "en"
}

// WhisperProfileUsable reports whether a profile can actually run, so
// speed-based routing never picks a local program whose binary or model file
// is missing. It is a variable so tests can route without touching the host.
var WhisperProfileUsable = func(wp types.WhisperProfile) bool {
	if wp.Engine != types.WhisperEngineLocal {
		return wp.URL != ""
	}
	bin := ResolveWhisperCLIBinary(wp.CliBinary)
	if _, err := exec.LookPath(bin); err != nil {
		return false
	}
	_, err := ResolveWhisperModelPath(wp.Model)
	return err == nil
}

// ResolveWhisperProfileForLanguage picks the fastest usable Whisper backend
// that supports lang. Speed decides rather than the configured default,
// because the backends differ by an order of magnitude: English routes to the
// local whisper-cli program, while Hebrew, which the English-only models
// cannot handle, falls back to the Docker server.
func ResolveWhisperProfileForLanguage(cfg types.Config, lang string) types.WhisperProfile {
	active := config.GetActiveWhisperProfile(&cfg)
	var best types.WhisperProfile
	found := false
	for _, p := range cfg.WhisperProfiles {
		wp := config.NormalizeWhisperProfile(p)
		if wp.Engine == types.WhisperEngineGemini || wp.Diarize || !WhisperProfileSupportsLanguage(wp, lang) || !WhisperProfileUsable(wp) {
			continue
		}
		if !found || fasterWhisperProfile(wp, best, active) {
			best, found = wp, true
		}
	}
	if found {
		return best
	}
	if active.Engine == types.WhisperEngineGemini {
		fallbackCfg := config.PrepareWhisperFallbackConfig(cfg)
		return config.GetActiveWhisperProfile(&fallbackCfg)
	}
	return active
}

func fasterWhisperProfile(candidate, best, active types.WhisperProfile) bool {
	if candidate.SpeedFactor != best.SpeedFactor {
		return candidate.SpeedFactor > best.SpeedFactor
	}
	return candidate.ID == active.ID
}

// ResolveLocalWhisperProfile keeps the Hebrew/English shorthand used by the
// speculative race, where Gemini is already the other racer.
func ResolveLocalWhisperProfile(cfg types.Config, isHebrew bool) types.WhisperProfile {
	return ResolveWhisperProfileForLanguage(cfg, WhisperTargetLanguage(cfg, isHebrew, ""))
}

// ResolveDiarizingProfile picks the server that labels speakers for lang: the
// one whose languages name lang exactly, else any that supports it. An empty lang
// matches any. The second result is false when no such profile is configured.
func ResolveDiarizingProfile(cfg types.Config, lang string) (types.WhisperProfile, bool) {
	var best types.WhisperProfile
	bestScore := 0
	for _, p := range cfg.WhisperProfiles {
		wp := config.NormalizeWhisperProfile(p)
		if !wp.Diarize || !WhisperProfileUsable(wp) || !WhisperProfileSupportsLanguage(wp, lang) {
			continue
		}
		score := 1
		for _, l := range wp.Languages {
			if strings.EqualFold(l, lang) {
				score = 2
			}
		}
		if score > bestScore {
			best, bestScore = wp, score
		}
	}
	return best, bestScore > 0
}

// ModelForLanguage is the model to ask a server for when transcribing lang: the
// profile's override for that language, else its default model.
func ModelForLanguage(wp types.WhisperProfile, lang string) string {
	for l, m := range wp.ModelByLanguage {
		if strings.EqualFold(l, lang) {
			return m
		}
	}
	return wp.Model
}
