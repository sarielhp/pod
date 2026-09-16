package port

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newTestPort avoids the shared registry so parallel tests cannot collide on
// one another's cooldown state.
// newTestPort isolates a port from the user's config directory, so a test
// neither inherits state from a previous run nor leaves any behind, and from
// the shared registry, so parallel tests cannot collide.
func newTestPort(t *testing.T, name string) *Port {
	t.Helper()
	return &Port{
		name:    name,
		dir:     t.TempDir(),
		Backoff: func(int, bool, time.Duration) time.Duration { return 0 },
	}
}

func TestCallReturnsOnFirstSuccess(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t1")
	var tried []string
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, m string) Attempt {
		tried = append(tried, m)
		return OK()
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(tried) != 1 || tried[0] != "a" {
		t.Errorf("tried %v, want only a", tried)
	}
}

func TestRateLimitedModelIsAbandonedWhileAnotherRemains(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t2")
	var tried []string
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, m string) Attempt {
		tried = append(tried, m)
		if m == "a" {
			// A minute-long wait must not be taken when another model is
			// free: that is the whole point of a per-model meter.
			return Attempt{Verdict: RateLimited, RetryAfter: time.Minute, Err: errors.New("429")}
		}
		return OK()
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(tried) != 2 || tried[0] != "a" || tried[1] != "b" {
		t.Errorf("tried %v, want a then b", tried)
	}
}

