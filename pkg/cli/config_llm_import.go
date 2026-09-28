package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"pod/pkg/config"
	"pod/pkg/util"
)

func copyLLMFromOpenCode(w io.Writer, cfg *Config) error {
	cleanModel, cleanSmallModel, ok := readOpenCodeConfig()
	if !ok {
		return nil
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if !cfg.IsOpenRouterAPIKeyEnabled() && apiKey != "" {
		apiKey = util.ZeroWipeKey(apiKey)
	}

	fmt.Fprintln(w, "Copying LLM settings from OpenCode...")
	fmt.Fprintf(w, "   - Imported Primary Model: '%s'\n", cleanModel)
	fmt.Fprintf(w, "   - Imported Small Model:   '%s'\n", cleanSmallModel)
	if apiKey != "" {
		fmt.Fprintln(w, "   - API Key detected: Yes")
	} else {
		fmt.Fprintln(w, "   - API Key detected: No key in ENV (Set OPENROUTER_API_KEY)")
	}

	nextID := 0
	for _, p := range cfg.Profiles {
		if p.ID > nextID {
			nextID = p.ID
		}
	}
	nextID++

	updated := false
	for _, mod := range []string{cleanModel, cleanSmallModel} {
		if mod != "" && upsertOpenCodeProfile(w, cfg, mod, apiKey, &nextID) {
			updated = true
		}
	}

	if updated {
		for i := range cfg.Profiles {
			if cfg.Profiles[i].Model == cleanModel {
				cfg.ActiveProfileID = cfg.Profiles[i].ID
				break
			}
		}
		if err := saveConfig(cfg); err != nil {
			return err
		}
		fmt.Fprintln(w, "Successfully imported OpenCode configuration!")
	}
	return nil
}

func readOpenCodeConfig() (string, string, bool) {
	ocPath := config.OpencodeConfigPath()
	if ocPath == "" || !util.FileExists(ocPath) {
		fmt.Fprintf(os.Stderr, "OpenCode configuration file not found.\n")
		return "", "", false
	}

	data, err := os.ReadFile(ocPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading OpenCode config: %v\n", err)
		return "", "", false
	}

	var ocConfig struct {
		Model      string `json:"model"`
		SmallModel string `json:"small_model"`
	}
	if err := json.Unmarshal(data, &ocConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing OpenCode config: %v\n", err)
		return "", "", false
	}

	return strings.TrimPrefix(ocConfig.Model, "openrouter/"), strings.TrimPrefix(ocConfig.SmallModel, "openrouter/"), true
}

func upsertOpenCodeProfile(w io.Writer, cfg *Config, mod, apiKey string, nextID *int) bool {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Model == mod || strings.Contains(cfg.Profiles[i].Name, mod) {
			if apiKey != "" {
				cfg.Profiles[i].APIKey = apiKey
			}
			cfg.Profiles[i].URL = "https://openrouter.ai/api/v1/chat/completions"
			cfg.Profiles[i].Type = "openrouter"
			fmt.Fprintf(w, "   Updated existing profile [%d] for model '%s'\n", cfg.Profiles[i].ID, mod)
			return true
		}
	}

	newProfile := LLMProfile{
		ID:     *nextID,
		Name:   fmt.Sprintf("OpenRouter - %s", mod),
		Type:   "openrouter",
		URL:    "https://openrouter.ai/api/v1/chat/completions",
		Model:  mod,
		APIKey: apiKey,
	}
	cfg.Profiles = append(cfg.Profiles, newProfile)
	fmt.Fprintf(w, "   Added new profile [%d] for model '%s'\n", *nextID, mod)
	*nextID++
	return true
}
