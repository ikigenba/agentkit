// Package retry provides the shared deterministic retry executor used by
// provider-facing retry loops.
package retry

import (
	"context"
	"math/rand"
	"time"
)

const (
	defaultMaxAttempts = 4
	defaultBaseDelay   = 500 * time.Millisecond
	defaultMaxDelay    = 30 * time.Second
)

// Clock is the deterministic retry seam.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
	Jitter(cap time.Duration) time.Duration
}

// RealClock is a production Clock backed by time and full jitter.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time {
	return time.Now()
}

// Sleep waits for d or returns the context error if the context is canceled.
func (RealClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Jitter returns a full-jitter delay in [0, cap).
func (RealClock) Jitter(cap time.Duration) time.Duration {
	if cap <= 1 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(cap)))
}

// Policy controls automatic retrying of transient failures.
type Policy struct {
	MaxAttempts      int
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	MaxElapsed       time.Duration
	IgnoreRetryAfter bool
}

// Decision classifies one attempt error.
type Decision struct {
	Retryable  bool
	RetryAfter time.Duration
}

// Do runs attempt until it succeeds, classify stops retrying, the retry budget
// is exhausted, or ctx is canceled while waiting between attempts.
func Do[T any](
	ctx context.Context,
	p Policy,
	clk Clock,
	attempt func() (T, error),
	classify func(error) Decision,
	onRetry func(err error, delay time.Duration),
) (T, error) {
	var zero T
	if clk == nil {
		clk = RealClock{}
	}
	p = withDefaults(p)
	start := clk.Now()

	for attemptNumber := 1; ; attemptNumber++ {
		value, err := attempt()
		if err == nil {
			return value, nil
		}
		decision := Decision{}
		if classify != nil {
			decision = classify(err)
		}
		if !decision.Retryable || attemptNumber >= p.MaxAttempts {
			return zero, err
		}
		delay := retryDelay(p, clk, start, attemptNumber, decision)
		if delay < 0 {
			return zero, err
		}
		if onRetry != nil {
			onRetry(err, delay)
		}
		if err := clk.Sleep(ctx, delay); err != nil {
			return zero, err
		}
	}
}

func withDefaults(p Policy) Policy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaultMaxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaultBaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = defaultMaxDelay
	}
	return p
}

func retryDelay(policy Policy, clock Clock, start time.Time, attempt int, decision Decision) time.Duration {
	if !policy.IgnoreRetryAfter && decision.RetryAfter > 0 {
		return boundedDelay(policy, clock, start, decision.RetryAfter)
	}
	return boundedDelay(policy, clock, start, clock.Jitter(backoffCap(policy, attempt)))
}

func boundedDelay(policy Policy, clock Clock, start time.Time, delay time.Duration) time.Duration {
	if delay < 0 {
		delay = 0
	}
	if policy.MaxElapsed == 0 {
		return delay
	}
	remaining := policy.MaxElapsed - clock.Now().Sub(start)
	if remaining < 0 || delay > remaining {
		return -1
	}
	return delay
}

func backoffCap(policy Policy, attempt int) time.Duration {
	delay := policy.BaseDelay
	for i := 1; i < attempt && delay < policy.MaxDelay; i++ {
		if delay > policy.MaxDelay/2 {
			delay = policy.MaxDelay
			break
		}
		delay *= 2
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}
