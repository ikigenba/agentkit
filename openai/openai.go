// Package openai implements the AgentKit provider SPI for OpenAI's Responses
// API. Costs resolved for subscription-authenticated turns are notional
// API-rate equivalents rather than subscription spend.
package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/openaicompat"
	"github.com/ikigenba/agentkit/internal/responses"
)

const (
	defaultBaseURL      = "https://api.openai.com"
	subscriptionBaseURL = "https://chatgpt.com"

	EmbedModel3Small = "text-embedding-3-small"
	EmbedModel3Large = "text-embedding-3-large"
)

// Credential is the closed set of credentials accepted by New and NewEmbedder.
type Credential interface {
	isCredential()
}

// APIKey authenticates requests with an OpenAI API key.
type APIKey string

func (APIKey) isCredential() {}

// TokenSource supplies the current bearer token and ChatGPT account ID for
// subscription authentication.
type TokenSource interface {
	Token(ctx context.Context) (bearer, accountID string, err error)
}

type subscriptionCredential struct {
	tokenSource TokenSource
}

func (subscriptionCredential) isCredential() {}

// Subscription authenticates Responses requests with a ChatGPT subscription.
func Subscription(ts TokenSource) Credential {
	return subscriptionCredential{tokenSource: ts}
}

// Option configures an OpenAI provider handle.
type Option func(*Provider)

// WithBaseURL points the provider at a different API root, primarily for
// offline httptest fixtures.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) {
		p.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient sets the HTTP client used by the provider.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// Provider is an OpenAI Responses API provider.
type Provider struct {
	apiKey       string
	tokenSource  TokenSource
	subscription bool
	baseURL      string
	client       *http.Client
	now          func() time.Time
}

// New constructs an OpenAI provider using cred.
func New(cred Credential, opts ...Option) *Provider {
	p := &Provider{baseURL: defaultBaseURL, now: time.Now}
	switch cred := cred.(type) {
	case APIKey:
		p.apiKey = string(cred)
	case subscriptionCredential:
		p.tokenSource = cred.tokenSource
		p.subscription = true
		p.baseURL = subscriptionBaseURL
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// NewEmbedder constructs an OpenAI embeddings provider.
func NewEmbedder(cred Credential, opts ...Option) agentkit.EmbeddingProvider {
	p := New(cred, opts...)
	embedder := &embeddingProvider{cfg: openaicompat.EmbeddingConfig{
		Identity:   agentkit.Identity{Provider: agentkit.ProviderOpenAI, Auth: agentkit.AuthAPIKey},
		BaseURL:    p.baseURL,
		APIKey:     p.apiKey,
		HTTPClient: p.client,
		Now:        p.now,
		Classify:   classifyEmbedding,
	}}
	if p.subscription {
		embedder.credentialErr = fmt.Errorf("openai: a ChatGPT subscription credential cannot serve embeddings; an API key is required: %w", agentkit.ErrInvalidConfig)
	}
	return embedder
}

// Identity identifies the OpenAI package and selected credential mode.
func (p *Provider) Identity() agentkit.Identity {
	if p != nil && p.subscription {
		return agentkit.Identity{Provider: agentkit.ProviderOpenAI, Auth: agentkit.AuthSubscription}
	}
	return agentkit.Identity{Provider: agentkit.ProviderOpenAI, Auth: agentkit.AuthAPIKey}
}

// RoundTrip performs one OpenAI Responses API model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if p == nil || req == nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig, 0, false)
	}
	if p.subscription && p.tokenSource == nil {
		err := fmt.Errorf("openai: ChatGPT subscription token source is absent: %w", agentkit.ErrMissingCredential)
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, err, 0, false)
	}
	if !p.subscription && p.apiKey == "" {
		err := fmt.Errorf("openai: API key is absent: %w", agentkit.ErrMissingCredential)
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, err, 0, false)
	}
	bearer := p.apiKey
	path := "/v1/responses"
	headers := make(http.Header)
	if p.subscription {
		var accountID string
		var err error
		bearer, accountID, err = p.tokenSource.Token(ctx)
		if err != nil {
			return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, p.providerTransportError(err), 0, false)
		}
		if bearer == "" || accountID == "" {
			return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig, 0, false)
		}
		path = "/backend-api/codex/responses"
		headers.Set("chatgpt-account-id", accountID)
		headers.Set("originator", "codex_cli_rs")
		headers.Set("OpenAI-Beta", "responses=experimental")
	}
	core := responses.New(responses.Config{
		Identity:     p.Identity(),
		BaseURL:      p.baseURL,
		Path:         path,
		ExtraHeaders: headers,
		Bearer:       func(context.Context) (string, error) { return bearer, nil },
		HTTPClient:   p.client,
		Classify:     func(status int, code, _ string) error { return classify(status, code, code) },
		Now:          p.now,
	})
	return core.RoundTrip(ctx, req)
}

