package gemini

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"pod/pkg/config"
	"pod/pkg/util"
)

const (
	DefaultRateLimitCooldown  = 1 * time.Hour
	DefaultDailyQuotaCooldown = 6 * time.Hour
	cooldownFileName          = ".gemini_cooldown.json"
)

type CooldownState struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`
}

var (
	breakerMu    sync.RWMutex
	cachedUntil  time.Time
	cachedReason string
)

func cooldownFilePath() string {
	return filepath.Join(config.ConfigDir(), cooldownFileName)
}

// MinRateLimitCooldown is the shortest lockout worth taking. Below this the
// breaker costs more in repeated failures than it saves.
const MinRateLimitCooldown = 90 * time.Second

// RateLimitCooldownFor is how long to stop calling Gemini after a 429.
//
// A per-minute quota and a per-day quota both arrive as 429, and they deserve
// very different responses. When the server says how long to wait — "please
// retry in 58.8s" — believe it: locking the only good free transcription
// backend out for an hour over a one-minute quota blip is a far worse outcome
// than one extra failed request. The default stands when the server says
// nothing, and a reply asking for longer than the default is not shortened.
func RateLimitCooldownFor(body []byte) time.Duration {
	requested := ExtractGeminiRetryDelay(body)
	if requested <= 0 || requested >= DefaultRateLimitCooldown {
		return DefaultRateLimitCooldown
	}
	// ExtractGeminiRetryDelay already pads past the moment the server named,
	// so the only adjustment left is the floor.
	if requested < MinRateLimitCooldown {
		return MinRateLimitCooldown
	}
	return requested
}

func TripCircuitBreaker(reason string, duration time.Duration) {
	breakerMu.Lock()
	defer breakerMu.Unlock()

	until := time.Now().Add(duration)
	cachedUntil = until
	cachedReason = reason

	state := CooldownState{
		Until:  until,
		Reason: reason,
	}
	if data, err := json.Marshal(state); err == nil {
		_ = util.WriteFileAtomic(cooldownFilePath(), data, 0644)
	}
}

func IsCircuitBreakerOpen() (bool, time.Time, string) {
	breakerMu.RLock()
	if time.Now().Before(cachedUntil) {
		until, reason := cachedUntil, cachedReason
		breakerMu.RUnlock()
		return true, until, reason
	}
	breakerMu.RUnlock()

	path := cooldownFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return false, time.Time{}, ""
	}

	var state CooldownState
	if err := json.Unmarshal(data, &state); err != nil {
		_ = os.Remove(path)
		return false, time.Time{}, ""
	}

	if time.Now().Before(state.Until) {
		breakerMu.Lock()
		cachedUntil = state.Until
		cachedReason = state.Reason
		breakerMu.Unlock()
		return true, state.Until, state.Reason
	}

	_ = os.Remove(path)
	return false, time.Time{}, ""
}

func ResetCircuitBreaker() {
	breakerMu.Lock()
	cachedUntil = time.Time{}
	cachedReason = ""
	breakerMu.Unlock()
	_ = os.Remove(cooldownFilePath())
}