func TestRetiredModelFallsThroughWithoutRetrying(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t3")
	calls := map[string]int{}
	err := p.Call(context.Background(), []string{"old", "new"}, func(_ context.Context, m string) Attempt {
		calls[m]++
		if m == "old" {
			return Attempt{Verdict: ModelGone, Err: errors.New("404")}
		}
		return OK()
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls["old"] != 1 {
		t.Errorf("retried a retired model %d times", calls["old"])
	}
}

func TestFatalStopsTheChain(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t4")
	boom := errors.New("bad request")
	calls := 0
	err := p.Call(context.Background(), []string{"a", "b", "c"}, func(_ context.Context, _ string) Attempt {
		calls++
		return Fail(boom)
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	// No other model would survive a malformed request, so spending the
	// chain on it wastes quota.
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestOverloadIsRetriedOnTheSameModel(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t5")
	calls := 0
	err := p.Call(context.Background(), []string{"a"}, func(_ context.Context, _ string) Attempt {
		calls++
		if calls < 3 {
			return Attempt{Verdict: Overloaded, Err: errors.New("503")}
		}
		return OK()
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestExhaustingTheChainClosesThePort(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t6")
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, _ string) Attempt {
		return Attempt{Verdict: RateLimited, RetryAfter: 5 * time.Minute, Err: errors.New("429")}
	})
	if err == nil {
		t.Fatal("expected an error once every model was spent")
	}
	open, _, _ := p.CooldownOpen()
	if !open {
		t.Error("port stayed open after every model was rate-limited")
	}

	// And while closed it refuses without spending a request.
	calls := 0
	err = p.Call(context.Background(), []string{"a"}, func(_ context.Context, _ string) Attempt {
		calls++
		return OK()
	})
	var cooldown *CooldownError
	if !errors.As(err, &cooldown) {
		t.Errorf("err = %v, want CooldownError", err)
	}
	if calls != 0 {
		t.Error("a closed port still issued a request")
	}
}

func TestOneExhaustedModelDoesNotCloseThePort(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t7")
	// On a per-model meter, an exhausted model is spare capacity elsewhere.
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, m string) Attempt {
		if m == "a" {
			return Attempt{Verdict: RateLimited, RetryAfter: time.Hour, Err: errors.New("429")}
		}
		return OK()
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if open, _, _ := p.CooldownOpen(); open {
		t.Error("port closed even though a model still answered")
	}
}

func TestRetiredModelsDoNotCloseThePort(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t8")
	err := p.Call(context.Background(), []string{"a"}, func(_ context.Context, _ string) Attempt {
		return Attempt{Verdict: ModelGone, Err: errors.New("404")}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	// Retirement says nothing about the quota.
	if open, _, _ := p.CooldownOpen(); open {
		t.Error("a retired model closed the whole port")
	}
}

func TestCancelledContextStopsImmediately(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t9")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Call(ctx, []string{"a"}, func(_ context.Context, _ string) Attempt { return OK() })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRetryDelay(t *testing.T) {
	t.Parallel()
	// A delay the upstream named wins over any schedule of ours.
	if got := retryDelay(1, true, 20*time.Second); got != 20*time.Second {
		t.Errorf("requested delay ignored: %v", got)
	}
	// ...but not an implausible one, which would stall the run.
	if got := retryDelay(1, false, 2*time.Hour); got != 2*time.Second {
		t.Errorf("absurd delay honoured: %v", got)
	}
	if got := retryDelay(3, true, 0); got != 8*time.Second {
		t.Errorf("overload backoff = %v, want 8s", got)
	}
	if got := retryDelay(3, false, 0); got != 6*time.Second {
		t.Errorf("plain backoff = %v, want 6s", got)
	}
}

func TestCooldownFor(t *testing.T) {
	t.Parallel()
	if got := CooldownFor(0); got != DefaultRateLimitCooldown {
		t.Errorf("no hint = %v", got)
	}
	// The case that mattered: a one-minute quota blip must not cost an hour.
	if got := CooldownFor(59 * time.Second); got != MinCooldown {
		t.Errorf("short hint = %v, want %v", got, MinCooldown)
	}
	if got := CooldownFor(5 * time.Minute); got != 5*time.Minute {
		t.Errorf("mid hint = %v", got)
	}
	if got := CooldownFor(2 * time.Hour); got != DefaultRateLimitCooldown {
		t.Errorf("long hint = %v, want the ceiling", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	if got := ParseRetryAfter([]byte("no hint here")); got != 0 {
		t.Errorf("got %v, want 0", got)
	}
	// The real Gemini free-tier message.
	if got := ParseRetryAfter([]byte("Please retry in 58.826548043s.")); got < 59*time.Second || got > 60*time.Second {
		t.Errorf("got %v, want ~59.8s", got)
	}
	if got := ParseRetryAfter([]byte("retry after 2 minutes")); got < 2*time.Minute {
		t.Errorf("got %v, want >= 2m", got)
	}
}

func TestPortsAreSharedByName(t *testing.T) {
	t.Parallel()
	// Two subsystems opening the same upstream must see one meter, which is
	// the entire reason this package exists.
	first, second := Open("shared-test"), Open("shared-test")
	if first != second {
		t.Error("Open returned distinct ports for one name")
	}
	if first == Open("other-test") {
		t.Error("distinct names shared a port")
	}
}

// A per-minute limit must cost about a minute, not an hour. The guard against
// mistaking one kind of quota for the other lives at classification time, in
// QuotaScope, which reads the structured quotaId rather than the prose; by the
// time an Attempt says Daily, that has already been established.
func TestAPerMinuteLimitCostsAboutAMinute(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t10")
	err := p.Call(context.Background(), []string{"a"}, func(_ context.Context, _ string) Attempt {
		return Attempt{
			Verdict:    RateLimited,
			RetryAfter: 26 * time.Second,
			Err:        errors.New("429"),
		}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	open, until, _ := p.CooldownOpen()
	if !open {
		t.Fatal("port should be closed")
	}
	if remaining := time.Until(until); remaining > 5*time.Minute {
		t.Errorf("cooldown is %v; a 26-second limit must not idle the port for hours", remaining)
	}
}

func TestADailyQuotaWithNoHintStillCostsHours(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t11")
	err := p.Call(context.Background(), []string{"a"}, func(_ context.Context, _ string) Attempt {
		return Attempt{Verdict: RateLimited, Daily: true, Err: errors.New("429")}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	_, until, _ := p.CooldownOpen()
	if remaining := time.Until(until); remaining < time.Hour {
		t.Errorf("cooldown is %v; a genuine daily exhaustion should wait hours", remaining)
	}
}

func TestASpentModelIsNotRediscovered(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t12")

	// The regression this exists to prevent: every chunk of one recording is
	// a separate Call, and each was re-trying the exhausted first model, so a
	// three-model chain tripled consumption of the quota it was conserving.
	calls := map[string]int{}
	attempt := func(_ context.Context, m string) Attempt {
		calls[m]++
		if m == "spent" {
			return Attempt{Verdict: RateLimited, RetryAfter: 5 * time.Minute, Err: errors.New("429")}
		}
		return OK()
	}

	for i := 0; i < 4; i++ {
		if err := p.Call(context.Background(), []string{"spent", "fresh"}, attempt); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if calls["spent"] != 1 {
		t.Errorf("spent model was tried %d times, want 1", calls["spent"])
	}
	if calls["fresh"] != 4 {
		t.Errorf("fresh model served %d calls, want 4", calls["fresh"])
	}
}

func TestEveryModelRestingCostsNoRequest(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t13")
	p.RestModel("a", time.Hour)
	p.RestModel("b", time.Hour)

	calls := 0
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, _ string) Attempt {
		calls++
		return OK()
	})
	if err == nil {
		t.Fatal("expected an error when every model is resting")
	}
	if calls != 0 {
		t.Errorf("issued %d requests with nothing available", calls)
	}
}

func TestRestExpires(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t14")
	p.RestModel("a", time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if p.ModelResting("a") {
		t.Error("rest did not expire")
	}
}

func TestRestIsNeverShortened(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t15")
	p.RestModel("a", time.Hour)
	p.RestModel("a", time.Second) // a concurrent caller with a weaker hint
	if !p.ModelResting("a") {
		t.Error("a long rest was overwritten by a short one")
	}
}

func TestQuotaScopeReadsTheStructuredQuotaID(t *testing.T) {
	t.Parallel()
	// The real refusal: a spent *daily* quota whose prose still says to retry
	// in 23 seconds, because that is when the rate bucket refills. Reading the
	// sentence gives the wrong answer; the quotaId gives the right one.
	daily := []byte(`{"error":{"code":429,"message":"You exceeded your current quota. Please retry in 23.11s.",
	  "details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[
	    {"quotaMetric":"generativelanguage.googleapis.com/generate_content_free_tier_requests",
	     "quotaId":"GenerateRequestsPerDayPerProjectPerModel","quotaValue":"20"}]}]}}`)
	got, known := QuotaScope(daily)
	if !known || !got {
		t.Errorf("daily=%v known=%v, want true/true", got, known)
	}

	perMinute := []byte(`{"error":{"details":[{"violations":[{"quotaId":"GenerateRequestsPerMinutePerProjectPerModel"}]}]}}`)
	if got, known := QuotaScope(perMinute); !known || got {
		t.Errorf("daily=%v known=%v, want false/true", got, known)
	}

	// A per-minute violation listed alongside a daily one that was not the
	// quota exceeded must still resolve to daily only when daily is named.
	if _, known := QuotaScope([]byte(`{"error":{"message":"no structured quota here"}}`)); known {
		t.Error("claimed to know the quota scope with no quotaId present")
	}
}

func TestADailyQuotaIgnoresAMisleadingRetryHint(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t16")
	err := p.Call(context.Background(), []string{"a"}, func(_ context.Context, _ string) Attempt {
		// Exactly what Google sends: daily exhaustion, 23-second hint.
		return Attempt{Verdict: RateLimited, RetryAfter: 24 * time.Second, Daily: true, Err: errors.New("429")}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	_, until, _ := p.CooldownOpen()
	if remaining := time.Until(until); remaining < time.Hour {
		t.Errorf("cooldown %v: a spent daily quota must not be retried in seconds", remaining)
	}
}

func TestADailyExhaustedModelRestsForTheDay(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t17")
	// Google's daily refusal carries a 23-second hint, which is the rate
	// bucket refilling rather than the day turning. Resting for that long
	// would spend a request every half minute on a model that is finished
	// until tomorrow.
	_ = p.Call(context.Background(), []string{"spent", "fresh"}, func(_ context.Context, m string) Attempt {
		if m == "spent" {
			return Attempt{Verdict: RateLimited, RetryAfter: 23 * time.Second, Daily: true, Err: errors.New("429")}
		}
		return OK()
	})
	if !p.ModelResting("spent") {
		t.Fatal("daily-exhausted model was not rested")
	}
	state := p.readState()
	if remaining := time.Until(state.Models["spent"]); remaining < time.Hour {
		t.Errorf("rest is %v; a spent daily quota should last hours", remaining)
	}
}

func TestUnusableOutputDoesNotClosethePort(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t18")
	// One model answers with something unusable, the next is out of quota.
	// The first still has quota and will probably answer properly next time,
	// so closing the port would idle a working model for hours.
	err := p.Call(context.Background(), []string{"garbled", "spent"}, func(_ context.Context, m string) Attempt {
		if m == "garbled" {
			return Attempt{Verdict: Unusable, Err: errors.New("bad json")}
		}
		return Attempt{Verdict: RateLimited, Daily: true, Err: errors.New("429")}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if open, _, _ := p.CooldownOpen(); open {
		t.Error("port closed although a model with quota remained")
	}
	if p.ModelResting("garbled") {
		t.Error("a model that merely answered badly was rested")
	}
	if !p.ModelResting("spent") {
		t.Error("the quota-exhausted model was not rested")
	}
}

func TestEveryModelOnQuotaStillClosesThePort(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t19")
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, _ string) Attempt {
		return Attempt{Verdict: RateLimited, Daily: true, Err: errors.New("429")}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if open, _, _ := p.CooldownOpen(); !open {
		t.Error("port stayed open with every model out of quota")
	}
}

func TestAPerModelQuotaNeverClosesThePort(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t20")
	// Google's refusal names GenerateRequestsPerDayPerProjectPerModel: this
	// model is finished for the day, its neighbours are untouched. Closing the
	// endpoint would idle every model that still had quota, including ones
	// outside the configured chain.
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, _ string) Attempt {
		return Attempt{Verdict: RateLimited, Daily: true, PerModel: true, Err: errors.New("429")}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if open, _, _ := p.CooldownOpen(); open {
		t.Error("a per-model quota closed the whole endpoint")
	}
	if !p.ModelResting("a") || !p.ModelResting("b") {
		t.Error("the exhausted models were not rested")
	}
}

func TestAnEndpointWideQuotaStillClosesThePort(t *testing.T) {
	t.Parallel()
	p := newTestPort(t, "t21")
	// A quota belonging to the project rather than the model means no model
	// will answer, so there is nothing to gain by asking each in turn.
	err := p.Call(context.Background(), []string{"a", "b"}, func(_ context.Context, _ string) Attempt {
		return Attempt{Verdict: RateLimited, Daily: true, Err: errors.New("429")}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if open, _, _ := p.CooldownOpen(); !open {
		t.Error("an endpoint-wide quota left the port open")
	}
}

func TestQuotaDetailDistinguishesPerModel(t *testing.T) {
	t.Parallel()
	perModel := []byte(`{"error":{"details":[{"violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel"}]}]}}`)
	daily, model, known := QuotaDetail(perModel)
	if !daily || !model || !known {
		t.Errorf("daily=%v perModel=%v known=%v", daily, model, known)
	}
	perProject := []byte(`{"error":{"details":[{"violations":[{"quotaId":"GenerateRequestsPerDayPerProject"}]}]}}`)
	if daily, model, known := QuotaDetail(perProject); !daily || model || !known {
		t.Errorf("daily=%v perModel=%v known=%v", daily, model, known)
	}
}
