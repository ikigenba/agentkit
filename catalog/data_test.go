package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestGrokNativeOfferingsAndRouterAlternatives(t *testing.T) {
	// R-DMDH-5FOB
	// R-DZZQ-6RVO
	// R-E17M-KJMD
	sharedRates := pricing(
		agentkit.RateTier{InputUncached: 1250, CacheReadInput: 200, Output: 2500},
		agentkit.RateTier{MinInputTokens: 200001, InputUncached: 2500, CacheReadInput: 400, Output: 5000},
	)
	wantNative := map[string]Offering{
		"grok-4.5": {
			Provider: agentkit.ProviderXAI, Context: 500_000,
			Pricing: pricing(
				agentkit.RateTier{InputUncached: 2000, CacheReadInput: 300, Output: 6000},
				agentkit.RateTier{MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 600, Output: 12000},
			),
			Reasoning: enumReasoning("effort", []string{"low", "medium", "high"}, "high", false),
		},
		"grok-4.6": {
			Provider: agentkit.ProviderXAI, Context: 500_000,
			Pricing: pricing(
				agentkit.RateTier{InputUncached: 2000, CacheReadInput: 500, Output: 6000},
				agentkit.RateTier{MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 1000, Output: 12000},
			),
			Reasoning: enumReasoning("effort", []string{"low", "medium", "high", "xhigh"}, "high", false),
		},
		"grok-4.3": {
			Provider: agentkit.ProviderXAI, Context: 1_000_000, Pricing: sharedRates,
			Reasoning: enumReasoning("effort", []string{"low", "medium", "high"}, "low", false),
		},
		"grok-4.20": {
			Provider: agentkit.ProviderXAI, Context: 1_000_000, Pricing: sharedRates,
			Reasoning: &ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, Default: fixed(agentkit.EnableReasoning())},
		},
		"grok-4.20-multi-agent": {
			Provider: agentkit.ProviderXAI, Context: 1_000_000, Pricing: sharedRates,
			Reasoning: enumReasoning("effort", []string{"low", "medium", "high", "xhigh"}, "high", false),
		},
	}
	wantDivergentRouter := map[string]Offering{
		"grok-4.3": {
			Provider: agentkit.ProviderOpenRouter, Context: 256_000,
			Pricing:   pricing(agentkit.RateTier{InputUncached: 3000, Output: 15000}),
			Reasoning: &ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
		},
		"grok-4.20": {
			Provider: agentkit.ProviderOpenRouter, Context: 2_000_000,
			Pricing: pricing(
				agentkit.RateTier{InputUncached: 3000, Output: 15000},
				agentkit.RateTier{MinInputTokens: 200001, InputUncached: 6000, Output: 30000},
			),
			Reasoning: toggleReasoning(true, true, DefaultOff),
		},
	}

	for model, want := range wantNative {
		t.Run(model, func(t *testing.T) {
			entry, ok := Lookup(model)
			if !ok {
				t.Fatalf("Lookup(%q) returned ok=false", model)
			}
			if entry.Vendor != VendorXAI {
				t.Errorf("Vendor = %q, want %q", entry.Vendor, VendorXAI)
			}
			if len(entry.Offerings) < 2 {
				t.Fatalf("Offerings = %#v, want native and OpenRouter routes", entry.Offerings)
			}
			if got := entry.Offerings[0]; !reflect.DeepEqual(got, want) {
				t.Errorf("native offering = %#v, want %#v", got, want)
			}
			router, ok := Offer(model, agentkit.ProviderOpenRouter)
			if !ok || entry.Offerings[0].Provider == agentkit.ProviderOpenRouter {
				t.Fatalf("OpenRouter alternative = %#v/%v, want later offering", router, ok)
			}
			if wantRouter, exact := wantDivergentRouter[model]; exact && !reflect.DeepEqual(router, wantRouter) {
				t.Errorf("OpenRouter offering = %#v, want retained terms %#v", router, wantRouter)
			}
		})
	}
}

