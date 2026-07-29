// Package openai implements the AgentKit provider SPI for OpenAI's Responses
// API. Costs resolved for subscription-authenticated turns are notional
// API-rate equivalents rather than subscription spend.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/openaicompat"
	"github.com/ikigenba/agentkit/internal/sse"
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
	apiKey, ok := cred.(APIKey)
	if !ok {
		panic("openai: embeddings require an APIKey credential")
	}
	p := New(apiKey, opts...)
	return &embeddingProvider{cfg: openaicompat.EmbeddingConfig{
		Provider:   "openai",
		BaseURL:    p.baseURL,
		APIKey:     p.apiKey,
		HTTPClient: p.client,
		Now:        p.now,
		Classify:   classifyEmbedding,
	}}
}

// Name labels OpenAI provider errors.
func (p *Provider) Name() string {
	if p != nil && p.subscription {
		return "openai.subscription"
	}
	return "openai.apikey"
}

// RoundTrip performs one OpenAI Responses API model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if p == nil || (p.apiKey == "" && (!p.subscription || p.tokenSource == nil)) || req == nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig, 0, false)
	}
	bearer := p.apiKey
	accountID := ""
	path := "/v1/responses"
	if p.subscription {
		var err error
		bearer, accountID, err = p.tokenSource.Token(ctx)
		if err != nil {
			return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, p.providerTransportError(err), 0, false)
		}
		if bearer == "" || accountID == "" {
			return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig, 0, false)
		}
		path = "/backend-api/codex/responses"
	}

	body, warnings, err := p.buildRequest(req)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, err, 0, false)
	}

	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, p.baseURL+path, body)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.providerTransportError(err), 0, false)
	}
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.subscription {
		httpReq.Header.Set("chatgpt-account-id", accountID)
		httpReq.Header.Set("originator", "codex_cli_rs")
		httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	}

	resp, err := httpx.Client(p.client).Do(httpReq)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.providerTransportError(err), 0, false)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.providerTransportError(err), 0, false)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.providerHTTPError(resp, raw), 0, false)
	}

	frames, err := sse.ReadAll(strings.NewReader(string(raw)))
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.providerTransportError(err), 0, false)
	}
	assembled, err := assemble(frames)
	if err != nil {
		return agentkit.NewRoundTrip(assembled.message, assembled.finish, assembled.usage, warnings, p.labelError(err), 0, false)
	}
	warnings = append(warnings, assembled.warnings...)
	return agentkit.NewRoundTrip(assembled.message, assembled.finish, assembled.usage, warnings, nil, 0, false)
}

type embeddingProvider struct {
	cfg openaicompat.EmbeddingConfig
}

func (p *embeddingProvider) Name() string {
	if p == nil {
		return "openai"
	}
	return p.cfg.Provider
}

