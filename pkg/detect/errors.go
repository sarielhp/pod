package detect

import (
	"encoding/json"
	"strings"

	"pod/pkg/port"
)

// summariseErrorBody reduces a provider's error body to its message.
//
// A Gemini quota refusal arrives as 39 lines of JSON with documentation links
// and quota metadata, and the one part a person needs — "please retry in
// 58.8s" — is buried in the middle of it. Printing the whole body as an error
// message pushed everything useful off the screen.
func summariseErrorBody(body []byte) string {
	if len(body) == 0 {
		return "(empty response)"
	}
	if msg := errorMessageField(body); msg != "" {
		return collapseWhitespace(msg)
	}
	return truncate(collapseWhitespace(string(body)), 300)
}

// errorMessageField pulls .error.message out of a response, accepting both
// the bare object and the single-element array some gateways wrap it in.
func errorMessageField(body []byte) string {
	type errorEnvelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	var one errorEnvelope
	if err := json.Unmarshal(body, &one); err == nil && one.Error.Message != "" {
		return one.Error.Message
	}
	var many []errorEnvelope
	if err := json.Unmarshal(body, &many); err == nil {
		for _, e := range many {
			if e.Error.Message != "" {
				return e.Error.Message
			}
		}
	}
	return ""
}

// isDailyQuota distinguishes a quota that resets tomorrow from one that
// resets within the minute. Both arrive as 429, and treating a per-minute
// blip as a daily exhaustion would idle the only good free backend for hours.
// isDailyQuota reports whether a refusal is a quota that resets tomorrow
// rather than one that resets within the minute.
//
// The structured quotaId is believed over anything in the prose, because the
// prose is actively misleading: a spent daily quota still says "please retry
// in 23s", which is when the rate bucket refills and not when the day turns.
// Only when no quotaId is present does this fall back to reading the text.
func isDailyQuota(body []byte) bool {
	if daily, known := port.QuotaScope(body); known {
		return daily
	}
	text := strings.ToLower(string(body))
	for _, marker := range []string{"per day", "perday", "daily limit", "requests per day"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
