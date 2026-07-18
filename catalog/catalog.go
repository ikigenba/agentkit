// Package catalog provides advisory metadata for commonly used models.
// Catalog coverage never controls whether a model can be sent to a provider.
package catalog

import (
	"encoding/json"
	"sort"

	"github.com/ikigenba/agentkit"
)

// Entry is the catalog metadata for one globally unique model name.
type Entry struct {
	Model     string
	Provider  string
	Routes    map[string]string
	Pricing   *agentkit.Pricing
	Reasoning *ReasoningSpec
	Context   int64
	Embedding *EmbeddingInfo
	Options   json.RawMessage
}

// ReasoningKind describes a model's native reasoning-control shape.
type ReasoningKind = agentkit.ReasoningKind

const (
	ReasoningEnum   = agentkit.ReasoningEnum
	ReasoningRange  = agentkit.ReasoningRange
	ReasoningToggle = agentkit.ReasoningToggle
)

// Sentinel records a magic reasoning-budget value and its meaning.
type Sentinel = agentkit.Sentinel

// ReasoningSpec describes a model's native reasoning vocabulary.
type ReasoningSpec = agentkit.ReasoningSpec

// EmbeddingInfo contains an embedding model's rates and capabilities.
type EmbeddingInfo struct {
	Pricing         agentkit.EmbeddingPricing
	NativeDimension int
	MinDimension    int
	MaxDimension    int
	MaxInputTokens  int
}

// Lookup returns advisory metadata for model. Unknown names remain runnable.
func Lookup(model string) (Entry, bool) {
	entry, ok := entries[model]
	if !ok {
		return Entry{}, false
	}
	return cloneEntry(entry), true
}

// Resolve selects a provider and wire model without ever rejecting free-form input.
func Resolve(provider, model string) (routeProvider, wireModel string, entry Entry, ok bool) {
	entry, ok = Lookup(model)
	if !ok {
		return provider, model, Entry{}, false
	}
	if provider == "" {
		provider = entry.Provider
	}
	wireModel, routed := entry.Routes[provider]
	if !routed && provider == entry.Provider {
		wireModel = entry.Model
		routed = true
	}
	if !routed {
		return provider, model, Entry{}, false
	}
	return provider, wireModel, entry, true
}

// ListByProvider returns all entries reachable through provider, sorted by model.
func ListByProvider(provider string) []Entry {
	var out []Entry
	for _, entry := range entries {
		if entry.Provider == provider {
			out = append(out, cloneEntry(entry))
			continue
		}
		if _, ok := entry.Routes[provider]; ok {
			out = append(out, cloneEntry(entry))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// Check reports whether v is accepted by a cataloged model's reasoning spec.
func Check(model string, v agentkit.ReasoningValue) (accepted bool, spec *ReasoningSpec, ok bool) {
	entry, ok := Lookup(model)
	if !ok || entry.Reasoning == nil {
		return false, nil, ok
	}
	return entry.Reasoning.Accepts(v), entry.Reasoning, true
}

func cloneEntry(entry Entry) Entry {
	entry.Routes = cloneMap(entry.Routes)
	entry.Options = append(json.RawMessage(nil), entry.Options...)
	if entry.Pricing != nil {
		pricing := *entry.Pricing
		pricing.Tiers = append([]agentkit.RateTier(nil), pricing.Tiers...)
		entry.Pricing = &pricing
	}
	if entry.Reasoning != nil {
		reasoning := *entry.Reasoning
		reasoning.Levels = append([]string(nil), reasoning.Levels...)
		reasoning.Sentinels = append([]agentkit.Sentinel(nil), reasoning.Sentinels...)
		entry.Reasoning = &reasoning
	}
	if entry.Embedding != nil {
		embedding := *entry.Embedding
		entry.Embedding = &embedding
	}
	return entry
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
