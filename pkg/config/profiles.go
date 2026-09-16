package config

import (
	"fmt"
	"strings"

	"pod/pkg/types"
	"pod/pkg/util"
)

var DefaultWhisperProfiles = []types.WhisperProfile{
	{
		ID:          1,
		Name:        "Local whisper-cli (tiny.en)",
		Engine:      types.WhisperEngineLocal,
		Model:       "tiny.en",
		SpeedFactor: 70.0,
		CliBinary:   "whisper-cli",
		Processors:  4,
		Threads:     4,
		Greedy:      true,
		Languages:   []string{"en"},
	},
	{
		ID:              2,
		Name:            "Docker Daemon (localhost:8088)",
		Engine:          types.WhisperEngineDocker,
		URL:             "http://127.0.0.1:8088/inference",
		SpeedFactor:     7.0,
		DockerContainer: "whisper",
		Languages:       []string{"en", "he"},
	},
	{
		ID:          3,
		Name:        "Gemini Flash (Google AI Studio Free)",
		Engine:      types.WhisperEngineGemini,
		Model:       "gemini-flash-latest",
		SpeedFactor: 60.0,
		Languages:   []string{"en", "he", "*"},
	},
}

var DefaultLLMProfiles = []types.LLMProfile{
	{ID: 1, Name: "Ollama Local (llama3.1:8b)", Type: "ollama", URL: "http://192.168.1.230:11434/v1/chat/completions", Model: "llama3.1:8b"},
	{ID: 2, Name: "OpenRouter - Claude 3.5 Sonnet", Type: "openrouter", URL: "https://openrouter.ai/api/v1/chat/completions", Model: "anthropic/claude-3.5-sonnet"},
	{ID: 3, Name: "OpenRouter - DeepSeek V4 Flash", Type: "openrouter", URL: "https://openrouter.ai/api/v1/chat/completions", Model: "deepseek/deepseek-v4-flash"},
	{ID: 4, Name: "OpenRouter - Gemini 2.5 Flash", Type: "openrouter", URL: "https://openrouter.ai/api/v1/chat/completions", Model: "google/gemini-2.5-flash"},
	{ID: 5, Name: "Google AI Studio - Gemini Flash (Free)", Type: "gemini", URL: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", Model: "gemini-flash-latest"},
}

func WhisperEngineBadge(engine types.WhisperEngine) string {
	switch engine {
	case types.WhisperEngineLocal:
		return "[LOCAL]"
	case types.WhisperEngineDocker:
		return "[DOCKER]"
	case types.WhisperEngineRemote:
		return "[REMOTE]"
	case types.WhisperEngineGemini:
		return "[GEMINI]"
	default:
		return "[" + strings.ToUpper(string(engine)) + "]"
	}
}

func InferWhisperEngine(wp types.WhisperProfile) types.WhisperEngine {
	if wp.Engine != "" {
		return wp.Engine
	}
	if wp.CliBinary != "" || wp.Model != "" && wp.URL == "" && wp.DockerContainer == "" {
		return types.WhisperEngineLocal
	}
	if wp.DockerContainer != "" || strings.Contains(wp.URL, "localhost") || strings.Contains(wp.URL, "127.0.0.1") {
		return types.WhisperEngineDocker
	}
	if strings.Contains(strings.ToLower(wp.Name), "gemini") {
		return types.WhisperEngineGemini
	}
	return types.WhisperEngineRemote
}

func SelectLLMProfile(cfg *types.Config, query string) (types.LLMProfile, error) {
	profile, err := selectLLMProfile(cfg, query)
	if err != nil {
		return profile, err
	}
	profile.APIKey = ResolveLLMAPIKey(profile, cfg)
	return profile, nil
}

func selectLLMProfile(cfg *types.Config, query string) (types.LLMProfile, error) {
	if cfg == nil || len(cfg.Profiles) == 0 {
		return types.LLMProfile{}, fmt.Errorf("no LLM profiles available")
	}
	if query == "" {
		if cfg.ActiveProfileID > 0 {
			for _, p := range cfg.Profiles {
				if p.ID == cfg.ActiveProfileID {
					return p, nil
				}
			}
		}
		for _, p := range cfg.Profiles {
			if p.ID == 3 || strings.Contains(strings.ToLower(p.Model), "deepseek-v4-flash") {
				return p, nil
			}
		}
		return cfg.Profiles[0], nil
	}

	id := 0
	fmt.Sscanf(query, "%d", &id)
	if id > 0 {
		for _, p := range cfg.Profiles {
			if p.ID == id {
				return p, nil
			}
		}
	}

	lowerQuery := strings.ToLower(query)
	for _, p := range cfg.Profiles {
		if strings.Contains(strings.ToLower(p.Name), lowerQuery) || strings.Contains(strings.ToLower(p.Model), lowerQuery) {
			return p, nil
		}
	}
	return types.LLMProfile{}, fmt.Errorf("LLM profile '%s' not found", query)
}

func GetProfileCost(profile types.LLMProfile) types.CostInfo {
	t := profile.Type
	u := profile.URL

	if t == "gemini" || strings.Contains(u, "googleapis.com") {
		return types.CostInfo{
			Type:     "Free Cloud",
			CostStr:  "Free ($0.00 / Google AI Studio)",
			Est1HStr: "$0.00",
		}
	}
	if t == "ollama" || strings.Contains(u, "11434") || strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1") {
		return types.CostInfo{
			Type:     "Local",
			CostStr:  "Free ($0.00 / Local GPU)",
			Est1HStr: "$0.00",
		}
	}

	return types.CostInfo{
		Type:     "Cloud/Custom",
		CostStr:  "Dynamic pricing / subscription",
		Est1HStr: "~$0.01 - $0.05 / 1-hr episode",
	}
}

