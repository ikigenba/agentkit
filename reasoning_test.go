package agentkit_test

import (
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestReasoningValueStatesAreMutuallyExclusive(t *testing.T) {
	// R-T6G3-NJ7L
	tests := []struct {
		name          string
		value         agentkit.ReasoningValue
		wantUnset     bool
		wantLevel     string
		wantHasLevel  bool
		wantBudget    int
		wantHasBudget bool
		wantDisabled  bool
	}{
		{name: "unset", value: agentkit.ReasoningValue{}, wantUnset: true},
		{name: "level", value: agentkit.Level("high"), wantLevel: "high", wantHasLevel: true},
		{name: "budget", value: agentkit.Budget(8000), wantBudget: 8000, wantHasBudget: true},
		{name: "disabled", value: agentkit.DisableReasoning(), wantDisabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.IsUnset(); got != tt.wantUnset {
				t.Fatalf("IsUnset() = %v, want %v", got, tt.wantUnset)
			}
			level, hasLevel := tt.value.Level()
			if level != tt.wantLevel || hasLevel != tt.wantHasLevel {
				t.Fatalf("Level() = %q, %v; want %q, %v", level, hasLevel, tt.wantLevel, tt.wantHasLevel)
			}
			budget, hasBudget := tt.value.Budget()
			if budget != tt.wantBudget || hasBudget != tt.wantHasBudget {
				t.Fatalf("Budget() = %d, %v; want %d, %v", budget, hasBudget, tt.wantBudget, tt.wantHasBudget)
			}
			if got := tt.value.Disabled(); got != tt.wantDisabled {
				t.Fatalf("Disabled() = %v, want %v", got, tt.wantDisabled)
			}
		})
	}
}
