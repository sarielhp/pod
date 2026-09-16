package gemini

import (
	"time"

	"pod/pkg/port"
)

// PortName identifies Google's own API endpoint as a metered resource.
//
// Ports are named for endpoint and credential, not for model family. Gemini
// reached through OpenRouter draws on OpenRouter's quota and is a different
// port entirely — measured, not assumed: OpenRouter served gemini-2.5-flash
// while this port was refusing every request.
const PortName = "google-ai-studio"

const (
	DefaultRateLimitCooldown  = port.DefaultRateLimitCooldown
	DefaultDailyQuotaCooldown = port.DefaultDailyCooldown
	MinRateLimitCooldown      = port.MinCooldown
)

// StudioPort is the meter shared by everything that calls Google's API
// directly — transcription here, and ad detection in pkg/detect. They speak
// different protocols to the same quota, so they must share one port or each
// will spend requests rediscovering a limit the other already hit.
func StudioPort() *port.Port { return port.Open(PortName) }

// RateLimitCooldownFor is how long to stop calling after a 429.
func RateLimitCooldownFor(body []byte) time.Duration {
	return port.CooldownFor(port.ParseRetryAfter(body))
}

// TripCircuitBreaker closes the port for a while.
func TripCircuitBreaker(reason string, duration time.Duration) {
	StudioPort().Trip(reason, duration)
}

// IsCircuitBreakerOpen reports whether the port is closed, until when, and why.
func IsCircuitBreakerOpen() (bool, time.Time, string) {
	return StudioPort().CooldownOpen()
}

// ResetCircuitBreaker reopens the port.
func ResetCircuitBreaker() { StudioPort().Reset() }

// ExtractGeminiRetryDelay reads a "please retry in 58.8s" hint from an error
// body.
func ExtractGeminiRetryDelay(body []byte) time.Duration { return port.ParseRetryAfter(body) }