func TestCatalogDataMatchesRecordedReference(t *testing.T) {
	// R-DQ16-AQWE
	models := make([]string, 0, len(entries))
	for model := range entries {
		models = append(models, model)
	}
	sort.Strings(models)

	rows := make([]referenceRow, 0, len(models))
	for _, model := range models {
		entry := entries[model]
		row := referenceRow{Entry: entry}
		for _, offering := range entry.Offerings {
			if offering.Reasoning == nil {
				row.ReasoningDefaults = append(row.ReasoningDefaults, "none")
				continue
			}
			switch offering.Reasoning.Default.Mode {
			case DefaultUnaudited:
				row.ReasoningDefaults = append(row.ReasoningDefaults, "unaudited")
			case DefaultOff:
				row.ReasoningDefaults = append(row.ReasoningDefaults, "off")
			case DefaultDynamic:
				row.ReasoningDefaults = append(row.ReasoningDefaults, "dynamic")
			case DefaultFixed:
				value := offering.Reasoning.Default.Value
				if level, ok := value.Level(); ok {
					row.ReasoningDefaults = append(row.ReasoningDefaults, "fixed:level:"+level)
				} else if budget, ok := value.Budget(); ok {
					row.ReasoningDefaults = append(row.ReasoningDefaults, fmt.Sprintf("fixed:budget:%d", budget))
				} else if value.Enabled() {
					row.ReasoningDefaults = append(row.ReasoningDefaults, "fixed:enabled")
				} else if value.Disabled() {
					row.ReasoningDefaults = append(row.ReasoningDefaults, "fixed:disabled")
				} else {
					t.Errorf("%s has an invalid fixed reasoning default", model)
				}
				if !offering.Reasoning.Accepts(value) {
					t.Errorf("%s fixed reasoning default is not accepted by its own spec", model)
				}
			default:
				t.Errorf("%s has unknown reasoning default mode %d", model, offering.Reasoning.Default.Mode)
			}
		}
		rows = append(rows, row)
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	const recordedReference = "45f953cfc81301bca63156d85898f34d1ce7781879f08be650a3756773785355"
	if got := hex.EncodeToString(digest[:]); got != recordedReference {
		t.Fatalf("catalog data differs from recorded reference table: got %s, want %s", got, recordedReference)
	}
}

func TestReasoningDefaultValueMatchesMode(t *testing.T) {
	// R-DHKK-RZ7M
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			if offering.Reasoning == nil {
				continue
			}
			fixed := offering.Reasoning.Default.Mode == DefaultFixed
			if nonzero := !offering.Reasoning.Default.Value.IsUnset(); nonzero != fixed {
				t.Errorf("%s offering %d default value nonzero = %v, fixed = %v", model, i, nonzero, fixed)
			}
		}
	}
}

func TestFixedReasoningDefaultsAreAccepted(t *testing.T) {
	// R-DISH-5QYB
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			spec := offering.Reasoning
			if spec != nil && spec.Default.Mode == DefaultFixed && !spec.Accepts(spec.Default.Value) {
				t.Errorf("%s offering %d rejects its fixed default", model, i)
			}
		}
	}
}

func TestCanEnableIsOnlySetOnToggleSpecs(t *testing.T) {
	// R-DK0D-JIP0
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			spec := offering.Reasoning
			if spec != nil && spec.CanEnable && spec.Kind != ReasoningToggle {
				t.Errorf("%s offering %d enables reasoning on kind %d", model, i, spec.Kind)
			}
		}
	}
}

func TestShippedOfferingsAreComplete(t *testing.T) {
	// R-E5FU-SAHM
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			if offering.Context <= 0 {
				t.Errorf("%s offering %d has non-positive context %d", model, i, offering.Context)
			}
			if entry.Embedding != nil {
				continue
			}
			if offering.Pricing == nil {
				t.Errorf("%s offering %d has nil pricing", model, i)
			} else if len(offering.Pricing.Tiers) == 0 {
				t.Errorf("%s offering %d has no pricing tiers", model, i)
			}
			if offering.Reasoning == nil {
				t.Errorf("%s offering %d has nil reasoning", model, i)
			}
		}
	}
}

func TestShippedReasoningDefaultsAreAudited(t *testing.T) {
	// R-E6NR-628B
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			if offering.Reasoning != nil && offering.Reasoning.Default.Mode == DefaultUnaudited {
				t.Errorf("%s offering %d has an unaudited reasoning default", model, i)
			}
		}
	}
}

func TestEnableReasoningAcceptanceMatchesExplicitPermission(t *testing.T) {
	// R-DNO2-OTX3
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			spec := offering.Reasoning
			if spec == nil {
				continue
			}
			got := spec.Accepts(agentkit.EnableReasoning())
			if got != spec.CanEnable {
				t.Errorf("%s offering %d Accepts(EnableReasoning) = %v, CanEnable = %v", model, i, got, spec.CanEnable)
			}
			if spec.Kind != ReasoningToggle && got {
				t.Errorf("%s offering %d kind %d accepts explicit enable", model, i, spec.Kind)
			}
		}
	}
}

