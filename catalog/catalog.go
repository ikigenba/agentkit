// Package catalog provides advisory metadata for commonly used models.
// Catalog coverage never controls whether a model can be sent to a provider.
package catalog

import (
	"encoding/json"
	"sort"

	"github.com/ikigenba/agentkit"
)

// Entry is everything the catalog knows about one globally unique model name.
type Entry struct {
	Model     string
	Vendor    VendorID
	Offerings []Offering
	Embedding *EmbeddingInfo
}

// Offering is one model as served by one provider.
type Offering struct {
	Provider  agentkit.ProviderID
	Pricing   *agentkit.Pricing
	Reasoning *ReasoningSpec
	Context   int64
	Options   json.RawMessage
}

// VendorID names the model's creator using OpenRouter namespace spelling.
type VendorID string

const (
	VendorAnthropic VendorID = "anthropic"
	VendorOpenAI    VendorID = "openai"
	VendorGoogle    VendorID = "google"
	VendorZAI       VendorID = "z-ai"
	VendorXAI       VendorID = "x-ai"
	VendorDeepSeek  VendorID = "deepseek"
	VendorMoonshot  VendorID = "moonshotai"
)

// ReasoningKind describes a model's native reasoning-control shape.
type ReasoningKind int

const (
	ReasoningEnum ReasoningKind = iota
	ReasoningRange
	ReasoningToggle
)

// Sentinel records a magic reasoning-budget value and its meaning.
type Sentinel struct {
	Value   int
	Meaning string
}

// ReasoningSpec describes one offering's native reasoning vocabulary.
type ReasoningSpec struct {
	Term       string
	Kind       ReasoningKind
	Levels     []string
	Min, Max   int
	Sentinels  []Sentinel
	CanEnable  bool
	CanDisable bool
	Default    ReasoningDefault
}

// ReasoningDefault describes provider behavior when no reasoning value is sent.
type ReasoningDefault struct {
	Mode  ReasoningDefaultMode
	Value agentkit.ReasoningValue
}

// ReasoningDefaultMode classifies an offering's unset reasoning behavior.
type ReasoningDefaultMode int

const (
	DefaultUnaudited ReasoningDefaultMode = iota
	DefaultOff
	DefaultFixed
	DefaultDynamic
)

// EmbeddingInfo contains an embedding model's rates and capabilities.
type EmbeddingInfo struct {
	Pricing         agentkit.EmbeddingPricing
	NativeDimension int
	MinDimension    int
	MaxDimension    int
	MaxInputTokens  int
}

// Coverage describes how much catalog information Resolve found.
type Coverage int

const (
	Curated Coverage = iota
	Passthru
	Unrouted
)

// Resolution is the advisory result for a provider/model pair.
type Resolution struct {
	Vendor    VendorID
	Provider  agentkit.ProviderID
	WireModel string
	Offering  Offering
	Coverage  Coverage
}

// Lookup returns advisory metadata for model. Unknown names remain runnable.
func Lookup(model string) (Entry, bool) {
	entry, ok := entries[model]
	if !ok {
		return Entry{}, false
	}
	return cloneEntry(entry), true
}

// Offerings returns a model's offerings in authored preference order.
func Offerings(model string) []Offering {
	entry, ok := Lookup(model)
	if !ok {
		return nil
	}
	return entry.Offerings
}

// Offer returns a model's offering for provider.
func Offer(model string, provider agentkit.ProviderID) (Offering, bool) {
	entry, ok := entries[model]
	if !ok {
		return Offering{}, false
	}
	for _, offering := range entry.Offerings {
		if offering.Provider == provider {
			return cloneOffering(offering), true
		}
	}
	return Offering{}, false
}

// WireModel derives the model id sent to provider.
func (e Entry) WireModel(provider agentkit.ProviderID) string {
	if provider == agentkit.ProviderOpenRouter {
		return string(e.Vendor) + "/" + e.Model
	}
	return e.Model
}

