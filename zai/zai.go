// Package zai implements the AgentKit provider SPI for Z.ai's GLM models.
package zai

import (
	"context"
	"net/http"
	"strings"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/openaicompat"
)

const (
	defaultBaseURL = "https://api.z.ai/api/paas/v4"

	ModelGLM52 = "glm-5.2"
	ModelGLM51 = "glm-5.1"
	ModelGLM47 = "glm-4.7"
	ModelGLM46 = "glm-4.6"
)

// Option configures a Z.ai provider handle.
type Option func(*options)

type options struct {
	baseURL string
	client  *http.Client
}

// WithBaseURL points the provider at a different API root, primarily for
// offline httptest fixtures.
func WithBaseURL(baseURL string) Option {
	return func(o *options) {
		o.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient sets the HTTP client used by the provider.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.client = client
	}
}

// Provider is a Z.ai GLM provider.
type Provider struct {
	compat *openaicompat.Provider
}

// New constructs a Z.ai provider. The Z.ai API root is baked into the provider;
// consumers supply only the API key.
func New(apiKey string, opts ...Option) *Provider {
	cfg := options{baseURL: defaultBaseURL}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Provider{compat: openaicompat.New(openaicompat.Config{
		Provider:                 "zai",
		BaseURL:                  cfg.baseURL,
		APIKey:                   apiKey,
		HTTPClient:               cfg.client,
		Pricing:                  pricingRegistry(),
		Classify:                 classify,
		WarnForcedToolChoiceAuto: true,
	})}
}

// Name labels Z.ai provider errors.
func (p *Provider) Name() string {
	return p.compat.Name()
}

// Pricing returns the model's baked-in pricing, if the model is supported.
func (p *Provider) Pricing(model string) (agentkit.Pricing, bool) {
	return p.compat.Pricing(model)
}

// RoundTrip performs one Z.ai Chat-Completions model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	return p.compat.RoundTrip(ctx, req)
}

// Reasoning exposes Z.ai's static native reasoning vocabulary.
var Reasoning agentkit.ReasoningInspector = reasoningInspector{}

type modelEntry struct {
	Pricing   agentkit.Pricing
	Reasoning agentkit.ReasoningSpec
}

var registry = map[string]modelEntry{
	ModelGLM52: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 1400, CacheReadInput: 260, Output: 4400,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort (+ toggle)", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"high", "max"},
			Default:    agentkit.Level("max"),
			CanDisable: true,
		},
	},
	ModelGLM51: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 1400, CacheReadInput: 260, Output: 4400,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort (+ toggle)", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"high", "max"},
			Default:    agentkit.Level("max"),
			CanDisable: true,
		},
	},
	ModelGLM47: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 600, CacheReadInput: 110, Output: 2200,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking", Kind: agentkit.ReasoningToggle,
			CanDisable: true,
		},
	},
	ModelGLM46: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 600, CacheReadInput: 110, Output: 2200,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking", Kind: agentkit.ReasoningToggle,
			CanDisable: true,
		},
	},
}

type reasoningInspector struct{}

func (reasoningInspector) ReasoningSpec(model string) (agentkit.ReasoningSpec, bool) {
	entry, ok := registry[model]
	if !ok {
		return agentkit.ReasoningSpec{}, false
	}
	return cloneReasoningSpec(entry.Reasoning), true
}

func (reasoningInspector) SupportedReasoning() map[string]agentkit.ReasoningSpec {
	out := make(map[string]agentkit.ReasoningSpec, len(registry))
	for model, entry := range registry {
		out[model] = cloneReasoningSpec(entry.Reasoning)
	}
	return out
}

func cloneReasoningSpec(spec agentkit.ReasoningSpec) agentkit.ReasoningSpec {
	spec.Levels = append([]string(nil), spec.Levels...)
	spec.Sentinels = append([]agentkit.Sentinel(nil), spec.Sentinels...)
	return spec
}

func pricingRegistry() map[string]agentkit.Pricing {
	out := make(map[string]agentkit.Pricing, len(registry))
	for model, entry := range registry {
		out[model] = entry.Pricing
	}
	return out
}

func classify(status int, code, message string) error {
	switch code {
	case "1001", "1002", "1003":
		return agentkit.ErrAuthentication
	case "1302", "1303":
		return agentkit.ErrRateLimited
	case "1230", "1234":
		return agentkit.ErrServerError
	case "1110", "1111", "1112", "1113", "1304", "1308", "1310":
		return agentkit.ErrBilling
	}

	lower := strings.ToLower(message)
	if strings.Contains(lower, "context") || strings.Contains(lower, "token limit") || strings.Contains(lower, "maximum length") {
		return agentkit.ErrContextLength
	}
	if strings.Contains(lower, "content_filter") || strings.Contains(lower, "safety") || strings.Contains(lower, "sensitive") {
		return agentkit.ErrContentFilter
	}

	switch status {
	case http.StatusUnauthorized:
		return agentkit.ErrAuthentication
	case http.StatusForbidden:
		return agentkit.ErrPermission
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return agentkit.ErrInvalidRequest
	case http.StatusNotFound:
		return agentkit.ErrNotFound
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return agentkit.ErrTimeout
	case http.StatusTooManyRequests:
		return agentkit.ErrRateLimited
	}
	if status >= 500 {
		return agentkit.ErrServerError
	}
	return agentkit.ErrUnknown
}
