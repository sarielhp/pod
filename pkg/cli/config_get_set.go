package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"pod/pkg/config"
)

func handleConfigSetAPIKey(cfg *Config, key, val string) (bool, error) {
	switch strings.ToLower(strings.ReplaceAll(key, "_", "-")) {
	case "gemini-api-key-enabled":
		b, err := strconv.ParseBool(val)
		if err != nil {
			return true, fmt.Errorf("invalid boolean value for %s: %s", key, val)
		}
		cfg.GeminiAPIKeyEnabled = &b
		return true, nil
	case "openrouter-api-key-enabled":
		b, err := strconv.ParseBool(val)
		if err != nil {
			return true, fmt.Errorf("invalid boolean value for %s: %s", key, val)
		}
		cfg.OpenRouterAPIKeyEnabled = &b
		return true, nil
	case "gemini-api-key":
		cfg.GeminiAPIKey = val
		return true, nil
	case "gemini-api-key-file", "gemini-key-file", "api-key-file":
		cfg.GeminiAPIKeyFile = val
		return true, nil
	case "gemini-model":
		cfg.GeminiModel = val
		return true, nil
	case "speculative-transcription", "speculative-competition":
		b, err := strconv.ParseBool(val)
		if err != nil {
			return true, fmt.Errorf("invalid boolean value for %s: %s", key, val)
		}
		cfg.SpeculativeTranscription = &b
		return true, nil
	case "competing-services", "speculative-services":
		parts := strings.Split(val, ",")
		var services []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				services = append(services, trimmed)
			}
		}
		cfg.CompetingServices = services
		return true, nil
	}
	return false, nil
}

func handleConfigSetBackend(cfg *Config, normKey, val string) bool {
	switch normKey {
	case "podcasts-dir", "podcasts.dir", "dir":
		cfg.PodcastsDir = val
	case "backend-type", "backend.type", "backend":
		cfg.BackendType = val
	case "podfetch-url", "podfetch.url":
		cfg.PodfetchURL = val
	case "podfetch-user", "podfetch.user":
		cfg.PodfetchUser = val
	case "podfetch-pass", "podfetch.pass":
		cfg.PodfetchPass = val
	case "podfetch-api-key", "podfetch.api-key", "podfetch-key", "podfetch-token":
		cfg.PodfetchAPIKey = val
	case "podfetch-db-path", "podfetch.db-path", "podfetch-db", "podfetch.db", "podfetch-sqlite-db-path":
		cfg.PodfetchDBPath = val
	case "server-base-url", "server.base-url", "server-url", "base-url":
		cfg.ServerBaseURL = strings.TrimRight(val, "/")
	default:
		return false
	}
	return true
}

func handleConfigSet(w io.Writer, cfg *Config, key, val string) error {
	if handled, err := handleConfigSetAPIKey(cfg, key, val); handled {
		if err != nil {
			return err
		}
		_ = config.SaveConfig(cfg)
		fmt.Fprintf(w, "Updated '%s' = '%s'\n", key, val)
		return nil
	}
	normKey := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
	if handleConfigSetBackend(cfg, normKey, val) {
		_ = config.SaveConfig(cfg)
		fmt.Fprintf(w, "Updated '%s' = '%s'\n", key, val)
		return nil
	}
	switch normKey {
	case "whisper-url", "whisper.url":
		cfg.WhisperURL = val
	case "whisper-language", "whisper.language", "language", "lang":
		cfg.WhisperLanguage = val
	case "whisper-wake-command", "whisper.wake-command", "wake-command":
		cfg.WhisperWakeCommand = val
	case "whisper-speed-factor", "whisper.speed-factor", "speed-factor":
		if sf, err := strconv.ParseFloat(val, 64); err == nil && sf > 0 {
			cfg.WhisperSpeedFactor = sf
		} else {
			return fmt.Errorf("invalid speed factor: %s", val)
		}
	case "active-profile-id", "profile-id", "llm-id":
		if id, err := strconv.Atoi(val); err == nil && id > 0 {
			cfg.ActiveProfileID = id
		} else {
			return fmt.Errorf("invalid profile id: %s", val)
		}
	case "active-whisper-id", "whisper-id":
		if id, err := strconv.Atoi(val); err == nil && id > 0 {
			cfg.ActiveWhisperID = id
		} else {
			return fmt.Errorf("invalid whisper id: %s", val)
		}
	case "default-download-policy", "default-download-mode", "download-policy":
		cfg.DefaultDownloadPolicy = config.NormalizeDownloadPolicy(val)
	case "default-download-k", "default-k", "download-k":
		if k, err := strconv.Atoi(val); err == nil && k > 0 {
			cfg.DefaultDownloadK = k
		} else {
			return fmt.Errorf("invalid default download k: %s", val)
		}
	case "default-ad-policy", "default-ad-removal", "default-ad-mode", "ad-policy", "ad-removal":
		cfg.DefaultAdRemoval = config.NormalizeAdRemovalMode(val)
	default:
		return fmt.Errorf("unknown configuration key: '%s'", key)
	}
	_ = config.SaveConfig(cfg)
	fmt.Fprintf(w, "Updated '%s' = '%s'\n", key, val)
	return nil
}

