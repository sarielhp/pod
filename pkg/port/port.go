// Package port mediates access to a metered upstream.
//
// A port is a resource with a meter, not a client. Several subsystems may
// speak quite different protocols to the same endpoint — pod uploads audio to
// Gemini's native API to transcribe, and posts OpenAI-compatible chat to the
// same host to detect advertisements — but they draw on one quota. Before
// this package each kept its own retry loop and its own idea of when to give
// up, and neither could see that the other had just been rate-limited: a
// transcription run would exhaust the per-minute budget and ad detection
// would then spend three more requests rediscovering it.
//
// What crosses the boundary is a verdict, never a payload. A caller performs
// its own request, parses its own response, and reports only what happened;
// the port owns the retry loop, the model chain and the cooldown. That keeps
// the two protocols separate — collapsing them would mean abstracting over
// the parts that genuinely differ — while making it impossible for their
// retry policies to drift apart again.
package port

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Verdict is what a caller made of one request.
type Verdict int

const (
	// Success means the caller got what it asked for. It has already kept
	// the payload; the port only needs to stop.
	Success Verdict = iota

	// RateLimited means the quota is spent. Because quota is metered per
	// model, another model on the same port may still answer.
	RateLimited

	// Overloaded means the upstream is momentarily short of capacity. No
	// restraint on our side fixes it and it usually clears within a minute,
	// so it is worth waiting out rather than abandoning.
	Overloaded

	// ModelGone means this model will never answer again — retired, or
	// withdrawn from new users. Retrying is pointless; the next model is not.
	ModelGone

	// Fatal means the request itself was wrong. No model will do better.
	Fatal

	// Unusable means the model answered, but not with something the caller
	// can use — the wrong shape, a refusal, a truncated reply. That is a
	// property of the model rather than of the request, so another model is
	// worth trying, and there is no reason to rest this one or close the port.
	Unusable
)

// Attempt is a caller's report on one request.
type Attempt struct {
	Verdict Verdict

	// RetryAfter is how long the upstream asked us to wait, when it said.
	// Believing it beats guessing: Gemini's per-minute quota error names
	// about 59 seconds, and treating that as an hour-long outage costs far
	// more than the one retry it saves.
	RetryAfter time.Duration

	// Daily distinguishes a quota that resets tomorrow from one that resets
	// within the minute. Both arrive as the same status code.
	Daily bool

	// PerModel says the exhausted quota belongs to this model alone, so the
	// port must stay open for the others.
	PerModel bool

	// Err is what to report if no model on this port succeeds.
	Err error
}

// OK reports a successful request.
func OK() Attempt { return Attempt{Verdict: Success} }

// Fail reports a request that no other model would survive either.
func Fail(err error) Attempt { return Attempt{Verdict: Fatal, Err: err} }

const (
	// A rate limit and a capacity shortage deserve different patience. Asking
	// harder makes a rate limit worse, while an overloaded model is simply
	// busy, so the two get different budgets.
	maxRateLimitAttempts = 3
	maxOverloadAttempts  = 6
)

// Port is a metered upstream. Ports are identified by endpoint and
// credential rather than by model family: the same model reached through a
// different provider draws on a different quota, so Gemini via OpenRouter and
// Gemini direct are two ports, not one.
type Port struct {
	name string

	// dir holds the cooldown file. Empty means the user's config directory,
	// which is what a real run wants; tests point it at a temporary directory
	// so they neither inherit a previous run's state nor leave any behind.
	dir string

	mu     sync.Mutex
	until  time.Time
	reason string

	// Notify reports a model switch. Nil stays silent, which is what a
	// library should do unless a caller asks otherwise.
	Notify func(from, to, reason string)

	// Backoff overrides the wait between attempts. Tests set a zero backoff:
	// without it, exercising the retry path means really sleeping through it,
	// and a suite that takes seconds to prove a branch stops being run.
	Backoff func(attempt int, overloaded bool, requested time.Duration) time.Duration
}

