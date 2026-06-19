// Package openai implements the AgentKit provider SPI for OpenAI's Responses
// API.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/sse"
)

const (
	defaultBaseURL = "https://api.openai.com"

	ModelGPT55Pro  = "gpt-5.5-pro"
	ModelGPT55     = "gpt-5.5"
	ModelGPT54     = "gpt-5.4"
	ModelGPT54Mini = "gpt-5.4-mini"
	ModelGPT54Nano = "gpt-5.4-nano"
)

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
	apiKey  string
	baseURL string
	client  *http.Client
	now     func() time.Time
}

// New constructs an OpenAI provider.
func New(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name labels OpenAI provider errors.
func (p *Provider) Name() string {
	return "openai"
}

// Pricing returns the model's baked-in pricing, if the model is supported.
func (p *Provider) Pricing(model string) (agentkit.Pricing, bool) {
	entry, ok := registry[model]
	return entry.Pricing, ok
}

// RoundTrip performs one OpenAI Responses API model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if p == nil || p.apiKey == "" || req == nil {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig)
	}
	if _, ok := p.Pricing(req.Model); !ok {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig)
	}

	body, err := p.buildRequest(req)
	if err != nil {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, err)
	}

	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, p.baseURL+"/v1/responses", body)
	if err != nil {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, providerTransportError(err))
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := httpx.Client(p.client).Do(httpReq)
	if err != nil {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, providerTransportError(err))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, providerTransportError(err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, p.providerHTTPError(resp, raw))
	}

	frames, err := sse.ReadAll(strings.NewReader(string(raw)))
	if err != nil {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, providerTransportError(err))
	}
	assembled, err := assemble(frames)
	if err != nil {
		return agentkit.NewRoundTrip(nil, assembled.message, assembled.finish, assembled.usage, nil, err)
	}
	return agentkit.NewRoundTrip(eventsSeq(assembled.events), assembled.message, assembled.finish, assembled.usage, assembled.warnings, nil)
}

// Reasoning exposes OpenAI's static native reasoning vocabulary.
var Reasoning agentkit.ReasoningInspector = reasoningInspector{}

type modelEntry struct {
	Pricing   agentkit.Pricing
	Reasoning agentkit.ReasoningSpec
}

var registry = map[string]modelEntry{
	ModelGPT55Pro: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 30000, CacheReadInput: 30000, Output: 180000,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort", Kind: agentkit.ReasoningEnum,
			Levels: []string{"high", "xhigh"}, Default: agentkit.Level("high"),
		},
	},
	ModelGPT55: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{
			{MinInputTokens: 0, InputUncached: 5000, CacheReadInput: 500, Output: 30000},
			{MinInputTokens: 272001, InputUncached: 10000, CacheReadInput: 1000, Output: 45000},
		}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"none", "low", "medium", "high", "xhigh"},
			Default:    agentkit.Level("medium"),
			CanDisable: true,
		},
	},
	ModelGPT54: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{
			{MinInputTokens: 0, InputUncached: 2500, CacheReadInput: 250, Output: 15000},
			{MinInputTokens: 272001, InputUncached: 5000, CacheReadInput: 500, Output: 22500},
		}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"none", "low", "medium", "high", "xhigh"},
			Default:    agentkit.Level("none"),
			CanDisable: true,
		},
	},
	ModelGPT54Mini: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 750, CacheReadInput: 75, Output: 4500,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"none", "low", "medium", "high", "xhigh"},
			Default:    agentkit.Level("none"),
			CanDisable: true,
		},
	},
	ModelGPT54Nano: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 200, CacheReadInput: 20, Output: 1250,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"none", "low", "medium", "high", "xhigh"},
			Default:    agentkit.Level("none"),
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

type responsesRequest struct {
	Model           string         `json:"model"`
	Stream          bool           `json:"stream"`
	Store           bool           `json:"store"`
	Include         []string       `json:"include"`
	Instructions    string         `json:"instructions,omitempty"`
	Input           []inputItem    `json:"input"`
	Tools           []toolDef      `json:"tools,omitempty"`
	Temperature     *float64       `json:"temperature,omitempty"`
	TopP            *float64       `json:"top_p,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningConf `json:"reasoning,omitempty"`
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

func (p *Provider) buildRequest(req *agentkit.Request) (responsesRequest, error) {
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
	if req.Gen.Reasoning != agentkit.EffortDefault {
		out.Reasoning = &reasoningConf{Effort: openAIReasoningEffort(req.Gen.Reasoning)}
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
			return responsesRequest{}, err
		}
		out.Input = append(out.Input, items...)
	}
	return out, nil
}

func openAIReasoningEffort(effort agentkit.ReasoningEffort) string {
	switch effort {
	case agentkit.EffortLow:
		return "low"
	case agentkit.EffortMedium:
		return "medium"
	case agentkit.EffortHigh, agentkit.EffortMax:
		return "high"
	case agentkit.EffortOff, agentkit.EffortMinimal:
		return "minimal"
	default:
		return ""
	}
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
	events   []agentkit.Event
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
				out.events = append(out.events, agentkit.TextDelta{Text: ev.Delta})
			}
		case "response.reasoning_summary_text.delta":
			if ev.Delta != "" {
				reasoningSummary.WriteString(ev.Delta)
				out.events = append(out.events, agentkit.ReasoningDelta{Text: ev.Delta})
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

func eventsSeq(events []agentkit.Event) iter.Seq[agentkit.Event] {
	return func(yield func(agentkit.Event) bool) {
		for _, ev := range events {
			if !yield(ev) {
				return
			}
		}
	}
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
		Provider:   "openai",
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
