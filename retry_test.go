package agentkit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

var retryTestPricing = Pricing{Tiers: []RateTier{{
	MinInputTokens: 0,
	InputUncached:  1,
	Output:         1,
}}}

type retryProvider struct {
	roundTrips []*RoundTrip
	calls      int
}

func (p *retryProvider) RoundTrip(context.Context, *Request) *RoundTrip {
	p.calls++
	if len(p.roundTrips) == 0 {
		return retryTextRoundTrip("ok")
	}
	rt := p.roundTrips[0]
	p.roundTrips = p.roundTrips[1:]
	return rt
}

func (p *retryProvider) Name() string {
	return "retry-test"
}

func (p *retryProvider) Pricing(string) (Pricing, bool) {
	return retryTestPricing, true
}

type fakeRetryClock struct {
	now           time.Time
	sleeps        []time.Duration
	jitterDivisor int
	cancelOnSleep context.CancelFunc
}

func (c *fakeRetryClock) Now() time.Time {
	return c.now
}

func (c *fakeRetryClock) Sleep(ctx context.Context, delay time.Duration) error {
	c.sleeps = append(c.sleeps, delay)
	if c.cancelOnSleep != nil {
		c.cancelOnSleep()
		return ctx.Err()
	}
	c.now = c.now.Add(delay)
	return ctx.Err()
}

func (c *fakeRetryClock) Jitter(cap time.Duration) time.Duration {
	if c.jitterDivisor <= 0 {
		return cap
	}
	return cap / time.Duration(c.jitterDivisor)
}

func TestRetryableFailureBeforeEventsRetriesToBudget(t *testing.T) {
	// R-P3LQ-QY2X
	finalErr := retryErr(ErrRateLimited)
	provider := &retryProvider{roundTrips: []*RoundTrip{
		retryErrorRoundTrip(ErrRateLimited),
		retryErrorRoundTrip(ErrRateLimited),
		retryErrorRoundTrip(finalErr),
		retryTextRoundTrip("unreached"),
	}}
	clock := &fakeRetryClock{}
	conv := &Conversation{
		Provider:   provider,
		Model:      "retry-model",
		Retry:      RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
		retryClock: clock,
	}

	stream := conv.Send(context.Background(), "hello")
	drainRetry(stream)

	if !errors.Is(stream.Err(), ErrRateLimited) || !errors.Is(stream.Err(), finalErr) {
		t.Fatalf("Err() = %v, want final rate-limit error", stream.Err())
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", provider.calls)
	}
	wantSleeps := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	if !reflect.DeepEqual(clock.sleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", clock.sleeps, wantSleeps)
	}
}

