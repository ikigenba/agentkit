package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/retry"
)

const maxEmbeddingInputsPerRequest = 2048

// EmbeddingConfig describes one OpenAI-compatible embeddings endpoint.
type EmbeddingConfig struct {
	Provider   string
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Now        func() time.Time
	Clock      retry.Clock
	Pricing    map[string]agentkit.EmbeddingPricing
	Specs      map[string]agentkit.EmbeddingSpec
	Classify   ErrorClassifier
}

// EmbeddingProvider implements agentkit.EmbeddingProvider for /v1/embeddings.
type EmbeddingProvider struct {
	cfg EmbeddingConfig
}

// NewEmbeddingProvider constructs an OpenAI-compatible embeddings provider.
func NewEmbeddingProvider(cfg EmbeddingConfig) *EmbeddingProvider {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &EmbeddingProvider{cfg: cfg}
}

// Name labels provider errors.
func (p *EmbeddingProvider) Name() string {
	return p.cfg.Provider
}

// Pricing returns the provider-local embedding pricing.
func (p *EmbeddingProvider) Pricing(model string) (agentkit.EmbeddingPricing, bool) {
	if p == nil {
		return agentkit.EmbeddingPricing{}, false
	}
	pricing, ok := p.cfg.Pricing[model]
	return pricing, ok
}

// Embed performs one logical embedding call, splitting large batches as needed.
func (p *EmbeddingProvider) Embed(ctx context.Context, req *agentkit.EmbedRequest) *agentkit.EmbedRoundTrip {
	if p == nil || p.cfg.APIKey == "" || p.cfg.BaseURL == "" || req == nil {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, agentkit.ErrInvalidConfig)
	}
	spec, ok := p.cfg.Specs[req.Model]
	if !ok {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, agentkit.ErrInvalidConfig)
	}
	if req.Dimensions != 0 && (req.Dimensions < spec.MinDimension || req.Dimensions > spec.MaxDimension) {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, agentkit.ErrInvalidConfig)
	}
	if len(req.Inputs) == 0 {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, agentkit.ErrInvalidInput)
	}

	var all [][]float32
	var usage agentkit.EmbeddingUsage
	for start := 0; start < len(req.Inputs); start += maxEmbeddingInputsPerRequest {
		end := start + maxEmbeddingInputsPerRequest
		if end > len(req.Inputs) {
			end = len(req.Inputs)
		}
		vectors, chunkUsage, err := p.embedChunkWithRetry(ctx, req, req.Inputs[start:end])
		if err != nil {
			return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, err)
		}
		all = append(all, vectors...)
		usage = addEmbeddingUsage(usage, chunkUsage)
	}
	return agentkit.NewEmbedRoundTrip(all, usage, nil, nil)
}

func (p *EmbeddingProvider) embedChunkWithRetry(ctx context.Context, req *agentkit.EmbedRequest, inputs []string) ([][]float32, agentkit.EmbeddingUsage, error) {
	clock := p.cfg.Clock
	if clock == nil {
		clock = retry.RealClock{}
	}
	type chunk struct {
		vectors [][]float32
		usage   agentkit.EmbeddingUsage
	}
	result, err := retry.Do(ctx, embeddingRetryPolicy(req.Retry), clock, func() (chunk, error) {
		vectors, usage, err := p.embedChunk(ctx, req, inputs)
		if err == nil {
			return chunk{vectors: vectors, usage: usage}, nil
		}
		return chunk{}, err
	}, embeddingRetryDecision, nil)
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, err
	}
	return result.vectors, result.usage, nil
}

func (p *EmbeddingProvider) embedChunk(ctx context.Context, req *agentkit.EmbedRequest, inputs []string) ([][]float32, agentkit.EmbeddingUsage, error) {
	body := embeddingRequest{
		Model: req.Model,
		Input: append([]string(nil), inputs...),
	}
	if req.Dimensions != 0 {
		body.Dimensions = req.Dimensions
	}

	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, p.cfg.BaseURL+"/v1/embeddings", body)
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, p.transportError(err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := httpx.Client(p.cfg.HTTPClient).Do(httpReq)
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, p.transportError(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, p.transportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, agentkit.EmbeddingUsage{}, p.httpError(resp, raw)
	}

	var payload embeddingResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, agentkit.EmbeddingUsage{}, p.transportError(err)
	}
	vectors, err := orderedEmbeddingVectors(payload.Data, len(inputs))
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, err
	}
	return vectors, embeddingUsage(payload.Usage), nil
}