func handleConfigGet(w io.Writer, cfg Config, key string) error {
	switch strings.ToLower(strings.ReplaceAll(key, "_", "-")) {
	case "podcasts-dir", "podcasts.dir", "dir":
		fmt.Fprintln(w, cfg.PodcastsDir)
	case "backend-type", "backend.type", "backend":
		fmt.Fprintln(w, cfg.BackendType)
	case "podfetch-url", "podfetch.url":
		fmt.Fprintln(w, cfg.PodfetchURL)
	case "podfetch-user", "podfetch.user":
		fmt.Fprintln(w, cfg.PodfetchUser)
	case "podfetch-pass", "podfetch.pass":
		fmt.Fprintln(w, cfg.PodfetchPass)
	case "podfetch-api-key", "podfetch.api-key", "podfetch-key", "podfetch-token":
		fmt.Fprintln(w, cfg.PodfetchAPIKey)
	case "podfetch-db-path", "podfetch.db-path", "podfetch-db", "podfetch.db", "podfetch-sqlite-db-path":
		fmt.Fprintln(w, cfg.PodfetchDBPath)
	case "server-base-url", "server.base-url", "server-url", "base-url":
		fmt.Fprintln(w, cfg.ServerBaseURL)
	case "whisper-url", "whisper.url":
		fmt.Fprintln(w, cfg.WhisperURL)
	case "whisper-language", "whisper.language", "lang":
		fmt.Fprintln(w, cfg.WhisperLanguage)
	case "active-profile-id", "llm-id":
		fmt.Fprintln(w, cfg.ActiveProfileID)
	case "active-whisper-id", "whisper-id":
		fmt.Fprintln(w, cfg.ActiveWhisperID)
	case "default-download-policy", "default-download-mode", "download-policy":
		fmt.Fprintln(w, cfg.DefaultDownloadPolicy)
	case "default-download-k", "default-k", "download-k":
		fmt.Fprintln(w, cfg.DefaultDownloadK)
	case "default-ad-policy", "default-ad-removal", "default-ad-mode", "ad-policy", "ad-removal":
		fmt.Fprintln(w, cfg.DefaultAdRemoval)
	case "gemini-api-key-enabled":
		fmt.Fprintln(w, cfg.IsGeminiAPIKeyEnabled())
	case "openrouter-api-key-enabled":
		fmt.Fprintln(w, cfg.IsOpenRouterAPIKeyEnabled())
	case "gemini-api-key":
		fmt.Fprintln(w, config.ResolveGeminiAPIKey(&cfg))
	case "gemini-api-key-file", "gemini-key-file", "api-key-file":
		fmt.Fprintln(w, cfg.GeminiAPIKeyFile)
	case "gemini-model":
		fmt.Fprintln(w, cfg.GetGeminiModel())
	case "speculative-transcription", "speculative-competition":
		fmt.Fprintln(w, cfg.IsSpeculativeTranscriptionEnabled())
	case "competing-services", "speculative-services":
		fmt.Fprintln(w, strings.Join(cfg.GetCompetingServices(), ", "))
	default:
		return fmt.Errorf("unknown configuration key %q; 'pod config show' prints the current settings by name", key)
	}
	return nil
}

