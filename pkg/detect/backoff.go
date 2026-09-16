package detect

import (
	"time"

	"pod/pkg/port"
)

// SetRetryBackoff overrides the wait between LLM request attempts. Pass nil to
// restore the real schedule.
//
// The waiting now happens inside pkg/port, which owns retrying for every
// subsystem sharing an upstream, so this forwards there rather than keeping a
// second schedule that could drift from it.
func SetRetryBackoff(f func(attempt int) time.Duration) {
	if f == nil {
		port.SetDefaultBackoff(nil)
		return
	}
	port.SetDefaultBackoff(func(attempt int, _ bool, _ time.Duration) time.Duration {
		return f(attempt)
	})
}