func TestNonRetryableFailuresAreNeverRetried(t *testing.T) {
	// R-P4TN-4PTM
	categories := []error{
		ErrAuthentication,
		ErrPermission,
		ErrInvalidRequest,
		ErrNotFound,
		ErrContextLength,
		ErrContentFilter,
		ErrBilling,
		ErrUnknown,
	}

	for _, category := range categories {
		t.Run(category.Error(), func(t *testing.T) {
			provider := &retryProvider{roundTrips: []*RoundTrip{
				retryErrorRoundTrip(category),
				retryTextRoundTrip("retried"),
			}}
			clock := &fakeRetryClock{}
			conv := &Conversation{
				Provider:   provider,
				Model:      "retry-model",
				Retry:      RetryPolicy{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
				retryClock: clock,
			}

			stream := conv.Send(context.Background(), "hello")
			drainRetry(stream)

			if !errors.Is(stream.Err(), category) {
				t.Fatalf("Err() = %v, want %v", stream.Err(), category)
			}
			if provider.calls != 1 {
				t.Fatalf("provider calls = %d, want 1", provider.calls)
			}
			if len(clock.sleeps) != 0 {
				t.Fatalf("sleeps = %v, want none", clock.sleeps)
			}
		})
	}
}

func TestFailureAfterFirstEventIsNotRetried(t *testing.T) {
	// R-P61J-IHKB
	provider := &retryProvider{roundTrips: []*RoundTrip{
		retryRoundTrip([]Event{TextDelta{Text: "partial"}}, retryAssistant(TextBlock{Text: "partial"}), FinishOther, retryErr(ErrNetwork)),
		retryTextRoundTrip("retried"),
	}}
	clock := &fakeRetryClock{}
	conv := &Conversation{
		Provider:   provider,
		Model:      "retry-model",
		Retry:      RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		retryClock: clock,
	}

	stream := conv.Send(context.Background(), "hello")
	events := drainRetry(stream)

	if got := retryText(events); got != "partial" {
		t.Fatalf("text events = %q, want partial", got)
	}
	if !errors.Is(stream.Err(), ErrNetwork) {
		t.Fatalf("Err() = %v, want ErrNetwork", stream.Err())
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if len(clock.sleeps) != 0 {
		t.Fatalf("sleeps = %v, want none", clock.sleeps)
	}
}

func TestRetryBudgetIsPerRoundTripInToolLoop(t *testing.T) {
	// R-Y878-6UDJ
	tool := RawTool("lookup", "lookup", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
		return "tool ok", nil
	})
	provider := &retryProvider{roundTrips: []*RoundTrip{
		retryErrorRoundTrip(ErrServerError),
		retryRoundTrip(
			[]Event{TextDelta{Text: "calling"}},
			retryAssistant(ToolUseBlock{ID: "toolu_retry", Name: "lookup", Input: json.RawMessage(`{}`)}),
			FinishToolUse,
			nil,
		),
		retryErrorRoundTrip(ErrServerError),
		retryTextRoundTrip("done"),
	}}
	clock := &fakeRetryClock{}
	conv := &Conversation{
		Provider:   provider,
		Model:      "retry-model",
		Tools:      []Tool{tool},
		Retry:      RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		retryClock: clock,
	}

	stream := conv.Send(context.Background(), "hello")
	events := drainRetry(stream)

	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if provider.calls != 4 {
		t.Fatalf("provider calls = %d, want first and second round trips to each get two attempts", provider.calls)
	}
	wantSleeps := []time.Duration{time.Millisecond, time.Millisecond}
	if !reflect.DeepEqual(clock.sleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", clock.sleeps, wantSleeps)
	}
	if got := retryText(events); got != "callingdone" {
		t.Fatalf("text events = %q, want callingdone", got)
	}
}

func TestRetryDelayUsesRetryAfterAndJitteredBackoff(t *testing.T) {
	t.Run("retry-after honored", func(t *testing.T) {
		// R-P79F-W9B0
		provider := &retryProvider{roundTrips: []*RoundTrip{
			retryErrorRoundTrip(&Error{Category: ErrRateLimited, RetryAfter: 42 * time.Millisecond}),
			retryTextRoundTrip("ok"),
		}}
		clock := &fakeRetryClock{jitterDivisor: 2}
		conv := &Conversation{
			Provider:   provider,
			Model:      "retry-model",
			Retry:      RetryPolicy{MaxAttempts: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 10 * time.Millisecond},
			retryClock: clock,
		}

		stream := conv.Send(context.Background(), "hello")
		drainRetry(stream)

		if err := stream.Err(); err != nil {
			t.Fatalf("Err() = %v, want nil", err)
		}
		if !reflect.DeepEqual(clock.sleeps, []time.Duration{42 * time.Millisecond}) {
			t.Fatalf("sleeps = %v, want RetryAfter delay", clock.sleeps)
		}
	})

	t.Run("full jitter exponential cap and ignore retry-after", func(t *testing.T) {
		// R-P79F-W9B0
		provider := &retryProvider{roundTrips: []*RoundTrip{
			retryErrorRoundTrip(&Error{Category: ErrRateLimited, RetryAfter: time.Second}),
			retryErrorRoundTrip(ErrServerError),
			retryTextRoundTrip("ok"),
		}}
		clock := &fakeRetryClock{}
		conv := &Conversation{
			Provider: provider,
			Model:    "retry-model",
			Retry: RetryPolicy{
				MaxAttempts:      3,
				BaseDelay:        10 * time.Millisecond,
				MaxDelay:         15 * time.Millisecond,
				IgnoreRetryAfter: true,
			},
			retryClock: clock,
		}

		stream := conv.Send(context.Background(), "hello")
		drainRetry(stream)

		if err := stream.Err(); err != nil {
			t.Fatalf("Err() = %v, want nil", err)
		}
		wantSleeps := []time.Duration{10 * time.Millisecond, 15 * time.Millisecond}
		if !reflect.DeepEqual(clock.sleeps, wantSleeps) {
			t.Fatalf("sleeps = %v, want jittered capped backoff %v", clock.sleeps, wantSleeps)
		}
	})
}

func TestContextCancellationDuringBackoffStopsRetrying(t *testing.T) {
	// R-P8HC-A11P
	ctx, cancel := context.WithCancel(context.Background())
	clock := &fakeRetryClock{cancelOnSleep: cancel}
	provider := &retryProvider{roundTrips: []*RoundTrip{
		retryErrorRoundTrip(ErrTimeout),
		retryTextRoundTrip("retried"),
	}}
	conv := &Conversation{
		Provider:   provider,
		Model:      "retry-model",
		Retry:      RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Second},
		retryClock: clock,
	}

	stream := conv.Send(ctx, "hello")
	drainRetry(stream)

	if !errors.Is(stream.Err(), context.Canceled) {
		t.Fatalf("Err() = %v, want context.Canceled", stream.Err())
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want no retry after canceled backoff", provider.calls)
	}
	if !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second}) {
		t.Fatalf("sleeps = %v, want one attempted backoff", clock.sleeps)
	}
}

func retryRoundTrip(events []Event, message Message, finish FinishReason, err error) *RoundTrip {
	return NewRoundTrip(func(yield func(Event) bool) {
		for _, ev := range events {
			if !yield(ev) {
				return
			}
		}
	}, message, finish, Usage{InputUncached: 1, Output: 1, Total: 2}, nil, err)
}

func retryErrorRoundTrip(err error) *RoundTrip {
	return retryRoundTrip(nil, Message{}, FinishOther, err)
}

func retryTextRoundTrip(text string) *RoundTrip {
	return retryRoundTrip([]Event{TextDelta{Text: text}}, retryAssistant(TextBlock{Text: text}), FinishStop, nil)
}

func retryAssistant(blocks ...Block) Message {
	return Message{Role: RoleAssistant, Blocks: blocks}
}

func retryErr(category error) *Error {
	return &Error{Category: category, Provider: "retry-test"}
}

func drainRetry(stream *Stream) []Event {
	var events []Event
	for ev := range stream.Events() {
		events = append(events, ev)
	}
	return events
}

func retryText(events []Event) string {
	var text string
	for _, ev := range events {
		if delta, ok := ev.(TextDelta); ok {
			text += delta.Text
		}
	}
	return text
}