var (
	registryMu sync.Mutex
	registry   = map[string]*Port{}
)

// Open returns the port with this name, creating it once. Callers share the
// returned port, which is what lets one subsystem's rate limit be visible to
// another.
func Open(name string) *Port {
	registryMu.Lock()
	defer registryMu.Unlock()
	if p, ok := registry[name]; ok {
		return p
	}
	p := &Port{name: name}
	registry[name] = p
	return p
}

// Name is the port's identifier, used for its cooldown file.
func (p *Port) Name() string { return p.name }

// CooldownError reports that the port is closed and for how long.
type CooldownError struct {
	Port   string
	Until  time.Time
	Reason string
}

func (e *CooldownError) Error() string {
	return fmt.Sprintf("%s in cooldown until %s: %s", e.Port, e.Until.Format("15:04:05"), e.Reason)
}

// Call runs fn against each model in turn until one succeeds or the chain is
// spent, retrying within a model where that is worth doing.
//
// models is the caller's preference order. An empty slice means the upstream
// chooses, and fn is called once with an empty model name.
func (p *Port) Call(ctx context.Context, models []string, fn func(context.Context, string) Attempt) error {
	if open, until, reason := p.CooldownOpen(); open {
		return &CooldownError{Port: p.name, Until: until, Reason: reason}
	}
	if len(models) == 0 {
		models = []string{""}
	}
	// Skip models already known to be spent. Quota is metered per model and
	// several callers work one port at once, so without this each of them
	// pays a request to rediscover what the first already established — on a
	// three-model chain that tripled consumption of the very quota the chain
	// exists to conserve.
	live := p.liveModels(models)
	if len(live) == 0 {
		return p.restingError(models)
	}

	// Closing the port is only right when every model refused on quota. A
	// model that answered with something unusable has quota left and will
	// probably answer properly next time, so shutting the port over it would
	// idle a working model for hours.
	allQuota := true

	var last Attempt
	for i, model := range live {
		isLast := i == len(live)-1
		attempt, err := p.callModel(ctx, model, isLast, fn)
		if err != nil {
			return err
		}
		if attempt.Verdict == Success {
			return nil
		}
		last = attempt
		if attempt.Verdict == Fatal {
			return attempt.Err
		}
		if attempt.Verdict == RateLimited {
			p.RestModel(model, restFor(attempt))
			if attempt.PerModel {
				// This model is finished; the endpoint is not.
				allQuota = false
			}
		} else {
			allQuota = false
		}
		if !isLast && p.Notify != nil {
			p.Notify(model, live[i+1], verdictReason(attempt))
		}
	}

	if allQuota {
		p.tripFor(last)
	}
	return last.Err
}

// restFor is how long to leave a model alone after it refuses on quota.
//
// A model whose daily allowance is gone must be rested for the day, not for
// the minute its error message suggests. Google's daily refusal still says
// "retry in 23s" — that is the rate bucket refilling — so honouring the hint
// would spend a request every half minute on a model that cannot answer again
// until tomorrow.
func restFor(a Attempt) time.Duration {
	if a.Daily {
		return DefaultDailyCooldown
	}
	return CooldownFor(a.RetryAfter)
}

// liveModels drops the models currently resting.
func (p *Port) liveModels(models []string) []string {
	live := make([]string, 0, len(models))
	for _, m := range models {
		if !p.ModelResting(m) {
			live = append(live, m)
		}
	}
	return live
}

// restingError reports that every model is spent without issuing a request.
func (p *Port) restingError(models []string) error {
	return fmt.Errorf("%s: every model is out of quota (%s)", p.name, strings.Join(models, ", "))
}

