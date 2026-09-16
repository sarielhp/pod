package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"pod/pkg/config"
	"pod/pkg/types"
)

// ChainAudit compares the configured model chain against what the key can
// actually reach.
//
// The chain is hand-written, which means it rots in two directions and
// neither announces itself. A model can be withdrawn — Google removed
// gemini-2.5-flash from new users — and a model can appear that would serve
// better or simply add its own daily allowance. gemini-3.6-flash was missing
// from the chain here and turned out to be the one model with quota left on
// the day it mattered.
type ChainAudit struct {
	// Chain is the configured order.
	Chain []string

	// Missing are chain entries the endpoint no longer offers.
	Missing []string

	// Unlisted are comparable models the endpoint offers that the chain does
	// not mention. They are candidates to evaluate, not to adopt blindly:
	// ordering models is a quality judgement that cannot be read off a
	// version number.
	Unlisted []string
}

// Stale reports whether the chain names a model that no longer exists.
func (a ChainAudit) Stale() bool { return len(a.Missing) > 0 }

// AuditModelChain lists the endpoint's models and compares them to the chain.
func AuditModelChain(ctx context.Context, cfg *types.Config) (ChainAudit, error) {
	audit := ChainAudit{Chain: GeminiModelChain(cfg)}

	available, err := ListAvailableModels(ctx, cfg)
	if err != nil {
		return audit, err
	}
	have := make(map[string]bool, len(available))
	for _, m := range available {
		have[m] = true
	}

	inChain := make(map[string]bool, len(audit.Chain))
	for _, m := range audit.Chain {
		inChain[m] = true
		if !have[m] {
			audit.Missing = append(audit.Missing, m)
		}
	}

	for _, m := range available {
		if !inChain[m] && comparableModel(m) {
			audit.Unlisted = append(audit.Unlisted, m)
		}
	}
	sort.Strings(audit.Unlisted)
	return audit, nil
}

// comparableModel reports a model worth weighing against the chain.
//
// Lite and preview variants and the special-purpose ones are left out: they
// are not the same class of model, and listing them as candidates would bury
// the one entry that matters. Aliases are excluded because they name no
// particular model and drift onto whichever is newest — and so most
// rate-limited.
func comparableModel(name string) bool {
	if !strings.Contains(name, "flash") {
		return false
	}
	for _, skip := range []string{"lite", "preview", "latest", "image", "tts", "live", "omni", "thinking"} {
		if strings.Contains(name, skip) {
			return false
		}
	}
	return true
}

// ListAvailableModels names the models this key can call.
func ListAvailableModels(ctx context.Context, cfg *types.Config) ([]string, error) {
	apiKey := config.ResolveGeminiAPIKey(cfg)
	if apiKey == "" {
		return nil, fmt.Errorf("no Gemini API key configured")
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://generativelanguage.googleapis.com/v1beta/models?pageSize=200", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list models: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Models []struct {
			Name    string   `json:"name"`
			Methods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse model list: %w", err)
	}

	var out []string
	for _, m := range payload.Models {
		if !supportsGeneration(m.Methods) {
			continue
		}
		out = append(out, strings.TrimPrefix(m.Name, "models/"))
	}
	sort.Strings(out)
	return out, nil
}

func supportsGeneration(methods []string) bool {
	for _, m := range methods {
		if m == "generateContent" {
			return true
		}
	}
	return false
}
