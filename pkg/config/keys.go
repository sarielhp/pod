package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pod/pkg/types"
	"pod/pkg/util"
)

func ResolveGeminiAPIKey(cfg *types.Config) string {
	if cfg == nil || !cfg.IsGeminiAPIKeyEnabled() {
		if cfg != nil && cfg.GeminiAPIKey != "" {
			_ = util.ZeroWipeKey(cfg.GeminiAPIKey)
		}
		if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
			_ = util.ZeroWipeKey(envKey)
			os.Unsetenv("GEMINI_API_KEY")
		}
		return ""
	}

	if cfg.GeminiAPIKey != "" && !util.IsZeroedKey(cfg.GeminiAPIKey) {
		return cfg.GeminiAPIKey
	}
	if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" && !util.IsZeroedKey(envKey) {
		return envKey
	}
	keyFile := ""
	if cfg != nil {
		keyFile = cfg.GeminiAPIKeyFile
	}
	if envFile := os.Getenv("GEMINI_API_KEY_FILE"); envFile != "" {
		keyFile = envFile
	}
	if keyFile != "" {
		return readKeyFile(keyFile)
	}
	return ""
}

func readKeyFile(path string) string {
	if path == "" {
		return ""
	}
	resolved := path
	if strings.HasPrefix(path, "~/") || path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				resolved = home
			} else {
				resolved = filepath.Join(home, path[2:])
			}
		}
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return ""
	}
	k := strings.TrimSpace(string(data))
	if util.IsZeroedKey(k) {
		return ""
	}
	return k
}

func sanitizeDisabledAPIKeys(cfg *types.Config) {
	if cfg == nil {
		return
	}
	if !cfg.IsGeminiAPIKeyEnabled() {
		if cfg.GeminiAPIKey != "" {
			cfg.GeminiAPIKey = util.ZeroWipeKey(cfg.GeminiAPIKey)
		}
		if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
			_ = util.ZeroWipeKey(envKey)
			os.Unsetenv("GEMINI_API_KEY")
		}
	}
	if !cfg.IsOpenRouterAPIKeyEnabled() {
		for i := range cfg.Profiles {
			p := &cfg.Profiles[i]
			if p.Type == "openrouter" || strings.Contains(p.URL, "openrouter") || strings.HasPrefix(p.APIKey, "sk-or-") {
				if p.APIKey != "" {
					p.APIKey = util.ZeroWipeKey(p.APIKey)
				}
			}
		}
		if envKey := os.Getenv("OPENROUTER_API_KEY"); envKey != "" {
			_ = util.ZeroWipeKey(envKey)
			os.Unsetenv("OPENROUTER_API_KEY")
		}
		if envKey := os.Getenv("OPENAI_API_KEY"); envKey != "" && strings.HasPrefix(envKey, "sk-or-") {
			_ = util.ZeroWipeKey(envKey)
			os.Unsetenv("OPENAI_API_KEY")
		}
	}
}

func validateOpenRouterKey(profile types.LLMProfile, apiKey string, enabled bool) (string, error) {
	isOpenRouter := profile.Type == "openrouter" || strings.Contains(profile.URL, "openrouter") || strings.HasPrefix(apiKey, "sk-or-")
	if isOpenRouter {
		if !enabled || util.IsZeroedKey(apiKey) {
			if apiKey != "" {
				_ = util.ZeroWipeKey(apiKey)
			}
			return "", fmt.Errorf("openrouter API key is disabled in configuration")
		}
	}
	return apiKey, nil
}

func validateGeminiKey(apiKey string, enabled bool) (string, error) {
	if !enabled || util.IsZeroedKey(apiKey) {
		if apiKey != "" {
			_ = util.ZeroWipeKey(apiKey)
		}
		return "", fmt.Errorf("gemini API key is disabled in configuration")
	}
	return apiKey, nil
}

func applyAPIKeyEnvOverrides(cfg *types.Config) {
	if cfg == nil {
		return
	}
	if v := os.Getenv("GEMINI_API_KEY_FILE"); v != "" {
		cfg.GeminiAPIKeyFile = v
	}
	if v := os.Getenv("GEMINI_API_KEY_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.GeminiAPIKeyEnabled = &b
		}
	}
	if v := os.Getenv("OPENROUTER_API_KEY_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.OpenRouterAPIKeyEnabled = &b
		}
	}
}

func readAuthSecret(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".config", "auth", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	k := strings.TrimSpace(string(data))
	if util.IsZeroedKey(k) {
		return ""
	}
	return k
}

func resolveOpenRouterAPIKey(profile types.LLMProfile, cfg *types.Config) string {
	isOpenRouter := profile.Type == "openrouter" || strings.Contains(profile.URL, "openrouter") || strings.HasPrefix(profile.APIKey, "sk-or-")
	if !isOpenRouter {
		return profile.APIKey
	}
	if cfg == nil || !cfg.IsOpenRouterAPIKeyEnabled() {
		if profile.APIKey != "" {
			_ = util.ZeroWipeKey(profile.APIKey)
		}
		if envKey := os.Getenv("OPENROUTER_API_KEY"); envKey != "" {
			_ = util.ZeroWipeKey(envKey)
			os.Unsetenv("OPENROUTER_API_KEY")
		}
		return ""
	}
	if profile.APIKey != "" && !util.IsZeroedKey(profile.APIKey) {
		return profile.APIKey
	}
	if envKey := os.Getenv("OPENROUTER_API_KEY"); envKey != "" && !util.IsZeroedKey(envKey) {
		return envKey
	}
	if envKey := os.Getenv("OPENAI_API_KEY"); envKey != "" && strings.HasPrefix(envKey, "sk-or-") && !util.IsZeroedKey(envKey) {
		return envKey
	}
	return readAuthSecret("openrouter_api_key")
}

func ResolveLLMAPIKey(profile types.LLMProfile, cfg *types.Config) string {
	if profile.Type == "gemini" || strings.Contains(profile.URL, "googleapis.com") {
		key := profile.APIKey
		if key == "" || util.IsZeroedKey(key) {
			key = ResolveGeminiAPIKey(cfg)
		}
		if cfg != nil {
			if valKey, err := validateGeminiKey(key, cfg.IsGeminiAPIKeyEnabled()); err == nil {
				return valKey
			}
			return ""
		}
		return key
	}
	key := resolveOpenRouterAPIKey(profile, cfg)
	if cfg != nil {
		if valKey, err := validateOpenRouterKey(profile, key, cfg.IsOpenRouterAPIKeyEnabled()); err == nil {
			return valKey
		}
		return ""
	}
	return key
}

func resolveAuthFolderCredentials(cfg *types.Config) {
	if cfg == nil {
		return
	}
	if cfg.PodfetchAPIKey == "" {
		cfg.PodfetchAPIKey = readAuthSecret("podfetch_api_key")
	}
	if cfg.PodfetchPass == "" {
		if pass := readAuthSecret("podfetch_password"); pass != "" {
			cfg.PodfetchPass = pass
		} else {
			cfg.PodfetchPass = readAuthSecret("podfetch_pass")
		}
	}
}
