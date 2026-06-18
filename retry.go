package agentkit

import (
	"context"
	"math/rand"
	"time"
)

const (
	defaultRetryAttempts = 4
	defaultRetryBase     = 500 * time.Millisecond
	defaultRetryMaxDelay = 30 * time.Second
)

// RetryPolicy controls automatic retrying of transient provider failures.
//
// The zero value uses defaults. The budget is per provider round trip, not per
// whole turn; the caller's context remains the overall turn timeout.
type RetryPolicy struct {
	MaxAttempts      int
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	MaxElapsed       time.Duration
	IgnoreRetryAfter bool
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaultRetryAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaultRetryBase
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = defaultRetryMaxDelay
	}
	return p
}

type retryClock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
	Jitter(time.Duration) time.Duration
}

type realRetryClock struct{}

func (realRetryClock) Now() time.Time {
	return time.Now()
}

func (realRetryClock) Sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (realRetryClock) Jitter(cap time.Duration) time.Duration {
	if cap <= 1 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(cap)))
}
