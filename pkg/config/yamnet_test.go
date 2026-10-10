package config

import (
	"pod/pkg/types"
	"testing"
)

func TestYamnetEndpointOverride(t *testing.T) {
	t.Setenv("YAMNET_URL", "http://classifier:8090")
	cfg := types.Config{}
	applyWhisperEnv(&cfg)
	if cfg.YamnetURL != "http://classifier:8090" {
		t.Fatal("YAMNET_URL was ignored")
	}
}
