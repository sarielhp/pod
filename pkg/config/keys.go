package config

import (
	"encoding/json"
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
	resolved := expandHome(path)
	if resolved == "" {
		return ""
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

// defaultPodcastIndexKeyFiles are where Podcast Index credentials are looked
// for when the configuration names no path, most specific first. The .json
// spelling is preferred because it is unambiguous; the labelled text file it
// replaced is still read so an existing install keeps working.
var defaultPodcastIndexKeyFiles = []string{
	"~/.config/auth/podcast_index_key.json",
	"~/.config/auth/podcast_index_key",
	"~/.config/auth/podcastindex_api_key",
}

// ResolvePodcastIndexCredentials returns the Podcast Index API key and secret.
//
// Podcast Index signs every request with both halves, so neither is useful
// alone. The configuration may name the file they are read from; otherwise the
// auth folder is searched. Environment variables win over the file, so a
// one-off run can be pointed at another account without editing anything. The
// credentials themselves never live in config.json — only the path to them
// does — because that file is copied, synced and shared.
func ResolvePodcastIndexCredentials(cfg *types.Config) (key, secret string) {
	paths := defaultPodcastIndexKeyFiles
	if cfg != nil {
		if p := strings.TrimSpace(cfg.PodcastIndexAPIKeyFile); p != "" {
			paths = []string{p}
		}
	}
	for _, p := range paths {
		k, s := readPodcastIndexKeyFile(p)
		if k == "" {
			continue
		}
		key, secret = k, s
		break
	}
	if v := strings.TrimSpace(os.Getenv("PODCASTINDEX_API_KEY")); v != "" {
		key = v
	}
	if v := strings.TrimSpace(os.Getenv("PODCASTINDEX_API_SECRET")); v != "" {
		secret = v
	}
	return key, secret
}

// readPodcastIndexKeyFile reads credentials from path. Two formats are
// understood: a JSON object with api_key/api_secret (or key/secret), and the
// labelled text file it replaced, "Key: …" and "Secret: …" on their own lines.
// A file holding a single bare value is taken to be the key alone.
func readPodcastIndexKeyFile(path string) (key, secret string) {
	resolved := expandHome(path)
	if resolved == "" {
		return "", ""
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", ""
	}
	if k, s := parsePodcastIndexJSON(data); k != "" || s != "" {
		return k, s
	}
	return parsePodcastIndexLines(string(data))
}

func parsePodcastIndexJSON(data []byte) (key, secret string) {
	var doc struct {
		APIKey    string `json:"api_key"`
		APISecret string `json:"api_secret"`
		Key       string `json:"key"`
		Secret    string `json:"secret"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", ""
	}
	key = firstNonEmpty(doc.APIKey, doc.Key)
	secret = firstNonEmpty(doc.APISecret, doc.Secret)
	return strings.TrimSpace(key), strings.TrimSpace(secret)
}

func parsePodcastIndexLines(text string) (key, secret string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		label, value, found := strings.Cut(line, ":")
		if !found {
			if key == "" && secret == "" {
				key = line
			}
			continue
		}
		value = strings.TrimSpace(value)
		switch normalizeCredentialLabel(label) {
		case "key":
			if value != "" {
				key = value
			}
		case "secret":
			if value != "" {
				secret = value
			}
		}
	}
	return key, secret
}

func normalizeCredentialLabel(label string) string {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(label), "_", " ")) {
	case "key", "api key", "apikey":
		return "key"
	case "secret", "api secret", "apisecret":
		return "secret"
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
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
	if v := os.Getenv("PODCASTINDEX_API_KEY_FILE"); v != "" {
		cfg.PodcastIndexAPIKeyFile = v
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