func (p *embeddingProvider) Embed(ctx context.Context, req *agentkit.EmbedRequest) *agentkit.EmbedRoundTrip {
	if p == nil {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, agentkit.ErrInvalidConfig)
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

type responsesRequest struct {
	Model           string                     `json:"model"`
	Stream          bool                       `json:"stream"`
	Store           bool                       `json:"store"`
	Include         []string                   `json:"include"`
	Instructions    string                     `json:"instructions,omitempty"`
	Input           []inputItem                `json:"input"`
	Tools           []toolDef                  `json:"tools,omitempty"`
	Temperature     *float64                   `json:"temperature,omitempty"`
	TopP            *float64                   `json:"top_p,omitempty"`
	MaxOutputTokens int                        `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningConf             `json:"reasoning,omitempty"`
	ProviderOptions map[string]json.RawMessage `json:"-"`
}

func (r responsesRequest) MarshalJSON() ([]byte, error) {
	type wireRequest responsesRequest
	raw, err := json.Marshal(wireRequest(r))
	if err != nil {
		return nil, err
	}
	if len(r.ProviderOptions) == 0 {
		return raw, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	for key, value := range r.ProviderOptions {
		body[key] = value
	}
	return json.Marshal(body)
}

type reasoningConf struct {
	Effort string `json:"effort"`
}

type inputItem struct {
	Type             string        `json:"type,omitempty"`
	Role             string        `json:"role,omitempty"`
	Content          []contentPart `json:"content,omitempty"`
	CallID           string        `json:"call_id,omitempty"`
	Output           string        `json:"output,omitempty"`
	EncryptedContent string        `json:"encrypted_content,omitempty"`
	Summary          any           `json:"summary,omitempty"`
	Name             string        `json:"name,omitempty"`
	Arguments        string        `json:"arguments,omitempty"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type summaryPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolDef struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (p *Provider) buildRequest(req *agentkit.Request) (responsesRequest, []agentkit.Warning, error) {
	out := responsesRequest{
		Model:   req.Model,
		Stream:  true,
		Store:   false,
		Include: []string{"reasoning.encrypted_content"},
		Input:   make([]inputItem, 0, len(req.Messages)),
	}
	if req.System != "" {
		out.Instructions = req.System
	}
	if req.Gen.Temperature != nil {
		out.Temperature = req.Gen.Temperature
	}
	if req.Gen.TopP != nil {
		out.TopP = req.Gen.TopP
	}
	if req.Gen.MaxTokens > 0 {
		out.MaxOutputTokens = req.Gen.MaxTokens
	}
	warnings, err := applyReasoning(req.Gen.Reasoning, &out)
	if err != nil {
		return responsesRequest{}, warnings, err
	}
	if len(req.ProviderOptions) != 0 {
		if err := json.Unmarshal(req.ProviderOptions, &out.ProviderOptions); err != nil || out.ProviderOptions == nil {
			return responsesRequest{}, warnings, fmt.Errorf("openai provider options must be a JSON object: %w", agentkit.ErrInvalidConfig)
		}
	}
	for _, tool := range req.Tools {
		out.Tools = append(out.Tools, toolDef{
			Type:        "function",
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.JSONSchema(),
		})
	}
	for _, message := range req.Messages {
		items, err := messageInputItems(message)
		if err != nil {
			return responsesRequest{}, warnings, err
		}
		out.Input = append(out.Input, items...)
	}
	return out, warnings, nil
}

func applyReasoning(value agentkit.ReasoningValue, out *responsesRequest) ([]agentkit.Warning, error) {
	if value.IsUnset() {
		return nil, nil
	}
	if value.Enabled() {
		return nil, fmt.Errorf("openai Responses API has no bare reasoning-on form: %w", agentkit.ErrInvalidConfig)
	}
	if value.Disabled() {
		out.Reasoning = &reasoningConf{Effort: "none"}
		return nil, nil
	}
	if level, ok := value.Level(); ok {
		out.Reasoning = &reasoningConf{Effort: level}
		return nil, nil
	}
	if _, ok := value.Budget(); ok {
		return nil, fmt.Errorf("openai Responses API cannot encode token-budget reasoning: %w", agentkit.ErrInvalidConfig)
	}
	return nil, fmt.Errorf("unknown OpenAI reasoning value: %w", agentkit.ErrInvalidConfig)
}

func messageInputItems(message agentkit.Message) ([]inputItem, error) {
	items := make([]inputItem, 0, len(message.Blocks))
	var textParts []contentPart
	flushText := func() {
		if len(textParts) == 0 {
			return
		}
		role := string(message.Role)
		if role == string(agentkit.RoleAssistant) {
			for i := range textParts {
				textParts[i].Type = "output_text"
			}
		}
		items = append(items, inputItem{Role: role, Content: textParts})
		textParts = nil
	}

	for _, block := range message.Blocks {
		switch block := block.(type) {
		case agentkit.TextBlock:
			typ := "input_text"
			if message.Role == agentkit.RoleAssistant {
				typ = "output_text"
			}
			textParts = append(textParts, contentPart{Type: typ, Text: block.Text})
		case agentkit.ToolUseBlock:
			flushText()
			items = append(items, inputItem{
				Type:      "function_call",
				CallID:    block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			})
		case agentkit.ToolResultBlock:
			flushText()
			items = append(items, inputItem{
				Type:   "function_call_output",
				CallID: block.ToolUseID,
				Output: block.Content,
			})
		case agentkit.ReasoningBlock:
			flushText()
			if item, ok := openAIReasoningItem(block); ok {
				items = append(items, item)
			}
		default:
			panic(fmt.Sprintf("unknown block type %T", block))
		}
	}
	flushText()
	return items, nil
}

func openAIReasoningItem(block agentkit.ReasoningBlock) (inputItem, bool) {
	var payload struct {
		Type             string `json:"type"`
		EncryptedContent string `json:"encrypted_content"`
	}
	if err := json.Unmarshal(block.Opaque, &payload); err != nil {
		return inputItem{}, false
	}
	if payload.Type != "reasoning" || payload.EncryptedContent == "" {
		return inputItem{}, false
	}
	item := inputItem{
		Type:             "reasoning",
		EncryptedContent: payload.EncryptedContent,
		Summary:          []summaryPart{},
	}
	if block.Summary != "" {
		item.Summary = []summaryPart{{Type: "summary_text", Text: block.Summary}}
	}
	return item, true
}

type assembledRoundTrip struct {
	message  agentkit.Message
	finish   agentkit.FinishReason
	usage    agentkit.Usage
	warnings []agentkit.Warning
}

type responseEvent struct {
	Type           string          `json:"type"`
	Delta          string          `json:"delta"`
	ItemID         string          `json:"item_id"`
	OutputIndex    int             `json:"output_index"`
	ContentIndex   int             `json:"content_index"`
	Arguments      string          `json:"arguments"`
	Response       responsePayload `json:"response"`
	Item           outputItem      `json:"item"`
	IncompleteInfo incompleteInfo  `json:"incomplete_details"`
	Usage          usagePayload    `json:"usage"`
}

type responsePayload struct {
	Status            string         `json:"status"`
	IncompleteDetails incompleteInfo `json:"incomplete_details"`
	Usage             usagePayload   `json:"usage"`
}

type incompleteInfo struct {
	Reason string `json:"reason"`
}

type outputItem struct {
	ID               string        `json:"id"`
	Type             string        `json:"type"`
	CallID           string        `json:"call_id"`
	Name             string        `json:"name"`
	Arguments        string        `json:"arguments"`
	Text             string        `json:"text"`
	EncryptedContent string        `json:"encrypted_content"`
	Summary          []summaryPart `json:"summary"`
}

type usagePayload struct {
	InputTokens         int64              `json:"input_tokens"`
	OutputTokens        int64              `json:"output_tokens"`
	TotalTokens         int64              `json:"total_tokens"`
	InputTokensDetails  inputTokenDetails  `json:"input_tokens_details"`
	OutputTokensDetails outputTokenDetails `json:"output_tokens_details"`
}

type inputTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type outputTokenDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type partialFunction struct {
	callID string
	name   string
	args   strings.Builder
}

func assemble(frames []sse.Event) (assembledRoundTrip, error) {
	var out assembledRoundTrip
	out.message = agentkit.Message{Role: agentkit.RoleAssistant}
	out.finish = agentkit.FinishStop
	functions := make(map[string]*partialFunction)
	var reasonings []agentkit.ReasoningBlock
	var visibleText strings.Builder
	var reasoningSummary strings.Builder

	for _, frame := range frames {
		if string(frame.Data) == "[DONE]" {
			continue
		}
		var ev responseEvent
		if err := json.Unmarshal(frame.Data, &ev); err != nil {
			return out, providerTransportError(err)
		}
		typ := ev.Type
		if typ == "" {
			typ = frame.Type
		}

		switch typ {
		case "response.output_text.delta":
			if ev.Delta != "" {
				visibleText.WriteString(ev.Delta)
			}
		case "response.reasoning_summary_text.delta":
			if ev.Delta != "" {
				reasoningSummary.WriteString(ev.Delta)
			}
		case "response.output_item.added":
			if ev.Item.Type == "function_call" {
				key := itemKey(ev)
				functions[key] = &partialFunction{callID: ev.Item.CallID, name: ev.Item.Name}
			}
		case "response.function_call_arguments.delta":
			fn := ensureFunction(functions, itemKey(ev))
			fn.args.WriteString(ev.Delta)
		case "response.function_call_arguments.done":
			fn := ensureFunction(functions, itemKey(ev))
			if ev.Arguments != "" {
				fn.args.Reset()
				fn.args.WriteString(ev.Arguments)
			}
		case "response.output_item.done":
			switch ev.Item.Type {
			case "function_call":
				key := itemKey(ev)
				fn := ensureFunction(functions, key)
				if fn.callID == "" {
					fn.callID = ev.Item.CallID
				}
				if fn.name == "" {
					fn.name = ev.Item.Name
				}
				if ev.Item.Arguments != "" {
					fn.args.Reset()
					fn.args.WriteString(ev.Item.Arguments)
				}
				out.message.Blocks = append(out.message.Blocks, functionBlock(fn))
				out.finish = agentkit.FinishToolUse
				delete(functions, key)
			case "reasoning":
				if ev.Item.EncryptedContent != "" {
					opaque, err := json.Marshal(struct {
						Type             string `json:"type"`
						EncryptedContent string `json:"encrypted_content"`
					}{Type: "reasoning", EncryptedContent: ev.Item.EncryptedContent})
					if err != nil {
						return out, providerTransportError(err)
					}
					reasonings = append(reasonings, agentkit.ReasoningBlock{
						Opaque:  opaque,
						Summary: summaryText(ev.Item.Summary, reasoningSummary.String()),
					})
				}
			}
		case "response.completed":
			usage, err := mapUsage(ev.Response.Usage)
			if err != nil {
				return out, err
			}
			out.usage = usage
			out.finish = finishFromResponse(ev.Response)
		case "response.incomplete":
			out.finish = finishFromIncomplete(ev.Response.IncompleteDetails)
		}
	}

	if visibleText.Len() > 0 {
		out.message.Blocks = append([]agentkit.Block{agentkit.TextBlock{Text: visibleText.String()}}, out.message.Blocks...)
	}
	if len(reasonings) > 0 {
		blocks := make([]agentkit.Block, 0, len(reasonings)+len(out.message.Blocks))
		for _, reasoning := range reasonings {
			blocks = append(blocks, reasoning)
		}
		blocks = append(blocks, out.message.Blocks...)
		out.message.Blocks = blocks
	}
	if len(functions) > 0 {
		for _, fn := range functions {
			out.message.Blocks = append(out.message.Blocks, functionBlock(fn))
		}
		out.finish = agentkit.FinishToolUse
	}
	if hasToolUse(out.message) {
		out.finish = agentkit.FinishToolUse
	}
	return out, nil
}

func itemKey(ev responseEvent) string {
	if ev.ItemID != "" {
		return ev.ItemID
	}
	if ev.Item.ID != "" {
		return ev.Item.ID
	}
	return fmt.Sprintf("%d", ev.OutputIndex)
}

func ensureFunction(functions map[string]*partialFunction, key string) *partialFunction {
	if fn := functions[key]; fn != nil {
		return fn
	}
	fn := &partialFunction{}
	functions[key] = fn
	return fn
}

func functionBlock(fn *partialFunction) agentkit.ToolUseBlock {
	input := json.RawMessage(fn.args.String())
	if !json.Valid(input) {
		input = json.RawMessage(`{}`)
	}
	return agentkit.ToolUseBlock{
		ID:    agentkit.NewToolUseID(),
		Name:  fn.name,
		Input: cloneRaw(input),
	}
}

func summaryText(parts []summaryPart, fallback string) string {
	if len(parts) == 0 {
		return fallback
	}
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(part.Text)
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}

func mapUsage(native usagePayload) (agentkit.Usage, error) {
	cached := native.InputTokensDetails.CachedTokens
	reasoning := native.OutputTokensDetails.ReasoningTokens
	if cached > native.InputTokens || reasoning > native.OutputTokens {
		return agentkit.Usage{}, &agentkit.Error{
			Category: agentkit.ErrUnknown,
			Provider: "openai",
			Message:  "provider usage details exceed native totals",
		}
	}
	usage := agentkit.Usage{
		InputUncached:   native.InputTokens - cached,
		CacheReadInput:  cached,
		CacheWriteInput: 0,
		CacheWrite5m:    0,
		CacheWrite1h:    0,
		Output:          native.OutputTokens - reasoning,
		ReasoningOutput: reasoning,
	}
	usage.Total = usage.InputUncached + usage.CacheReadInput + usage.CacheWriteInput + usage.Output + usage.ReasoningOutput
	if native.TotalTokens != 0 && native.TotalTokens != usage.Total {
		return agentkit.Usage{}, &agentkit.Error{
			Category: agentkit.ErrUnknown,
			Provider: "openai",
			Message:  "provider usage total does not equal mapped buckets",
		}
	}
	return usage, nil
}

func finishFromResponse(resp responsePayload) agentkit.FinishReason {
	if resp.IncompleteDetails.Reason != "" {
		return finishFromIncomplete(resp.IncompleteDetails)
	}
	switch resp.Status {
	case "incomplete":
		return agentkit.FinishMaxTokens
	default:
		return agentkit.FinishStop
	}
}

func finishFromIncomplete(info incompleteInfo) agentkit.FinishReason {
	switch info.Reason {
	case "max_output_tokens":
		return agentkit.FinishMaxTokens
	case "content_filter":
		return agentkit.FinishContentFilter
	default:
		return agentkit.FinishOther
	}
}

func hasToolUse(message agentkit.Message) bool {
	for _, block := range message.Blocks {
		if _, ok := block.(agentkit.ToolUseBlock); ok {
			return true
		}
	}
	return false
}

func providerTransportError(err error) error {
	category := agentkit.ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = agentkit.ErrTimeout
	}
	return &agentkit.Error{
		Category: category,
		Provider: "openai",
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
	copy.Provider = p.Name()
	return &copy
}

func (p *Provider) providerHTTPError(resp *http.Response, raw []byte) error {
	var envelope struct {
		Error struct {
			Message string          `json:"message"`
			Type    string          `json:"type"`
			Code    json.RawMessage `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	typ := envelope.Error.Type
	code := rawString(envelope.Error.Code)
	message := envelope.Error.Message
	if message == "" {
		message = string(raw)
	}
	if typ == "" {
		typ = code
	} else if code != "" {
		typ += ":" + code
	}

	category := classify(resp.StatusCode, envelope.Error.Type, code)
	return &agentkit.Error{
		Category:   category,
		Provider:   p.Name(),
		StatusCode: resp.StatusCode,
		Type:       typ,
		Message:    message,
		RequestID:  resp.Header.Get("x-request-id"),
		RetryAfter: httpx.RetryAfter(resp.Header.Get("Retry-After"), p.now()),
		Raw:        append(json.RawMessage(nil), raw...),
	}
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
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

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