// Resolve answers a provider/model query without rejecting free-form input.
func Resolve(provider agentkit.ProviderID, model string) Resolution {
	entry, known := entries[model]
	if provider == "" {
		if !known {
			return Resolution{WireModel: model, Coverage: Unrouted}
		}
		offering := cloneOffering(entry.Offerings[0])
		return Resolution{
			Vendor:    entry.Vendor,
			Provider:  offering.Provider,
			WireModel: entry.WireModel(offering.Provider),
			Offering:  offering,
			Coverage:  Curated,
		}
	}
	if known {
		for _, offering := range entry.Offerings {
			if offering.Provider == provider {
				offering = cloneOffering(offering)
				return Resolution{
					Vendor:    entry.Vendor,
					Provider:  provider,
					WireModel: entry.WireModel(provider),
					Offering:  offering,
					Coverage:  Curated,
				}
			}
		}
		return Resolution{Vendor: entry.Vendor, Provider: provider, WireModel: model, Coverage: Passthru}
	}
	return Resolution{Provider: provider, WireModel: model, Coverage: Passthru}
}

// ListCurated returns entries carrying an offering for provider, sorted by model.
func ListCurated(provider agentkit.ProviderID) []Entry {
	var out []Entry
	for _, entry := range entries {
		for _, offering := range entry.Offerings {
			if offering.Provider == provider {
				out = append(out, cloneEntry(entry))
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// Check reports whether v is accepted by a model's offering for provider.
func Check(model string, provider agentkit.ProviderID, v agentkit.ReasoningValue) (accepted bool, spec *ReasoningSpec, ok bool) {
	offering, ok := Offer(model, provider)
	if !ok {
		return false, nil, false
	}
	if offering.Reasoning == nil {
		return false, nil, true
	}
	return offering.Reasoning.accepts(v), offering.Reasoning, true
}

func (s ReasoningSpec) accepts(v agentkit.ReasoningValue) bool {
	if v.IsUnset() {
		return true
	}
	if v.Disabled() {
		return s.CanDisable
	}
	if v.Enabled() {
		return s.Kind == ReasoningToggle && s.CanEnable
	}
	switch s.Kind {
	case ReasoningEnum:
		level, ok := v.Level()
		if !ok {
			return false
		}
		for _, candidate := range s.Levels {
			if candidate == level {
				return true
			}
		}
		return false
	case ReasoningRange:
		budget, ok := v.Budget()
		if !ok {
			return false
		}
		if budget >= s.Min && budget <= s.Max {
			return true
		}
		for _, sentinel := range s.Sentinels {
			if sentinel.Value == budget {
				return true
			}
		}
		return false
	case ReasoningToggle:
		return false
	default:
		return false
	}
}

func cloneEntry(entry Entry) Entry {
	entry.Offerings = cloneOfferings(entry.Offerings)
	if entry.Embedding != nil {
		embedding := *entry.Embedding
		entry.Embedding = &embedding
	}
	return entry
}

func cloneOfferings(in []Offering) []Offering {
	if in == nil {
		return nil
	}
	out := make([]Offering, len(in))
	for i, offering := range in {
		out[i] = cloneOffering(offering)
	}
	return out
}

func cloneOffering(offering Offering) Offering {
	offering.Options = append(json.RawMessage(nil), offering.Options...)
	if offering.Pricing != nil {
		pricing := *offering.Pricing
		pricing.Tiers = append([]agentkit.RateTier(nil), pricing.Tiers...)
		offering.Pricing = &pricing
	}
	if offering.Reasoning != nil {
		reasoning := *offering.Reasoning
		reasoning.Levels = append([]string(nil), reasoning.Levels...)
		reasoning.Sentinels = append([]Sentinel(nil), reasoning.Sentinels...)
		offering.Reasoning = &reasoning
	}
	return offering
}