func setPodcastsDir(w io.Writer, cfg *Config, dir string) {
	cfg.PodcastsDir = dir
	_ = config.SaveConfig(cfg)
	fmt.Fprintf(w, "Default podcasts directory updated to: '%s'\n", dir)
}

func printConfig(w io.Writer, cfg Config) {
	fmt.Fprintf(w, "Configuration file: '%s'\n", config.ConfigPath())
	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "(not set)"
	}
	fmt.Fprintf(w, "  podcasts_dir:             %s\n", podcastsDir)
	if cfg.ServerBaseURL != "" {
		fmt.Fprintf(w, "  server_base_url:          %s\n", cfg.ServerBaseURL)
	}
	if cfg.DefaultDownloadPolicy != "" {
		fmt.Fprintf(w, "  default_download_policy:  %s\n", cfg.DefaultDownloadPolicy)
	}
	if cfg.DefaultDownloadK > 0 {
		fmt.Fprintf(w, "  default_download_k:       %d\n", cfg.DefaultDownloadK)
	}
	if cfg.DefaultAdRemoval != "" {
		fmt.Fprintf(w, "  default_ad_policy:        %s\n", cfg.DefaultAdRemoval)
	}
	fmt.Fprintf(w, "  whisper_url:              %s\n", cfg.WhisperURL)
	fmt.Fprintf(w, "  whisper_speed_factor:     %.1f\n", cfg.WhisperSpeedFactor)
	if cfg.WhisperDockerContainer != "" {
		fmt.Fprintf(w, "  whisper_docker_container: %s\n", cfg.WhisperDockerContainer)
	}
	if cfg.WhisperWakeCommand != "" {
		fmt.Fprintf(w, "  whisper_wake_command:     %s\n", cfg.WhisperWakeCommand)
	}
	if cfg.WhisperLanguage != "" {
		fmt.Fprintf(w, "  whisper_language:         %s\n", cfg.WhisperLanguage)
	}
	fmt.Fprintf(w, "  active_profile_id:        %d\n", cfg.ActiveProfileID)
	if cfg.ActiveWhisperID > 0 {
		fmt.Fprintf(w, "  active_whisper_id:        %d\n", cfg.ActiveWhisperID)
	}
	if cfg.BackendType != "" {
		fmt.Fprintf(w, "  backend_type:             %s\n", cfg.BackendType)
	}
	if cfg.PodfetchURL != "" {
		fmt.Fprintf(w, "  podfetch_url:             %s\n", cfg.PodfetchURL)
	}
	if cfg.PodfetchUser != "" {
		fmt.Fprintf(w, "  podfetch_user:            %s\n", cfg.PodfetchUser)
	}
	if cfg.PodfetchDBPath != "" {
		fmt.Fprintf(w, "  podfetch_db_path:         %s\n", cfg.PodfetchDBPath)
	}
	if cfg.GeminiAPIKeyFile != "" {
		fmt.Fprintf(w, "  gemini_api_key_file:      %s\n", cfg.GeminiAPIKeyFile)
	}
	fmt.Fprintf(w, "  gemini_api_key_enabled:   %v\n", cfg.IsGeminiAPIKeyEnabled())
	fmt.Fprintf(w, "  openrouter_api_key_enabled: %v\n", cfg.IsOpenRouterAPIKeyEnabled())
	fmt.Fprintf(w, "  speculative_transcription: %v\n", cfg.IsSpeculativeTranscriptionEnabled())
	fmt.Fprintf(w, "  competing_services:       %s\n", strings.Join(cfg.GetCompetingServices(), ", "))
}
