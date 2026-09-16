package gemini

import (
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/config"
)

func TestCircuitBreakerLifeCycle(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestConfigPath(filepath.Join(tempDir, "config.json"))
	defer config.SetTestConfigPath("")

	ResetCircuitBreaker()
	if open, _, _ := IsCircuitBreakerOpen(); open {
		t.Fatal("expected circuit breaker to be closed initially")
	}

	TripCircuitBreaker("Quota exceeded", 2*time.Hour)
	open, until, reason := IsCircuitBreakerOpen()
	if !open {
		t.Fatal("expected circuit breaker to be open after trip")
	}
	if reason != "Quota exceeded" {
		t.Errorf("expected reason %q, got %q", "Quota exceeded", reason)
	}
	if time.Until(until) < 1*time.Hour {
		t.Errorf("expected until > 1h in future, got %v", until)
	}

	// Forget what this process cached, so the next check must consult the file.
	StudioPort().DropCache()

	// Should reload from disk
	open, _, _ = IsCircuitBreakerOpen()
	if !open {
		t.Fatal("expected circuit breaker to be reloaded from disk")
	}

	ResetCircuitBreaker()
	if open, _, _ := IsCircuitBreakerOpen(); open {
		t.Fatal("expected circuit breaker to be closed after reset")
	}
}

func TestCircuitBreakerExpiration(t *testing.T) {
	tempDir := t.TempDir()
	config.SetTestConfigPath(filepath.Join(tempDir, "config.json"))
	defer config.SetTestConfigPath("")

	ResetCircuitBreaker()
	TripCircuitBreaker("Past error", -1*time.Second)

	if open, _, _ := IsCircuitBreakerOpen(); open {
		t.Fatal("expected expired circuit breaker to be closed")
	}
}