type embeddingProvider struct {
	cfg           openaicompat.EmbeddingConfig
	credentialErr error
}

func (p *embeddingProvider) Identity() agentkit.Identity {
	if p == nil {
		return agentkit.Identity{Provider: agentkit.ProviderOpenAI, Auth: agentkit.AuthAPIKey}
	}
	return p.cfg.Identity
}

func (p *embeddingProvider) Embed(ctx context.Context, req *agentkit.EmbedRequest) *agentkit.EmbedRoundTrip {
	if p == nil {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, agentkit.ErrInvalidConfig)
	}
	if p.credentialErr != nil {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, p.credentialErr)
	}
	if p.cfg.APIKey == "" {
		err := fmt.Errorf("openai: API key is absent: %w", agentkit.ErrMissingCredential)
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, err)
	}
	return openaicompat.NewEmbeddingProvider(p.cfg).Embed(ctx, req)
}

func classifyEmbedding(status int, code, message string) error {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "context length") || strings.Contains(lower, "too many tokens") {
		return agentkit.ErrContextLength
	}
	return classify(status, code, code)
}

type responsesRequest = responses.Request
type toolDef = responses.ToolDef
type usagePayload = responses.UsagePayload

func (p *Provider) buildRequest(req *agentkit.Request) (responsesRequest, []agentkit.Warning, error) {
	return responses.BuildRequest(req, false)
}

func mapUsage(native usagePayload) (agentkit.Usage, error) {
	return responses.MapUsage(native)
}

func providerTransportError(err error) error {
	category := agentkit.ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = agentkit.ErrTimeout
	}
	return &agentkit.Error{
		Category: category,
		Provider: agentkit.ProviderOpenAI,
		Err:      err,
		Message:  err.Error(),
	}
}

func (p *Provider) providerTransportError(err error) error {
	return p.labelError(providerTransportError(err))
}

func (p *Provider) labelError(err error) error {
	var providerErr *agentkit.Error
	if !errors.As(err, &providerErr) {
		return err
	}
	copy := *providerErr
	identity := p.Identity()
	copy.Provider = identity.Provider
	copy.Auth = identity.Auth
	return &copy
}

func classify(status int, typ, code string) error {
	switch code {
	case "context_length_exceeded":
		return agentkit.ErrContextLength
	case "content_filter":
		return agentkit.ErrContentFilter
	case "insufficient_quota", "billing_hard_limit_reached":
		return agentkit.ErrBilling
	}
	switch typ {
	case "tokens", "context_length_exceeded":
		return agentkit.ErrContextLength
	case "content_filter":
		return agentkit.ErrContentFilter
	case "insufficient_quota", "billing_error":
		return agentkit.ErrBilling
	case "server_overloaded":
		return agentkit.ErrOverloaded
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
		if status == http.StatusServiceUnavailable {
			return agentkit.ErrOverloaded
		}
		return agentkit.ErrServerError
	}
	return agentkit.ErrUnknown
}
