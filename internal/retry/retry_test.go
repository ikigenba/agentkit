package retry

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

var errRetry = errors.New("retry")

type fakeClock struct {
	now       time.Time
	jitter    []time.Duration
	jitterCap []time.Duration
	sleeps    []time.Duration
	cancel    context.CancelFunc
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	if c.cancel != nil {
		c.cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.now = c.now.Add(d)
	return nil
}

func (c *fakeClock) Jitter(cap time.Duration) time.Duration {
	c.jitterCap = append(c.jitterCap, cap)
	if len(c.jitter) == 0 {
		return cap
	}
	d := c.jitter[0]
	c.jitter = c.jitter[1:]
	return d
}

func TestDoUsesRetryAfterAndJitteredExponentialBackoff(t *testing.T) {
	t.Run("honors positive retry-after", func(t *testing.T) {
		clock := &fakeClock{}
		calls := 0
		_, err := Do(context.Background(), Policy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: time.Second}, clock, func() (string, error) {
			calls++
			if calls == 1 {
				return "", errRetry
			}
			return "ok", nil
		}, func(error) Decision {
			return Decision{Retryable: true, RetryAfter: 42 * time.Millisecond}
		}, nil)

		// R-IUBG-95CC
		if err != nil {
			t.Fatalf("Do() error = %v, want nil", err)
		}
		if !reflect.DeepEqual(clock.sleeps, []time.Duration{42 * time.Millisecond}) {
			t.Fatalf("sleeps = %v, want RetryAfter delay", clock.sleeps)
		}
		if len(clock.jitterCap) != 0 {
			t.Fatalf("jitter caps = %v, want none when RetryAfter is honored", clock.jitterCap)
		}
	})

	t.Run("uses full-jitter exponential caps when retry-after ignored", func(t *testing.T) {
		clock := &fakeClock{jitter: []time.Duration{5 * time.Millisecond, 11 * time.Millisecond, 17 * time.Millisecond}}
		calls := 0
		_, err := Do(context.Background(), Policy{
			MaxAttempts:      4,
			BaseDelay:        10 * time.Millisecond,
			MaxDelay:         25 * time.Millisecond,
			IgnoreRetryAfter: true,
		}, clock, func() (string, error) {
			calls++
			if calls < 4 {
				return "", errRetry
			}
			return "ok", nil
		}, func(error) Decision {
			return Decision{Retryable: true, RetryAfter: time.Hour}
		}, nil)

		// R-IUBG-95CC
		if err != nil {
			t.Fatalf("Do() error = %v, want nil", err)
		}
		wantCaps := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond}
		if !reflect.DeepEqual(clock.jitterCap, wantCaps) {
			t.Fatalf("jitter caps = %v, want %v", clock.jitterCap, wantCaps)
		}
		wantSleeps := []time.Duration{5 * time.Millisecond, 11 * time.Millisecond, 17 * time.Millisecond}
		if !reflect.DeepEqual(clock.sleeps, wantSleeps) {
			t.Fatalf("sleeps = %v, want %v", clock.sleeps, wantSleeps)
		}
	})
}

func TestDoStopsOnMaxAttemptsAndNonRetryable(t *testing.T) {
	t.Run("max attempts returns final error", func(t *testing.T) {
		clock := &fakeClock{jitter: []time.Duration{time.Millisecond, time.Millisecond}}
		errFinal := errors.New("final")
		calls := 0
		_, err := Do(context.Background(), Policy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}, clock, func() (string, error) {
			calls++
			if calls == 3 {
				return "", errFinal
			}
			return "", errRetry
		}, func(error) Decision {
			return Decision{Retryable: true}
		}, nil)

		// R-IWR9-0OTQ
		if !errors.Is(err, errFinal) {
			t.Fatalf("Do() error = %v, want final error", err)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3", calls)
		}
		if len(clock.sleeps) != 2 {
			t.Fatalf("sleeps = %v, want two sleeps before final attempt", clock.sleeps)
		}
	})

	t.Run("non retryable returns immediately", func(t *testing.T) {
		clock := &fakeClock{}
		calls := 0
		_, err := Do(context.Background(), Policy{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}, clock, func() (string, error) {
			calls++
			return "", errRetry
		}, func(error) Decision {
			return Decision{Retryable: false}
		}, nil)

		// R-IWR9-0OTQ
		if !errors.Is(err, errRetry) {
			t.Fatalf("Do() error = %v, want retry error", err)
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
		if len(clock.sleeps) != 0 {
			t.Fatalf("sleeps = %v, want none", clock.sleeps)
		}
	})
}

