package config

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/types"
)

func TestReadKeyFile(t *testing.T) {
	t.Parallel()
	if got := readKeyFile(""); got != "" {
		t.Errorf("expected empty string for empty path, got %q", got)
	}
	if got := readKeyFile("/path/to/nonexistent/file/abs_test"); got != "" {
		t.Errorf("expected empty string for missing file, got %q", got)
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key.txt")
	if err := os.WriteFile(keyPath, []byte("  secret-key-123\n\n"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	if got := readKeyFile(keyPath); got != "secret-key-123" {
		t.Errorf("expected 'secret-key-123', got %q", got)
	}

	zeroPath := filepath.Join(tmpDir, "zero.txt")
	if err := os.WriteFile(zeroPath, []byte("00000000"), 0600); err != nil {
		t.Fatalf("failed to write zero key: %v", err)
	}
	if got := readKeyFile(zeroPath); got != "" {
		t.Errorf("expected zeroed key to be ignored, got %q", got)
	}
}

func TestResolveGeminiAPIKeyFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "gemini_key.txt")
	if err := os.WriteFile(keyPath, []byte("gemini-test-key-file"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	enabled := true
	cfg := &types.Config{
		GeminiConfig: types.GeminiConfig{
			GeminiAPIKeyEnabled: &enabled,
			GeminiAPIKeyFile:    keyPath,
		},
	}

	if got := ResolveGeminiAPIKey(cfg); got != "gemini-test-key-file" {
		t.Errorf("expected 'gemini-test-key-file', got %q", got)
	}

	cfg.GeminiAPIKeyFile = filepath.Join(tmpDir, "missing.txt")
	if got := ResolveGeminiAPIKey(cfg); got != "" {
		t.Errorf("expected empty string for missing key file, got %q", got)
	}
}

func TestResolveGeminiAPIKeyPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key_file.txt")
	if err := os.WriteFile(keyPath, []byte("from-file"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	enabled := true
	cfg := &types.Config{
		GeminiConfig: types.GeminiConfig{
			GeminiAPIKeyEnabled: &enabled,
			GeminiAPIKey:        "direct-key",
			GeminiAPIKeyFile:    keyPath,
		},
	}

	if got := ResolveGeminiAPIKey(cfg); got != "direct-key" {
		t.Errorf("expected direct-key, got %q", got)
	}

	cfg.GeminiAPIKey = ""
	t.Setenv("GEMINI_API_KEY", "env-key")
	if got := ResolveGeminiAPIKey(cfg); got != "env-key" {
		t.Errorf("expected env-key, got %q", got)
	}

	t.Setenv("GEMINI_API_KEY", "")
	envKeyPath := filepath.Join(tmpDir, "env_file.txt")
	if err := os.WriteFile(envKeyPath, []byte("from-env-file"), 0600); err != nil {
		t.Fatalf("failed to write env key file: %v", err)
	}
	t.Setenv("GEMINI_API_KEY_FILE", envKeyPath)
	if got := ResolveGeminiAPIKey(cfg); got != "from-env-file" {
		t.Errorf("expected from-env-file, got %q", got)
	}
}

func TestApplyAPIKeyEnvOverridesFile(t *testing.T) {
	cfg := &types.Config{}
	t.Setenv("GEMINI_API_KEY_FILE", "/custom/path/key.txt")
	applyAPIKeyEnvOverrides(cfg)
	if cfg.GeminiAPIKeyFile != "/custom/path/key.txt" {
		t.Errorf("expected /custom/path/key.txt, got %q", cfg.GeminiAPIKeyFile)
	}
}

func TestResolveGeminiAPIKeyNoSearch(t *testing.T) {
	enabled := true
	cfg := &types.Config{
		GeminiConfig: types.GeminiConfig{
			GeminiAPIKeyEnabled: &enabled,
		},
	}
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY_FILE", "")
	if got := ResolveGeminiAPIKey(cfg); got != "" {
		t.Errorf("expected empty string when no key or key file configured, got %q", got)
	}
}

func TestResolveLLMAPIKey(t *testing.T) {
	t.Parallel()
	enabled := true
	disabled := false
	cfg := &types.Config{
		GeminiConfig: types.GeminiConfig{
			GeminiAPIKey:        "gemini-key-123",
			GeminiAPIKeyEnabled: &enabled,
		},
	}

	geminiProfile := types.LLMProfile{
		Type:  "gemini",
		URL:   "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		Model: "gemini-flash-latest",
	}

	if got := ResolveLLMAPIKey(geminiProfile, cfg); got != "gemini-key-123" {
		t.Errorf("expected gemini-key-123, got %q", got)
	}

	cfg.GeminiAPIKeyEnabled = &disabled
	if got := ResolveLLMAPIKey(geminiProfile, cfg); got != "" {
		t.Errorf("expected empty when gemini disabled, got %q", got)
	}
}
