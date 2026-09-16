package gemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"pod/pkg/types"
	"pod/pkg/util"
)

// DefaultGeminiModelChain is the order in which models are tried.
//
// The first entry is pinned rather than an alias on purpose. "gemini-flash-
// latest" silently follows Google's newest flash model, and the newest model
// carries the smallest free-tier allowance, so the alias drifts onto whatever
// is most rate-limited without anyone changing a line of code. Pinning also
// means a saved transcript names a real model.
//
// The later entries exist because the free tier meters requests per model.
// When the first model's quota is spent the second one's is untouched, which
// turns a hard stop into spare capacity. They also cover the other way a
// pinned model fails: retirement. Google withdrew gemini-2.5-flash from new
// users, and a pinned model will eventually answer 404 the same way.
//
// Newer models are preferred because they timestamp more finely — the
// difference that matters for placing an advertisement cut — not because they
// transcribe words more accurately, where the models are close.
var DefaultGeminiModelChain = []string{
	"gemini-3.8-flash",
	"gemini-3.7-flash",
	"gemini-3.5-flash",
}

// GeminiModelChain is the list of models to try, in order, for this config.
//
// An explicitly configured model leads, because a stated preference should be
// honoured, but it does not stand alone: the point of the chain is that an
// exhausted or retired model is survivable, and that applies just as much to
// one the user named.
func GeminiModelChain(cfg *types.Config) []string {
	var chain []string
	seen := map[string]bool{}
	add := func(m string) {
		if m == "" || seen[m] {
			return
		}
		seen[m] = true
		chain = append(chain, m)
	}
	if cfg != nil {
		add(cfg.GeminiModel)
	}
	for _, m := range DefaultGeminiModelChain {
		add(m)
	}
	if len(chain) == 0 {
		add(defaultGeminiModel)
	}
	return chain
}

// modelSelector hands out the model to use and advances past one that turns
// out to be unavailable.
//
// It is shared by the goroutines transcribing the chunks of a single file so
// that one chunk discovering an exhausted model moves every other chunk along
// with it. Without that, each chunk would spend its own retries rediscovering
// the same 429, burning the very quota the chain is trying to conserve — and
// chunks of one transcript would end up produced by different models.
type modelSelector struct {
	mu     sync.Mutex
	models []string
	idx    int
}

func newModelSelector(models []string) *modelSelector {
	return &modelSelector{models: models}
}

func (s *modelSelector) current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idx >= len(s.models) {
		return ""
	}
	return s.models[s.idx]
}

// isLast reports whether model is the final entry, i.e. whether anything
// remains to fall back to.
func (s *modelSelector) isLast(model string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.models) > 0 && s.models[len(s.models)-1] == model
}

// advancePast moves to the next model, but only if failed is still the
// current one. Several chunks failing on the same model must cost one step,
// not one step each, or a single exhausted model would skip the whole chain.
// It reports the next model and whether one remains.
func (s *modelSelector) advancePast(failed string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idx < len(s.models) && s.models[s.idx] == failed {
		s.idx++
	}
	if s.idx >= len(s.models) {
		return "", false
	}
	return s.models[s.idx], true
}

func announceModelSwitch(from, to string, reason string) {
	fmt.Println(util.BoldYellow(fmt.Sprintf("   Gemini: %s is unavailable (%s); trying %s", from, reason, to)))
}

// callStudioAcrossModels transcribes one chunk, walking the model chain when a
// model reports itself unavailable.
//
// Only that specific condition advances the chain. A malformed response, a
// blocked candidate or a network failure says nothing about the model's
// availability, so those are returned as they are rather than spending the
// remaining models on a request that will fail the same way each time.
func callStudioAcrossModels(ctx context.Context, apiKey, fileURI string, sel *modelSelector) (*types.GeminiResponsePayload, error) {
	var lastUnavailable *ModelUnavailableError

	for {
		model := sel.current()
		if model == "" {
			break
		}

		payload, err := callGeminiStudioModel(ctx, apiKey, model, fileURI, sel.isLast(model))
		if err == nil {
			return payload, nil
		}

		var unavailable *ModelUnavailableError
		if !errors.As(err, &unavailable) {
			return nil, err
		}
		lastUnavailable = unavailable

		reason := "quota exhausted"
		switch {
		case unavailable.Status == http.StatusNotFound:
			reason = "retired"
		case unavailable.Daily:
			reason = "daily quota exhausted"
		}
		next, ok := sel.advancePast(model)
		if !ok {
			break
		}
		if next != model {
			announceModelSwitch(model, next, reason)
		}
	}

	if lastUnavailable == nil {
		return nil, fmt.Errorf("no gemini model available")
	}
	// Every model is spent, so this is the point at which it is worth
	// stopping Gemini calls altogether — not the moment the first model ran
	// out, which is what the circuit breaker used to react to.
	cooldown := RateLimitCooldownFor(lastUnavailable.Body)
	if lastUnavailable.Daily {
		cooldown = DefaultDailyQuotaCooldown
	}
	if lastUnavailable.Status != http.StatusNotFound {
		TripCircuitBreaker(FormatGeminiErrorBody(lastUnavailable.Body), cooldown)
	}
	return nil, lastUnavailable
}
