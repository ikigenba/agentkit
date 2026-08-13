// Package xai implements the AgentKit provider SPI for xAI's Responses API.
package xai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/responses"
)

const defaultBaseURL = "https://api.x.ai"

// Credential is the closed set of credentials accepted by New.
type Credential interface{ isCredential() }

// APIKey authenticates requests with an xAI platform API key.
type APIKey string

func (APIKey) isCredential() {}

// TokenSource supplies a current SuperGrok subscription bearer token.
type TokenSource interface {
	Token(ctx context.Context) (bearer string, err error)
}

type subscriptionCredential struct{ tokenSource TokenSource }

func (subscriptionCredential) isCredential() {}

// Subscription authenticates requests with a SuperGrok subscription.
func Subscription(ts TokenSource) Credential {
	return subscriptionCredential{tokenSource: ts}
}

// Option configures an xAI provider handle.
type Option func(*Provider)

// WithBaseURL points the provider at a different API root.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithHTTPClient sets the HTTP client used by the provider.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) { p.client = client }
}

// Provider is an xAI Responses API provider.
type Provider struct {
	apiKey       string
	tokenSource  TokenSource
	subscription bool
	baseURL      string
	client       *http.Client
}

// New constructs an xAI provider using cred.
func New(cred Credential, opts ...Option) *Provider {
	p := &Provider{baseURL: defaultBaseURL}
	switch cred := cred.(type) {
	case APIKey:
		p.apiKey = string(cred)
	case subscriptionCredential:
		p.subscription = true
		p.tokenSource = cred.tokenSource
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Identity identifies xAI and the selected credential mode.
func (p *Provider) Identity() agentkit.Identity {
	auth := agentkit.AuthAPIKey
	if p != nil && p.subscription {
		auth = agentkit.AuthSubscription
	}
	return agentkit.Identity{Provider: agentkit.ProviderXAI, Auth: auth}
}

// RoundTrip performs one xAI Responses API model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if p == nil || req == nil {
		return failed(agentkit.ErrInvalidConfig)
	}
	if p.subscription && p.tokenSource == nil {
		return failed(fmt.Errorf("xai: SuperGrok subscription token source is absent: %w", agentkit.ErrMissingCredential))
	}
	if !p.subscription && p.apiKey == "" {
		return failed(fmt.Errorf("xai: API key is absent: %w", agentkit.ErrMissingCredential))
	}
	bearer := func(context.Context) (string, error) { return p.apiKey, nil }
	if p.subscription {
		bearer = p.tokenSource.Token
	}
	core := responses.New(responses.Config{
		Identity:           p.Identity(),
		BaseURL:            p.baseURL,
		Path:               "/v1/responses",
		Bearer:             bearer,
		HTTPClient:         p.client,
		Classify:           classify,
		CostExtractor:      extractCost,
		AllowBareReasoning: true,
	})
	return core.RoundTrip(ctx, req)
}

func failed(err error) *agentkit.RoundTrip {
	return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, err, 0, false)
}

func extractCost(raw json.RawMessage) (agentkit.Cost, bool) {
	var usage struct {
		Ticks *int64 `json:"cost_in_usd_ticks"`
	}
	if json.Unmarshal(raw, &usage) != nil || usage.Ticks == nil {
		return 0, false
	}
	return agentkit.Cost(*usage.Ticks / 10), true
}

func classify(status int, code, message string) error {
	lowerCode := strings.ToLower(code)
	lowerMessage := strings.ToLower(message)
	if status == http.StatusUnauthorized && lowerCode == "unauthenticated:no-credentials" {
		return agentkit.ErrAuthentication
	}
	if status == http.StatusBadRequest && lowerCode == "invalid-argument" {
		if strings.Contains(lowerMessage, "incorrect api key provided") {
			return agentkit.ErrAuthentication
		}
		if strings.Contains(lowerMessage, "model not found") {
			return agentkit.ErrInvalidRequest
		}
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
	case http.StatusServiceUnavailable:
		return agentkit.ErrOverloaded
	}
	if status >= 500 {
		return agentkit.ErrServerError
	}
	return agentkit.ErrUnknown
}
