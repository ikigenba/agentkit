package agentkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestErrorCategorySentinelsMatchWithErrorsIs(t *testing.T) {
	// R-BVYY-B2AX
	sentinels := []error{
		agentkit.ErrAuthentication,
		agentkit.ErrPermission,
		agentkit.ErrInvalidRequest,
		agentkit.ErrNotFound,
		agentkit.ErrRateLimited,
		agentkit.ErrOverloaded,
		agentkit.ErrServerError,
		agentkit.ErrTimeout,
		agentkit.ErrNetwork,
		agentkit.ErrContextLength,
		agentkit.ErrContentFilter,
		agentkit.ErrBilling,
		agentkit.ErrUnknown,
	}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			err := &agentkit.Error{Category: sentinel, Provider: "provider"}
			if !errors.Is(err, sentinel) {
				t.Fatalf("errors.Is(%T{Category: %q}, %q) = false, want true", err, sentinel, sentinel)
			}

			for _, other := range sentinels {
				if other == sentinel {
					continue
				}
				if errors.Is(err, other) {
					t.Fatalf("errors.Is(%T{Category: %q}, %q) = true, want false", err, sentinel, other)
				}
			}
		})
	}
}

func TestProviderErrorsRemainDistinctFromOrchestrationSentinels(t *testing.T) {
	// R-I5VJ-CTXE
	providerErr := &agentkit.Error{Category: agentkit.ErrBilling, Provider: "fake"}
	provider := newFakeProvider(newRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, providerErr))
	stream := (&agentkit.Conversation{Provider: provider, Model: testModel}).Send(context.Background(), "hello")
	drain(stream)
	var asProvider *agentkit.Error
	if !errors.As(stream.Err(), &asProvider) || asProvider.Category == nil {
		t.Fatalf("provider Err() = %v, want *agentkit.Error with category", stream.Err())
	}

	sentinels := []struct {
		name   string
		target error
		err    error
	}{
		{name: "tool loop limit", target: agentkit.ErrToolLoopLimit, err: orchestrationSentinelErr(t, agentkit.ErrToolLoopLimit)},
		{name: "stream pending", target: agentkit.ErrStreamPending, err: orchestrationSentinelErr(t, agentkit.ErrStreamPending)},
		{name: "closed", target: agentkit.ErrClosed, err: orchestrationSentinelErr(t, agentkit.ErrClosed)},
	}
	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.target) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", tc.err, tc.target)
			}
			var providerErr *agentkit.Error
			if errors.As(tc.err, &providerErr) {
				t.Fatalf("errors.As(%v, *agentkit.Error) = true, want false", tc.err)
			}
		})
	}
}

func TestBoundaryValidationSentinelsAreNotProviderErrors(t *testing.T) {
	// R-7CYE-KS40
	for _, sentinel := range []error{agentkit.ErrInvalidConfig, agentkit.ErrInvalidInput} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			var stream *agentkit.Stream
			switch sentinel {
			case agentkit.ErrInvalidConfig:
				stream = (&agentkit.Conversation{Model: testModel}).Send(context.Background(), "hello")
			case agentkit.ErrInvalidInput:
				stream = (&agentkit.Conversation{Provider: newFakeProvider(), Model: testModel}).Send(context.Background(), "")
			}
			drain(stream)
			if !errors.Is(stream.Err(), sentinel) {
				t.Fatalf("Err() = %v, want %v", stream.Err(), sentinel)
			}
			var providerErr *agentkit.Error
			if errors.As(stream.Err(), &providerErr) {
				t.Fatalf("errors.As(%v, *agentkit.Error) = true, want false", stream.Err())
			}
		})
	}
}

func orchestrationSentinelErr(t *testing.T, sentinel error) error {
	t.Helper()
	switch sentinel {
	case agentkit.ErrToolLoopLimit:
		provider := newFakeProvider()
		provider.roundTripFn = func(context.Context, *agentkit.Request) *agentkit.RoundTrip {
			return newRoundTrip(nil, assistant(agentkit.ToolUseBlock{ID: testToolUseID, Name: "missing", Input: []byte(`{}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil)
		}
		stream := (&agentkit.Conversation{Provider: provider, Model: testModel, MaxToolIterations: 1}).Send(context.Background(), "hello")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrToolLoopLimit) {
			t.Fatalf("tool loop Err() = %v, want ErrToolLoopLimit", stream.Err())
		}
		return stream.Err()
	case agentkit.ErrStreamPending:
		conv := &agentkit.Conversation{Provider: newFakeProvider(textRoundTrip("first")), Model: testModel}
		_ = conv.Send(context.Background(), "first")
		stream := conv.Send(context.Background(), "second")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrStreamPending) {
			t.Fatalf("stream pending Err() = %v, want ErrStreamPending", stream.Err())
		}
		return stream.Err()
	case agentkit.ErrClosed:
		conv := &agentkit.Conversation{Provider: newFakeProvider(), Model: testModel}
		if err := conv.Close(); err != nil {
			t.Fatalf("Close() = %v, want nil", err)
		}
		stream := conv.Send(context.Background(), "after close")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrClosed) {
			t.Fatalf("closed Err() = %v, want ErrClosed", stream.Err())
		}
		return stream.Err()
	default:
		t.Fatalf("unsupported sentinel %v", sentinel)
		return nil
	}
}
