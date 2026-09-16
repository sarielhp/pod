package port

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pod/pkg/config"
	"pod/pkg/util"
)

const (
	// DefaultRateLimitCooldown applies when the upstream refuses on quota and
	// says nothing about when to come back.
	DefaultRateLimitCooldown = 1 * time.Hour

	// DefaultDailyCooldown applies when the quota resets tomorrow rather than
	// within the minute.
	DefaultDailyCooldown = 6 * time.Hour

	// MinCooldown is the shortest lockout worth taking. Below this the port
	// costs more in repeated failures than it saves.
	MinCooldown = 90 * time.Second
)

// cooldownState is what is written to disk.
//
// The meter is shared by every pod process on this machine, not just this
// one, so the cooldown has to outlive the process that discovered it.
type cooldownState struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`

	// Models records which individual models are spent and until when. The
	// quota is metered per model, so this is the level the knowledge actually
	// lives at — and it has to persist, because each pod invocation is a new
	// process while one recording's chunks are transcribed across several.
	// Without it every chunk rediscovers the same exhausted model, and pays a
	// request to learn what the previous chunk already knew.
	Models map[string]time.Time `json:"models,omitempty"`
}

func (p *Port) cooldownPath() string {
	dir := p.dir
	if dir == "" {
		dir = config.ConfigDir()
	}
	return filepath.Join(dir, ".cooldown-"+p.name+".json")
}

// CooldownOpen reports whether this port is closed, until when, and why.
func (p *Port) CooldownOpen() (bool, time.Time, string) {
	p.mu.Lock()
	if time.Now().Before(p.until) {
		until, reason := p.until, p.reason
		p.mu.Unlock()
		return true, until, reason
	}
	p.mu.Unlock()

	p.mu.Lock()
	state := p.readState()
	p.mu.Unlock()

	// An expired port cooldown must not delete the file: it also carries the
	// per-model rests, which outlive it and are the thing that stops every
	// caller rediscovering an exhausted model. Expired entries are pruned
	// when the state is next written, not when it is read.
	if !time.Now().Before(state.Until) {
		return false, time.Time{}, ""
	}

	p.mu.Lock()
	p.until, p.reason = state.Until, state.Reason
	p.mu.Unlock()
	return true, state.Until, state.Reason
}

// readState returns what is on disk, or an empty state.
func (p *Port) readState() cooldownState {
	var state cooldownState
	data, err := os.ReadFile(p.cooldownPath())
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return cooldownState{}
	}
	return state
}

func (p *Port) writeState(state cooldownState) {
	// Prune rests that have lapsed, so the file does not accumulate an entry
	// for every model ever tried.
	now := time.Now()
	for model, until := range state.Models {
		if !now.Before(until) {
			delete(state.Models, model)
		}
	}
	if data, err := json.Marshal(state); err == nil {
		_ = util.WriteFileAtomic(p.cooldownPath(), data, 0644)
	}
}

// ModelResting reports whether this model is known to be out of quota.
func (p *Port) ModelResting(model string) bool {
	if model == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	until, ok := p.readState().Models[model]
	return ok && time.Now().Before(until)
}

// RestModel records that a model is out of quota, so the next caller skips it
// rather than spending a request to learn the same thing.
func (p *Port) RestModel(model string, d time.Duration) {
	if model == "" || d <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.readState()
	if state.Models == nil {
		state.Models = map[string]time.Time{}
	}
	until := time.Now().Add(d)
	// Never shorten a rest another caller already recorded.
	if existing, ok := state.Models[model]; ok && existing.After(until) {
		return
	}
	state.Models[model] = until
	p.writeState(state)
}

// Trip closes the port for a while.
func (p *Port) Trip(reason string, d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.until = time.Now().Add(d)
	p.reason = reason
	state := p.readState()
	state.Until, state.Reason = p.until, reason
	p.writeState(state)
}

// DropCache forgets what this process last read, so the next check consults
// the file again. The meter is shared with every other pod process on the
// machine, so an in-process cache can be stale: another run may have tripped
// the port since.
func (p *Port) DropCache() {
	p.mu.Lock()
	p.until = time.Time{}
	p.reason = ""
	p.mu.Unlock()
}

// Reset reopens the port.
func (p *Port) Reset() {
	p.mu.Lock()
	p.until = time.Time{}
	p.reason = ""
	p.mu.Unlock()
	_ = os.Remove(p.cooldownPath())
}

// tripFor closes the port once every model has been tried.
//
// This is deliberately not done when the first model runs out: on a per-model
// meter, one exhausted model is spare capacity elsewhere, and closing the
// port then would throw away the models that still work. A retired model
// never closes the port at all — it says nothing about the quota.
func (p *Port) tripFor(a Attempt) {
	if a.Verdict != RateLimited {
		return
	}
	d := CooldownFor(a.RetryAfter)
	if a.Daily {
		d = DefaultDailyCooldown
	}
	reason := "quota exhausted"
	if a.Err != nil {
		reason = a.Err.Error()
	}
	p.Trip(reason, d)
}

// CooldownFor is how long to stay away after a rate limit.
//
// A per-minute quota and a per-day quota arrive identically and deserve very
// different responses. When the upstream says how long to wait, believe it:
// losing the only good free backend for an hour over a one-minute limit costs
// far more than the retry it saves. A request for longer than the default is
// not shortened.
func CooldownFor(requested time.Duration) time.Duration {
	if requested <= 0 || requested >= DefaultRateLimitCooldown {
		return DefaultRateLimitCooldown
	}
	if requested < MinCooldown {
		return MinCooldown
	}
	return requested
}

var retryAfterRegex = regexp.MustCompile(`(?i)(?:please retry in|retry in|retry after)\s+([0-9.]+)\s*(s(?:ec(?:ond)?)?|m(?:in(?:ute)?)?)?`)

// ParseRetryAfter reads a "please retry in 58.8s" hint out of an error body.
// The returned duration is padded by a second so a retry does not land on the
// boundary and fail again.
func ParseRetryAfter(body []byte) time.Duration {
	matches := retryAfterRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(string(matches[1]), 64)
	if err != nil || val <= 0 {
		return 0
	}
	unit := ""
	if len(matches) >= 3 {
		unit = strings.ToLower(string(matches[2]))
	}
	if strings.HasPrefix(unit, "m") {
		return time.Duration(val*60*float64(time.Second)) + time.Second
	}
	return time.Duration((val + 1.0) * float64(time.Second))
}

// QuotaScope reports whether a refusal names a per-day quota, and whether it
// said anything definite at all.
//
// Google's 429 carries a QuotaFailure detail whose quotaId names the quota
// actually violated — "GenerateRequestsPerDayPerProjectPerModel" or the
// PerMinute equivalent. That is worth far more than the prose around it: the
// message accompanying a spent daily quota still says "retry in 23s", because
// that is when the rate bucket refills, not when the day resets. Reading the
// sentence instead of the field turns a day-long exhaustion into a retry
// loop that can never succeed.
func QuotaScope(body []byte) (daily bool, known bool) {
	daily, _, known = QuotaDetail(body)
	return daily, known
}

// QuotaDetail additionally reports whether the exhausted quota belongs to the
// model rather than to the whole endpoint.
//
// It matters because a per-model allowance says nothing about the other
// models: "GenerateRequestsPerDayPerProjectPerModel" means this model is
// finished for the day while its neighbours are untouched. Closing the whole
// endpoint over it would idle every model that still had quota — including
// ones outside the configured chain.
func QuotaDetail(body []byte) (daily, perModel, known bool) {
	for _, m := range quotaIDRegex.FindAllSubmatch(body, -1) {
		id := strings.ToLower(string(m[1]))
		if !strings.Contains(id, "perday") && !strings.Contains(id, "perminute") {
			continue
		}
		known = true
		if strings.Contains(id, "permodel") {
			perModel = true
		}
		if strings.Contains(id, "perday") {
			daily = true
			return daily, perModel, known
		}
	}
	return daily, perModel, known
}

var quotaIDRegex = regexp.MustCompile(`(?i)"quotaId"\s*:\s*"([^"]+)"`)
