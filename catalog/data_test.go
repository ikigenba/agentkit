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
				} else {
					t.Errorf("%s has an invalid fixed reasoning default", model)
				}
				if !offering.Reasoning.accepts(value) {
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
	const recordedReference = "71edd090f612bb3fe40a54ef8caefdb308f1f1c82ed21e2e8b8a9cd94e5c3564"
	if got := hex.EncodeToString(digest[:]); got != recordedReference {
		t.Fatalf("catalog data differs from recorded reference table: got %s, want %s", got, recordedReference)
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
