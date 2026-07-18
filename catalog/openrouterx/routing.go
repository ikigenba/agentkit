// Package openrouterx contains typed builders for OpenRouter provider options.
package openrouterx

import "encoding/json"

// MaxPrice limits OpenRouter prices in dollars per million units.
type MaxPrice struct {
	Prompt     float64 `json:"prompt,omitempty"`
	Completion float64 `json:"completion,omitempty"`
	Image      float64 `json:"image,omitempty"`
	Request    float64 `json:"request,omitempty"`
}

// Routing describes OpenRouter's provider-routing preferences.
type Routing struct {
	Order          []string
	Only           []string
	Ignore         []string
	Sort           string
	MaxPrice       *MaxPrice
	Quantizations  []string
	ZDR            bool
	AllowFallbacks *bool
}

// ProviderOptions emits the fragment consumed by agentkit.Request.ProviderOptions.
func (r Routing) ProviderOptions() json.RawMessage {
	provider := struct {
		Order          []string  `json:"order,omitempty"`
		Only           []string  `json:"only,omitempty"`
		Ignore         []string  `json:"ignore,omitempty"`
		Sort           string    `json:"sort,omitempty"`
		MaxPrice       *MaxPrice `json:"max_price,omitempty"`
		Quantizations  []string  `json:"quantizations,omitempty"`
		ZDR            bool      `json:"zdr,omitempty"`
		AllowFallbacks *bool     `json:"allow_fallbacks,omitempty"`
	}{r.Order, r.Only, r.Ignore, r.Sort, r.MaxPrice, r.Quantizations, r.ZDR, r.AllowFallbacks}
	out, err := json.Marshal(struct {
		Provider any `json:"provider"`
	}{Provider: provider})
	if err != nil {
		return nil
	}
	return out
}