func TestDoHonorsMaxElapsedBudget(t *testing.T) {
	t.Run("stops before sleeping past max elapsed", func(t *testing.T) {
		clock := &fakeClock{jitter: []time.Duration{20 * time.Millisecond}}
		calls := 0
		_, err := Do(context.Background(), Policy{
			MaxAttempts: 3,
			BaseDelay:   20 * time.Millisecond,
			MaxDelay:    20 * time.Millisecond,
			MaxElapsed:  10 * time.Millisecond,
		}, clock, func() (string, error) {
			calls++
			return "", errRetry
		}, func(error) Decision {
			return Decision{Retryable: true}
		}, nil)

		// R-IXZ5-EGKF
		if !errors.Is(err, errRetry) {
			t.Fatalf("Do() error = %v, want retry error", err)
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want no retry attempt", calls)
		}
		if len(clock.sleeps) != 0 {
			t.Fatalf("sleeps = %v, want none past elapsed budget", clock.sleeps)
		}
	})

	t.Run("zero max elapsed applies no cap", func(t *testing.T) {
		clock := &fakeClock{jitter: []time.Duration{20 * time.Millisecond}}
		calls := 0
		_, err := Do(context.Background(), Policy{
			MaxAttempts: 2,
			BaseDelay:   20 * time.Millisecond,
			MaxDelay:    20 * time.Millisecond,
			MaxElapsed:  0,
		}, clock, func() (string, error) {
			calls++
			if calls == 1 {
				return "", errRetry
			}
			return "ok", nil
		}, func(error) Decision {
			return Decision{Retryable: true}
		}, nil)

		// R-IXZ5-EGKF
		if err != nil {
			t.Fatalf("Do() error = %v, want nil", err)
		}
		if !reflect.DeepEqual(clock.sleeps, []time.Duration{20 * time.Millisecond}) {
			t.Fatalf("sleeps = %v, want one sleep despite zero elapsed cap", clock.sleeps)
		}
	})
}

func TestDoContextCancellationDuringSleepStopsRetrying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clock := &fakeClock{jitter: []time.Duration{time.Second}, cancel: cancel}
	calls := 0
	_, err := Do(ctx, Policy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Second}, clock, func() (string, error) {
		calls++
		return "", errRetry
	}, func(error) Decision {
		return Decision{Retryable: true}
	}, nil)

	// R-IZ71-S8B4
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do() error = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want no retry after canceled sleep", calls)
	}
	if !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second}) {
		t.Fatalf("sleeps = %v, want attempted backoff sleep", clock.sleeps)
	}
}

func TestDoDefaultsZeroPolicy(t *testing.T) {
	t.Run("zero policy", func(t *testing.T) {
		clock := &fakeClock{jitter: []time.Duration{1, 2, 3}}
		calls := 0
		classifications := 0
		_, err := Do(context.Background(), Policy{}, clock, func() (string, error) {
			calls++
			return "", errRetry
		}, func(error) Decision {
			classifications++
			if classifications == 3 {
				return Decision{Retryable: true, RetryAfter: time.Hour}
			}
			return Decision{Retryable: true}
		}, nil)

		// R-J0EY-601T
		if !errors.Is(err, errRetry) {
			t.Fatalf("Do() error = %v, want retry error", err)
		}
		if calls != 4 {
			t.Fatalf("calls = %d, want default 4 attempts", calls)
		}
		wantCaps := []time.Duration{500 * time.Millisecond, time.Second}
		if !reflect.DeepEqual(clock.jitterCap, wantCaps) {
			t.Fatalf("jitter caps = %v, want default base exponential caps %v", clock.jitterCap, wantCaps)
		}
		wantSleeps := []time.Duration{1, 2, time.Hour}
		if !reflect.DeepEqual(clock.sleeps, wantSleeps) {
			t.Fatalf("sleeps = %v, want %v with RetryAfter unbounded by elapsed cap", clock.sleeps, wantSleeps)
		}
	})

	t.Run("zero max delay", func(t *testing.T) {
		clock := &fakeClock{jitter: []time.Duration{20 * time.Second, 30 * time.Second, 30 * time.Second}}
		_, err := Do(context.Background(), Policy{MaxAttempts: 4, BaseDelay: 20 * time.Second}, clock, func() (string, error) {
			return "", errRetry
		}, func(error) Decision {
			return Decision{Retryable: true}
		}, nil)

		// R-J0EY-601T
		if !errors.Is(err, errRetry) {
			t.Fatalf("Do() error = %v, want retry error", err)
		}
		wantCaps := []time.Duration{20 * time.Second, 30 * time.Second, 30 * time.Second}
		if !reflect.DeepEqual(clock.jitterCap, wantCaps) {
			t.Fatalf("jitter caps = %v, want default max delay caps %v", clock.jitterCap, wantCaps)
		}
	})
}
