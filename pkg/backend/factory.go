package backend

import (
	"fmt"

	"pod/pkg/progress"
	"pod/pkg/types"
)

func IsStandalone(cfg *types.Config) bool {
	return true
}

func isPodfetchActive(cfg *types.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.PodfetchURL != "" || cfg.PodfetchDBPath != ""
}

func FromAppConfig(cfg *types.Config, rep progress.Reporter) (Backend, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	bCfg := Config{
		PodcastsDir:       cfg.PodcastsDir,
		SubscriptionsFile: cfg.SubscriptionsFile,
		ServerBaseURL:     cfg.ServerBaseURL,
		Progress:          rep,
	}
	return New("standalone", bCfg)
}

// ReaderFromAppConfig returns a PodcastReader from configured backend settings, used for import.
func ReaderFromAppConfig(cfg *types.Config, rep progress.Reporter) (PodcastReader, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if isPodfetchActive(cfg) {
		return New("podfetch", Config{
			Host:              cfg.PodfetchURL,
			User:              cfg.PodfetchUser,
			Pass:              cfg.PodfetchPass,
			Token:             cfg.PodfetchAPIKey,
			APIKey:            cfg.PodfetchAPIKey,
			DBPath:            cfg.PodfetchDBPath,
			PodcastsDir:       cfg.PodcastsDir,
			SubscriptionsFile: cfg.SubscriptionsFile,
			Progress:          rep,
		})
	}
	return FromAppConfig(cfg, rep)
}

// SyncEpisodeDuration is a no-op in standalone mode.
func SyncEpisodeDuration(cfg *types.Config, filePath string, duration float64) error {
	return nil
}
