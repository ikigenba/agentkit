package agentkit

import (
	"errors"
	"time"

	internalretry "github.com/ikigenba/agentkit/internal/retry"
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

type retryClock = internalretry.Clock

type realRetryClock = internalretry.RealClock

func retryPolicy(p RetryPolicy) internalretry.Policy {
	return internalretry.Policy{
		MaxAttempts:      p.MaxAttempts,
		BaseDelay:        p.BaseDelay,
		MaxDelay:         p.MaxDelay,
		MaxElapsed:       p.MaxElapsed,
		IgnoreRetryAfter: p.IgnoreRetryAfter,
	}
}

func retryDecision(err error) internalretry.Decision {
	return internalretry.Decision{
		Retryable: errors.Is(err, ErrRateLimited) ||
			errors.Is(err, ErrOverloaded) ||
			errors.Is(err, ErrServerError) ||
			errors.Is(err, ErrTimeout) ||
			errors.Is(err, ErrNetwork),
		RetryAfter: retryAfter(err),
	}
}

func retryAfter(err error) time.Duration {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return providerErr.RetryAfter
	}
	return 0
}
