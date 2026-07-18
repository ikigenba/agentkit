// Package zai implements the AgentKit provider SPI for Z.ai's GLM models.
package zai

import (
	"context"
	"net/http"
	"strings"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/openaicompat"
)

const defaultBaseURL = "https://api.z.ai/api/paas/v4"

// Credential is the closed set of credentials accepted by New.
type Credential interface {
	zaiCredential() credential
}

type credential struct {
	apiKey string
}

// APIKey authenticates requests with a Z.ai API key.
type APIKey string

func (key APIKey) zaiCredential() credential {
	return credential{apiKey: string(key)}
}

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
func New(cred Credential, opts ...Option) *Provider {
	var auth credential
	if cred != nil {
		auth = cred.zaiCredential()
	}
	cfg := options{baseURL: defaultBaseURL}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Provider{compat: openaicompat.New(openaicompat.Config{
		Provider:                 "zai",
		BaseURL:                  cfg.baseURL,
		APIKey:                   auth.apiKey,
		HTTPClient:               cfg.client,
		Classify:                 classify,
		WarnForcedToolChoiceAuto: true,
	})}
}

// Name labels Z.ai provider errors.
func (p *Provider) Name() string {
	return p.compat.Name()
}

// RoundTrip performs one Z.ai Chat-Completions model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	return p.compat.RoundTrip(ctx, req)
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
