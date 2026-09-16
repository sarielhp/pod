package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"pod/pkg/types"
	"pod/pkg/util"
)

var defaultKeyEnabled = true
var defaultKeyDisabled = false

func DefaultConfig() types.Config {
	return types.Config{
		Instructions:     "Configuration file for pod. Select profiles by ID or set active_profile_id.",
		ChunkDurationSec: 0,
		ActiveProfileID:  3,
		Profiles:         DefaultLLMProfiles,

		WhisperConfig: types.WhisperConfig{
			WhisperURL:         "http://127.0.0.1:8088/inference",
			WhisperSpeedFactor: 70.0,
			ActiveWhisperID:    1,
			WhisperProfiles:    DefaultWhisperProfiles,
			WhisperEngine:      types.WhisperEngineLocal,
			WhisperModel:       "tiny.en",
			WhisperCliBinary:   "whisper-cli",
			WhisperProcessors:  4,
			WhisperThreads:     4,
			WhisperGreedy:      true,
		},
		PolicyConfig: types.PolicyConfig{
			DefaultDownloadPolicy: "latest",
			DefaultDownloadK:      3,
			DefaultAdRemoval:      "all",
		},
		GeminiConfig: types.GeminiConfig{
			GeminiProjectID:         "",
			GeminiStagingBucket:     "",
			GeminiLocation:          "us-central1",
			GeminiAPIKeyEnabled:     &defaultKeyEnabled,
			OpenRouterAPIKeyEnabled: &defaultKeyEnabled,
		},
		SpeculativeConfig: types.SpeculativeConfig{
			SpeculativeTranscription: &defaultKeyDisabled,
			CompetingServices:        []string{"gemini", "whisper"},
		},
		BackendConfig: types.BackendConfig{
			BackendType: "standalone",
		},
	}
}

func EnsureConfigExists() (*types.Config, error) {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}
	path := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := DefaultConfig()
		ip := localIP()
		cfg.WhisperURL = fmt.Sprintf("http://%s:8088/inference", ip)
		for i := range cfg.Profiles {
			cfg.Profiles[i].URL = replaceIP(cfg.Profiles[i].URL, ip)
		}
		for i := range cfg.WhisperProfiles {
			if cfg.WhisperProfiles[i].Engine == types.WhisperEngineDocker {
				cfg.WhisperProfiles[i].URL = replaceIP(cfg.WhisperProfiles[i].URL, ip)
			}
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := util.WriteFileAtomic(path, append(data, '\n'), 0600); err != nil {
			return nil, fmt.Errorf("failed to write default config to %s: %w", path, err)
		}
		return &cfg, nil
	}
	return LoadConfig()
}

func LoadConfig() (*types.Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg types.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config json in %s: %w", path, err)
	}
	applyEnvOverrides(&cfg)
	resolveAuthFolderCredentials(&cfg)
	sanitizeDisabledAPIKeys(&cfg)
	return &cfg, nil
}

func SaveConfig(cfg *types.Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	return util.WriteFileAtomic(ConfigPath(), append(data, '\n'), 0600)
}

func applyEnvOverrides(cfg *types.Config) {
	if cfg == nil {
		return
	}
	applyBackendEnv(cfg)
	applyWhisperEnv(cfg)
	applyRemoteEnv(cfg)
	applyAPIKeyEnvOverrides(cfg)
}

func applyBackendEnv(cfg *types.Config) {
	if v := os.Getenv("WHISPER_URL"); v != "" {
		cfg.WhisperURL = v
	}
	if v := os.Getenv("BACKEND_TYPE"); v != "" {
		cfg.BackendType = v
	}
	if v := os.Getenv("PODFETCH_URL"); v != "" {
		cfg.PodfetchURL = v
	}
	if v := os.Getenv("PODFETCH_USER"); v != "" {
		cfg.PodfetchUser = v
	}
	if v := os.Getenv("PODFETCH_PASS"); v != "" {
		cfg.PodfetchPass = v
	}
	if v := os.Getenv("PODFETCH_API_KEY"); v != "" {
		cfg.PodfetchAPIKey = v
	}
	if v := os.Getenv("PODFETCH_DB_PATH"); v != "" {
		cfg.PodfetchDBPath = v
	}
	if v := os.Getenv("PODCASTS_DIR"); v != "" {
		cfg.PodcastsDir = v
	}
	if v := os.Getenv("SERVER_BASE_URL"); v != "" {
		cfg.ServerBaseURL = v
	}
	if v := os.Getenv("SUBSCRIPTIONS_FILE"); v != "" {
		cfg.SubscriptionsFile = v
	}
}

func applyWhisperEnv(cfg *types.Config) {
	if v := os.Getenv("WHISPER_LANGUAGE"); v != "" {
		cfg.WhisperLanguage = v
	}
	if v := os.Getenv("WHISPER_DOCKER_CONTAINER"); v != "" {
		cfg.WhisperDockerContainer = v
	}
	if v := os.Getenv("WHISPER_WAKE_COMMAND"); v != "" {
		cfg.WhisperWakeCommand = v
	}
}

func applyRemoteEnv(cfg *types.Config) {
	if v := os.Getenv("DEFAULT_DOWNLOAD_POLICY"); v != "" {
		cfg.DefaultDownloadPolicy = NormalizeDownloadPolicy(v)
	}
	if v := os.Getenv("DEFAULT_DOWNLOAD_K"); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k > 0 {
			cfg.DefaultDownloadK = k
		}
	}
	if v := os.Getenv("DEFAULT_AD_REMOVAL"); v != "" {
		cfg.DefaultAdRemoval = NormalizeAdRemovalMode(v)
	}
}

func SubscriptionsFilePath(cfg *types.Config) string {
	if cfg != nil && cfg.SubscriptionsFile != "" {
		return cfg.SubscriptionsFile
	}
	return filepath.Join(ConfigDir(), "podcasts.json")
}
