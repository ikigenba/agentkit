package agentkit_test

import (
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
