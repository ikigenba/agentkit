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
	const recordedReference = "55cd8ee108603f0dc897439ffd96cd22fdfd7e1c6ee19286740941c2e295408b"
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

func TestRangeCanDisableMatchesOffVocabulary(t *testing.T) {
	// R-DL89-XAFP
	for model, entry := range entries {
		for i, offering := range entry.Offerings {
			spec := offering.Reasoning
			if spec == nil || spec.Kind != ReasoningRange {
				continue
			}
			derived := spec.Min == 0
			for _, sentinel := range spec.Sentinels {
				derived = derived || sentinel.Meaning == "off"
			}
			if spec.CanDisable != derived {
				t.Errorf("%s offering %d CanDisable = %v, derived off vocabulary = %v", model, i, spec.CanDisable, derived)
			}
		}
	}
}

func TestPrimaryChatReasoningDefaultsAreAudited(t *testing.T) {
	// R-DMG6-B26E
	for model, entry := range entries {
		if entry.Embedding != nil {
			continue
		}
		if len(entry.Offerings) == 0 || entry.Offerings[0].Reasoning == nil {
			t.Errorf("%s has no primary reasoning spec", model)
			continue
		}
		if entry.Offerings[0].Reasoning.Default.Mode == DefaultUnaudited {
			t.Errorf("%s has an unaudited primary reasoning default", model)
		}
	}
}

func TestOfferingAuditTierRequiresCompletePrimaryTerms(t *testing.T) {
	// R-LSJR-XP4J
	var incompleteSecondary bool
	for model, entry := range entries {
		if entry.Embedding != nil {
			continue
		}
		if len(entry.Offerings) == 0 || entry.Offerings[0].Pricing == nil || entry.Offerings[0].Reasoning == nil {
			t.Errorf("%s has incomplete primary terms", model)
		}
		for _, offering := range entry.Offerings[1:] {
			incompleteSecondary = incompleteSecondary || offering.Pricing == nil || offering.Reasoning == nil
		}
	}
	if !incompleteSecondary {
		t.Error("catalog has no secondary offering demonstrating the unaudited tier")
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