// callModel runs fn against one model, retrying while that is worthwhile.
//
// A rate limit is only worth waiting out on the last model: while another
// model remains, the minute the upstream asks for is a minute spent not
// working, when a model with untouched quota would answer at once.
func (p *Port) callModel(ctx context.Context, model string, isLast bool, fn func(context.Context, string) Attempt) (Attempt, error) {
	overloaded := false
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return Attempt{}, err
		}

		got := fn(ctx, model)
		switch got.Verdict {
		case Success, Fatal, ModelGone, Unusable:
			return got, nil
		case RateLimited:
			if !isLast || attempt >= maxRateLimitAttempts {
				return got, nil
			}
		case Overloaded:
			overloaded = true
			if attempt >= maxOverloadAttempts {
				return got, nil
			}
		}

		if err := sleep(ctx, p.delay(attempt, overloaded, got.RetryAfter)); err != nil {
			return Attempt{}, err
		}
	}
}

var (
	defaultBackoffMu sync.Mutex
	defaultBackoff   func(attempt int, overloaded bool, requested time.Duration) time.Duration
)

// SetDefaultBackoff overrides the wait between attempts for every port that
// has not set its own. Pass nil to restore the real schedule.
//
// Tests set a zero backoff. Without it, exercising a retry path means really
// sleeping through it, and a suite slow enough to skip is a suite that stops
// catching things.
func SetDefaultBackoff(f func(attempt int, overloaded bool, requested time.Duration) time.Duration) {
	defaultBackoffMu.Lock()
	defaultBackoff = f
	defaultBackoffMu.Unlock()
}

func (p *Port) delay(attempt int, overloaded bool, requested time.Duration) time.Duration {
	if p.Backoff != nil {
		return p.Backoff(attempt, overloaded, requested)
	}
	defaultBackoffMu.Lock()
	f := defaultBackoff
	defaultBackoffMu.Unlock()
	if f != nil {
		return f(attempt, overloaded, requested)
	}
	return retryDelay(attempt, overloaded, requested)
}

// retryDelay is how long to wait before trying again. A delay the upstream
// named beats any schedule of ours.
func retryDelay(attempt int, overloaded bool, requested time.Duration) time.Duration {
	if requested > 0 && requested <= time.Minute {
		return requested
	}
	if overloaded {
		// 2s, 4s, 8s, 16s, 32s: about a minute in total, which is the
		// timescale a "demand spike" message refers to.
		return time.Duration(1<<attempt) * time.Second
	}
	return time.Duration(attempt*2) * time.Second
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func verdictReason(a Attempt) string {
	switch a.Verdict {
	case ModelGone:
		return "retired"
	case Unusable:
		return "unusable output"
	case Overloaded:
		return "overloaded"
	case RateLimited:
		if a.Daily {
			return "daily quota exhausted"
		}
		return "quota exhausted"
	default:
		return "unavailable"
	}
}

// canonicalPorts maps a known endpoint host to a stable port name.
//
// The names must agree across subsystems, because agreeing is the point: pod
// reaches Google's API with two different protocols, and only a shared name
// makes them share a meter.
var canonicalPorts = map[string]string{
	"generativelanguage.googleapis.com": "google-ai-studio",
	"openrouter.ai":                     "openrouter",
	"api.openai.com":                    "openai",
	"api.anthropic.com":                 "anthropic",
}

// ForEndpoint returns the port metering this URL.
//
// Keying on the host rather than on the model is what keeps Gemini-via-
// OpenRouter separate from Gemini-direct. They serve overlapping models and
// bill against entirely different quotas: when this port was refusing every
// request, the same model answered immediately through OpenRouter.
func ForEndpoint(rawURL string) *Port {
	return Open(EndpointName(rawURL))
}

// EndpointName is the port name for a URL.
func EndpointName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	host := strings.ToLower(u.Hostname())
	if name, ok := canonicalPorts[host]; ok {
		return name
	}
	// A local model server has no quota worth metering, but naming it
	// consistently still keeps its failures out of everyone else's cooldown.
	return strings.ReplaceAll(host, ".", "-")
}