func SetDefaultProfile(cfg *types.Config, targetID int) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	for _, p := range cfg.Profiles {
		if p.ID == targetID {
			cfg.ActiveProfileID = targetID
			return nil
		}
	}
	return fmt.Errorf("profile ID [%d] not found in configuration", targetID)
}

func NormalizeWhisperProfile(p types.WhisperProfile) types.WhisperProfile {
	if p.Engine == "" {
		p.Engine = InferWhisperEngine(p)
	}
	if p.SpeedFactor <= 0 {
		p.SpeedFactor = 70.0
	}
	return p
}

func GetActiveWhisperProfile(cfg *types.Config) types.WhisperProfile {
	if cfg == nil {
		return types.WhisperProfile{}
	}
	for _, wp := range cfg.WhisperProfiles {
		if wp.ID == cfg.ActiveWhisperID {
			return NormalizeWhisperProfile(wp)
		}
	}
	if len(cfg.WhisperProfiles) > 0 {
		return NormalizeWhisperProfile(cfg.WhisperProfiles[0])
	}
	return types.WhisperProfile{
		ID:          1,
		Name:        "Default Whisper",
		Engine:      types.WhisperEngineLocal,
		SpeedFactor: 70.0,
	}
}

func PrepareWhisperFallbackConfig(cfg types.Config) types.Config {
	fallback := cfg
	for _, wp := range cfg.WhisperProfiles {
		engine := wp.Engine
		if engine == "" {
			engine = InferWhisperEngine(wp)
		}
		if engine == types.WhisperEngineGemini || engine == types.WhisperEngineRemote {
			continue
		}
		if engine == types.WhisperEngineLocal || engine == types.WhisperEngineDocker || util.IsLocalHost(util.ExtractHost(wp.URL)) {
			fallback.ActiveWhisperID = wp.ID
			return fallback
		}
	}
	for _, wp := range cfg.WhisperProfiles {
		engine := wp.Engine
		if engine == "" {
			engine = InferWhisperEngine(wp)
		}
		if engine != types.WhisperEngineGemini {
			fallback.ActiveWhisperID = wp.ID
			return fallback
		}
	}
	fallback.WhisperEngine = types.WhisperEngineLocal
	return fallback
}
