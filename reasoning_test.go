package agentkit_test

import (
	"reflect"
	"testing"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/zai"
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

func TestProviderReasoningInspectorsExposeDesignSpecs(t *testing.T) {
	providers := map[string]struct {
		inspector agentkit.ReasoningInspector
		specs     map[string]agentkit.ReasoningSpec
	}{
		"zai": {
			inspector: zai.Reasoning,
			specs: map[string]agentkit.ReasoningSpec{
				zai.ModelGLM52: {
					Term: "effort (+ toggle)", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"high", "max"},
					Default:    agentkit.Level("max"),
					CanDisable: true,
				},
				zai.ModelGLM51: {
					Term: "effort (+ toggle)", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"high", "max"},
					Default:    agentkit.Level("max"),
					CanDisable: true,
				},
				zai.ModelGLM47: {
					Term: "thinking", Kind: agentkit.ReasoningToggle,
					CanDisable: true,
				},
				zai.ModelGLM46: {
					Term: "thinking", Kind: agentkit.ReasoningToggle,
					CanDisable: true,
				},
			},
		},
	}

	for name, provider := range providers {
		t.Run(name, func(t *testing.T) {
			// R-S6NB-RYUE, R-S7V8-5QL3
			supported := provider.inspector.SupportedReasoning()
			if !reflect.DeepEqual(supported, provider.specs) {
				t.Fatalf("SupportedReasoning() = %#v, want %#v", supported, provider.specs)
			}
			for model, want := range provider.specs {
				// R-S934-JIBS, R-EN2N-9B9F
				got, ok := provider.inspector.ReasoningSpec(model)
				if !ok {
					t.Fatalf("ReasoningSpec(%q) ok=false, want true", model)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("ReasoningSpec(%q) = %#v, want %#v", model, got, want)
				}
				// R-EPIG-0UQT
				if !got.Accepts(got.Default) {
					t.Fatalf("ReasoningSpec(%q).Default = %#v, want accepted by its own spec %#v", model, got.Default, got)
				}
			}
			if got, ok := provider.inspector.ReasoningSpec("unknown-model"); ok {
				t.Fatalf("ReasoningSpec(unknown-model) = %#v, true; want false", got)
			}
		})
	}
}