type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data  []embeddingItem    `json:"data"`
	Usage embeddingUsageBody `json:"usage"`
}

type embeddingItem struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type embeddingUsageBody struct {
	PromptTokens int64 `json:"prompt_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

type embeddingErrorBody struct {
	Error embeddingErrorPayload `json:"error"`
}

type embeddingErrorPayload struct {
	Message string          `json:"message"`
	Type    string          `json:"type"`
	Code    json.RawMessage `json:"code"`
}

func orderedEmbeddingVectors(data []embeddingItem, count int) ([][]float32, error) {
	if len(data) != count {
		return nil, &agentkit.Error{Category: agentkit.ErrUnknown, Message: "provider embedding count does not match input count"}
	}
	vectors := make([][]float32, count)
	seen := make([]bool, count)
	for _, item := range data {
		if item.Index < 0 || item.Index >= count || seen[item.Index] {
			return nil, &agentkit.Error{Category: agentkit.ErrUnknown, Message: "provider embedding index is invalid"}
		}
		vectors[item.Index] = append([]float32(nil), item.Embedding...)
		seen[item.Index] = true
	}
	return vectors, nil
}

func embeddingUsage(native embeddingUsageBody) agentkit.EmbeddingUsage {
	total := native.TotalTokens
	if total == 0 {
		total = native.PromptTokens
	}
	return agentkit.EmbeddingUsage{InputTokens: native.PromptTokens, Total: total}
}

func addEmbeddingUsage(a, b agentkit.EmbeddingUsage) agentkit.EmbeddingUsage {
	return agentkit.EmbeddingUsage{InputTokens: a.InputTokens + b.InputTokens, Total: a.Total + b.Total}
}

func (p *EmbeddingProvider) transportError(err error) error {
	category := agentkit.ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = agentkit.ErrTimeout
	}
	return &agentkit.Error{
		Category: category,
		Provider: p.cfg.Provider,
		Message:  err.Error(),
		Err:      err,
	}
}

func (p *EmbeddingProvider) httpError(resp *http.Response, raw []byte) error {
	var envelope embeddingErrorBody
	_ = json.Unmarshal(raw, &envelope)
	code := rawString(envelope.Error.Code)
	if code == "" {
		code = envelope.Error.Type
	}
	typ := envelope.Error.Type
	if typ == "" {
		typ = code
	} else if code != "" && code != typ {
		typ += ":" + code
	}
	message := envelope.Error.Message
	if message == "" {
		message = string(raw)
	}
	category := agentkit.ErrUnknown
	if p.cfg.Classify != nil {
		category = p.cfg.Classify(resp.StatusCode, code, message)
	}
	return &agentkit.Error{
		Category:   category,
		Provider:   p.cfg.Provider,
		StatusCode: resp.StatusCode,
		Type:       typ,
		Message:    message,
		RequestID:  resp.Header.Get("x-request-id"),
		RetryAfter: httpx.RetryAfter(resp.Header.Get("Retry-After"), p.cfg.Now()),
		Raw:        append(json.RawMessage(nil), raw...),
	}
}

func embeddingRetryPolicy(p agentkit.RetryPolicy) retry.Policy {
	return retry.Policy{
		MaxAttempts:      p.MaxAttempts,
		BaseDelay:        p.BaseDelay,
		MaxDelay:         p.MaxDelay,
		MaxElapsed:       p.MaxElapsed,
		IgnoreRetryAfter: p.IgnoreRetryAfter,
	}
}

func embeddingRetryDecision(err error) retry.Decision {
	return retry.Decision{
		Retryable: errors.Is(err, agentkit.ErrRateLimited) ||
			errors.Is(err, agentkit.ErrOverloaded) ||
			errors.Is(err, agentkit.ErrServerError) ||
			errors.Is(err, agentkit.ErrTimeout) ||
			errors.Is(err, agentkit.ErrNetwork),
		RetryAfter: embeddingRetryAfter(err),
	}
}

func embeddingRetryAfter(err error) time.Duration {
	var providerErr *agentkit.Error
	if errors.As(err, &providerErr) {
		return providerErr.RetryAfter
	}
	return 0
}
