package agentkit_test

import (
	"reflect"
	"testing"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/anthropic"
	"github.com/ikigenba/agentkit/google"
	"github.com/ikigenba/agentkit/openai"
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
		"anthropic": {
			inspector: anthropic.Reasoning,
			specs: map[string]agentkit.ReasoningSpec{
				anthropic.ModelOpus48: {
					Term: "effort", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"low", "medium", "high", "xhigh", "max"},
					Default:    agentkit.Level("high"),
					CanDisable: true,
				},
				anthropic.ModelSonnet46: {
					Term: "effort", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"low", "medium", "high", "max"},
					Default:    agentkit.Level("high"),
					CanDisable: true,
				},
				anthropic.ModelHaiku45: {
					Term: "thinking budget", Kind: agentkit.ReasoningRange,
					Min:        1024,
					Max:        4096,
					Default:    agentkit.DisableReasoning(),
					CanDisable: true,
				},
			},
		},
		"google": {
			inspector: google.Reasoning,
			specs: map[string]agentkit.ReasoningSpec{
				google.ModelFlash25: {
					Term: "thinking budget", Kind: agentkit.ReasoningRange,
					Min:        0,
					Max:        24576,
					Sentinels:  []agentkit.Sentinel{{Value: 0, Meaning: "off"}, {Value: -1, Meaning: "dynamic"}},
					Default:    agentkit.Budget(-1),
					CanDisable: true,
				},
				google.ModelPro25: {
					Term: "thinking budget", Kind: agentkit.ReasoningRange,
					Min:       128,
					Max:       32768,
					Sentinels: []agentkit.Sentinel{{Value: -1, Meaning: "dynamic"}},
					Default:   agentkit.Budget(-1),
				},
				google.ModelFlash35: {
					Term: "thinking level", Kind: agentkit.ReasoningEnum,
					Levels: []string{"minimal", "low", "medium", "high"}, Default: agentkit.Level("medium"),
				},
				google.ModelLite31: {
					Term: "thinking level", Kind: agentkit.ReasoningEnum,
					Levels: []string{"minimal", "low", "medium", "high"}, Default: agentkit.Level("medium"),
				},
				google.ModelPro31Preview: {
					Term: "thinking level", Kind: agentkit.ReasoningEnum,
					Levels: []string{"low", "medium", "high"}, Default: agentkit.Level("high"),
				},
			},
		},
		"openai": {
			inspector: openai.Reasoning,
			specs: map[string]agentkit.ReasoningSpec{
				openai.ModelGPT55Pro: {
					Term: "effort", Kind: agentkit.ReasoningEnum,
					Levels: []string{"high", "xhigh"}, Default: agentkit.Level("high"),
				},
				openai.ModelGPT55: {
					Term: "effort", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"none", "low", "medium", "high", "xhigh"},
					Default:    agentkit.Level("medium"),
					CanDisable: true,
				},
				openai.ModelGPT54: {
					Term: "effort", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"none", "low", "medium", "high", "xhigh"},
					Default:    agentkit.Level("none"),
					CanDisable: true,
				},
				openai.ModelGPT54Mini: {
					Term: "effort", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"none", "low", "medium", "high", "xhigh"},
					Default:    agentkit.Level("none"),
					CanDisable: true,
				},
				openai.ModelGPT54Nano: {
					Term: "effort", Kind: agentkit.ReasoningEnum,
					Levels:     []string{"none", "low", "medium", "high", "xhigh"},
					Default:    agentkit.Level("none"),
					CanDisable: true,
				},
			},
		},
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