func TestOfferingProvidersAreExportedProviderIDs(t *testing.T) {
	// R-LTRO-BGV8
	valid := map[agentkit.ProviderID]bool{
		agentkit.ProviderAnthropic:  true,
		agentkit.ProviderOpenAI:     true,
		agentkit.ProviderGoogle:     true,
		agentkit.ProviderZAI:        true,
		agentkit.ProviderXAI:        true,
		agentkit.ProviderOpenRouter: true,
	}
	for model, entry := range entries {
		for _, offering := range entry.Offerings {
			if !valid[offering.Provider] {
				t.Errorf("%s has non-provider offering id %q", model, offering.Provider)
			}
		}
	}
	for _, forbidden := range []agentkit.ProviderID{
		agentkit.ProviderID(agentkit.AuthAPIKey),
		agentkit.ProviderID(agentkit.AuthSubscription),
		"openai.subscription",
		"zai",
	} {
		if valid[forbidden] {
			t.Errorf("credential mode or misspelling %q is accepted as a provider", forbidden)
		}
	}
}

func TestDirectVendorOfferingIsFirst(t *testing.T) {
	// R-LW7H-30CM
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			if string(offering.Provider) == string(entry.Vendor) && i != 0 {
				t.Errorf("%s direct offering is at index %d, want index 0", model, i)
			}
		}
	}
	glm := entries["glm-5.2"]
	if len(glm.Offerings) < 2 || glm.Offerings[0].Provider != agentkit.ProviderZAI || glm.Offerings[1].Provider != agentkit.ProviderOpenRouter {
		t.Fatalf("glm-5.2 offerings = %#v, want direct ZAI route first", glm.Offerings)
	}
}

func TestEveryShippedChatEntryHasOpenRouterOffering(t *testing.T) {
	// R-EBJC-P573
	for model, entry := range entries {
		if entry.Embedding != nil {
			continue
		}
		found := false
		for _, offering := range entry.Offerings {
			if offering.Provider == agentkit.ProviderOpenRouter {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s has no OpenRouter offering", model)
		}
	}
}

func TestOnlyDottedAnthropicRoutesOverrideWireNames(t *testing.T) {
	// R-E7VN-JTZ0
	want := map[string]string{
		"claude-opus-4-8":   "anthropic/claude-opus-4.8",
		"claude-sonnet-4-6": "anthropic/claude-sonnet-4.6",
		"claude-haiku-4-5":  "anthropic/claude-haiku-4.5",
	}
	for model, entry := range entries {
		for _, offering := range entry.Offerings {
			if offering.WireName == "" {
				continue
			}
			if offering.Provider != agentkit.ProviderOpenRouter {
				t.Errorf("%s has wire-name override on provider %q", model, offering.Provider)
			}
			if got, ok := want[model]; !ok || offering.WireName != got {
				t.Errorf("%s wire-name override = %q, want %q", model, offering.WireName, got)
			}
			delete(want, model)
		}
	}
	for model, wireName := range want {
		t.Errorf("%s is missing OpenRouter wire-name override %q", model, wireName)
	}
}

func TestNativeRangeCanDisableMatchesReachableOffBudget(t *testing.T) {
	// R-EABG-BDGE
	for model, entry := range entries {
		for _, offering := range entry.Offerings {
			spec := offering.Reasoning
			if offering.Provider == agentkit.ProviderOpenRouter || spec == nil || spec.Kind != ReasoningRange {
				continue
			}
			derived := spec.Min == 0
			for _, sentinel := range spec.Sentinels {
				if sentinel.Meaning == "off" {
					derived = true
					break
				}
			}
			if spec.CanDisable != derived {
				t.Errorf("%s native range CanDisable = %v, reachable off budget = %v", model, spec.CanDisable, derived)
			}
		}
	}

	flash, _ := Offer("gemini-2.5-flash", agentkit.ProviderGoogle)
	if flash.Reasoning == nil || flash.Reasoning.Min != 0 || !flash.Reasoning.CanDisable {
		t.Fatalf("gemini-2.5-flash native range = %#v, want reachable off and CanDisable", flash.Reasoning)
	}
	pro, _ := Offer("gemini-2.5-pro", agentkit.ProviderGoogle)
	if pro.Reasoning == nil || pro.Reasoning.Min != 128 || pro.Reasoning.CanDisable {
		t.Fatalf("gemini-2.5-pro native range = %#v, want no reachable off and CanDisable false", pro.Reasoning)
	}
	haiku, ok := Offer("claude-haiku-4-5", agentkit.ProviderOpenRouter)
	if !ok || haiku.Reasoning == nil || haiku.Reasoning.Min != 1024 || !haiku.Reasoning.CanDisable {
		t.Fatalf("claude-haiku-4-5 OpenRouter range = %#v/%v, want scoped exception", haiku.Reasoning, ok)
	}
	for _, sentinel := range haiku.Reasoning.Sentinels {
		if sentinel.Meaning == "off" {
			t.Fatalf("claude-haiku-4-5 OpenRouter range unexpectedly has off sentinel: %#v", sentinel)
		}
	}
}

type referenceRow struct {
	Entry
	ReasoningDefaults []string
}
