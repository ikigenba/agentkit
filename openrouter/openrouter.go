// Package openrouter implements the AgentKit provider SPI for OpenRouter.
package openrouter

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/openaicompat"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

// Credential is the closed set of credentials accepted by New.
type Credential interface {
	openrouterCredential() credential
}

type credential struct {
	apiKey string
}

// APIKey authenticates requests with an OpenRouter API key.
type APIKey string

func (key APIKey) openrouterCredential() credential {
	return credential{apiKey: string(key)}
}

// Option configures an OpenRouter provider handle.
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

// Provider is an OpenRouter Chat-Completions provider.
type Provider struct {
	compat *openaicompat.Provider
}

// New constructs an OpenRouter provider. The API root is baked into the
// provider; consumers supply only an API key.
func New(cred Credential, opts ...Option) *Provider {
	var auth credential
	if cred != nil {
		auth = cred.openrouterCredential()
	}
	cfg := options{baseURL: defaultBaseURL}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Provider{compat: openaicompat.New(openaicompat.Config{
		Provider:         "openrouter",
		BaseURL:          cfg.baseURL,
		APIKey:           auth.apiKey,
		HTTPClient:       cfg.client,
		Classify:         classify,
		ReasoningEncoder: encodeReasoning,
		CostExtractor:    extractCost,
	})}
}

// Name labels OpenRouter provider errors.
func (p *Provider) Name() string {
	return p.compat.Name()
}

// RoundTrip performs one OpenRouter Chat-Completions model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	return p.compat.RoundTrip(ctx, req)
}

func encodeReasoning(value agentkit.ReasoningValue) (json.RawMessage, error) {
	if value.IsUnset() {
		return nil, nil
	}
	if value.Enabled() {
		return json.RawMessage(`{"reasoning":{"enabled":true}}`), nil
	}
	if value.Disabled() {
		return json.RawMessage(`{"reasoning":{"enabled":false}}`), nil
	}
	if level, ok := value.Level(); ok {
		return json.Marshal(map[string]any{"reasoning": map[string]any{"effort": level}})
	}
	if budget, ok := value.Budget(); ok {
		return json.Marshal(map[string]any{"reasoning": map[string]any{"max_tokens": budget}})
	}
	return nil, agentkit.ErrInvalidConfig
}

func extractCost(raw json.RawMessage) (agentkit.Cost, bool) {
	var usage struct {
		Cost        *float64 `json:"cost"`
		CostDetails struct {
			UpstreamInferenceCost *float64 `json:"upstream_inference_cost"`
		} `json:"cost_details"`
	}
	if err := json.Unmarshal(raw, &usage); err != nil || usage.Cost == nil {
		return 0, false
	}
	total := *usage.Cost
	if usage.CostDetails.UpstreamInferenceCost != nil {
		total += *usage.CostDetails.UpstreamInferenceCost
	}
	return agentkit.Cost(math.Round(total * 1_000_000_000)), true
}

func classify(status int, _, message string) error {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "context length") || strings.Contains(lower, "maximum context") {
		return agentkit.ErrContextLength
	}
	if strings.Contains(lower, "content filter") || strings.Contains(lower, "safety") {
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
	case http.StatusPaymentRequired:
		return agentkit.ErrBilling
	case http.StatusTooManyRequests:
		return agentkit.ErrRateLimited
	}
	if status >= 500 {
		return agentkit.ErrServerError
	}
	return agentkit.ErrUnknown
}
