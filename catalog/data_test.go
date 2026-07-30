package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/ikigenba/agentkit"
)

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
	const recordedReference = "fbab166a95e0b6c04ed02d7eddc83428e3414304fc546325ef99fe443e281be6"
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

type referenceRow struct {
	Entry
	ReasoningDefaults []string
}
