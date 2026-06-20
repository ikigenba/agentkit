// Package anthropic implements the Anthropic Messages API provider.
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/sse"
)

const (
	ModelOpus48   = "claude-opus-4-8"
	ModelSonnet46 = "claude-sonnet-4-6"
	ModelHaiku45  = "claude-haiku-4-5"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"
	defaultMaxOut  = 4096
)

// Reasoning exposes Anthropic's static native reasoning vocabulary.
var Reasoning agentkit.ReasoningInspector = reasoningInspector{}

type modelEntry struct {
	Pricing   agentkit.Pricing
	Reasoning agentkit.ReasoningSpec
}

var registry = map[string]modelEntry{
	ModelOpus48: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 5000, CacheReadInput: 500, CacheWrite5m: 6250, CacheWrite1h: 10000, Output: 25000,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"low", "medium", "high", "xhigh", "max"},
			Default:    agentkit.Level("high"),
			CanDisable: true,
		},
	},
	ModelSonnet46: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 3000, CacheReadInput: 300, CacheWrite5m: 3750, CacheWrite1h: 6000, Output: 15000,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "effort", Kind: agentkit.ReasoningEnum,
			Levels:     []string{"low", "medium", "high", "max"},
			Default:    agentkit.Level("high"),
			CanDisable: true,
		},
	},
	ModelHaiku45: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 1000, CacheReadInput: 100, CacheWrite5m: 1250, CacheWrite1h: 2000, Output: 5000,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking budget", Kind: agentkit.ReasoningRange,
			Min:        1024,
			Max:        defaultMaxOut,
			Default:    agentkit.DisableReasoning(),
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

// Option customizes an Anthropic provider handle.
type Option func(*Provider)

// WithBaseURL points the provider at an alternate API root.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) {
		p.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient sets the HTTP client used for requests.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// Provider is an Anthropic Messages API implementation of agentkit.Provider.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// New constructs an Anthropic provider handle.
func New(apiKey string, opts ...Option) *Provider {
	p := &Provider{apiKey: apiKey, baseURL: defaultBaseURL}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Name() string {
	return "anthropic"
}

func (p *Provider) Pricing(model string) (agentkit.Pricing, bool) {
	entry, ok := registry[model]
	return entry.Pricing, ok
}

func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if _, ok := registry[req.Model]; !ok {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig)
	}
	body, warnings, err := buildRequest(req)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, err)
	}

	endpoint, err := url.JoinPath(p.baseURL, "/v1/messages")
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, err)
	}
	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Anthropic-Version", apiVersion)
	httpReq.Header.Set("X-API-Key", p.apiKey)

	resp, err := httpx.Client(p.client).Do(httpReq)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, classifyTransport(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, classifyHTTP(resp, raw))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, classifyTransport(err))
	}
	message, finish, usage, parseErr := parseStream(raw)
	return agentkit.NewRoundTrip(message, finish, usage, warnings, parseErr)
}

type messageRequest struct {
	Model       string         `json:"model"`
	MaxTokens   int            `json:"max_tokens"`
	System      []wireBlock    `json:"system,omitempty"`
	Messages    []wireMessage  `json:"messages"`
	Tools       []wireTool     `json:"tools,omitempty"`
	Stream      bool           `json:"stream"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	Thinking    map[string]any `json:"thinking,omitempty"`
	Output      map[string]any `json:"output_config,omitempty"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

type wireTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	CacheControl *cacheControl   `json:"cache_control,omitempty"`
}

type wireBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      string          `json:"content,omitempty"`
	IsError      bool            `json:"is_error,omitempty"`
	Signature    string          `json:"signature,omitempty"`
	CacheControl *cacheControl   `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"`
}

func buildRequest(req *agentkit.Request) (messageRequest, []agentkit.Warning, error) {
	maxTokens := req.Gen.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxOut
	}
	out := messageRequest{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		Messages:    make([]wireMessage, 0, len(req.Messages)),
		Tools:       make([]wireTool, 0, len(req.Tools)),
		Stream:      true,
		Temperature: req.Gen.Temperature,
		TopP:        req.Gen.TopP,
	}
	if req.System != "" {
		out.System = []wireBlock{{Type: "text", Text: req.System}}
	}
	for _, tool := range req.Tools {
		out.Tools = append(out.Tools, wireTool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.JSONSchema(),
		})
	}
	for _, msg := range req.Messages {
		wire, err := convertMessage(msg)
		if err != nil {
			return out, nil, err
		}
		out.Messages = append(out.Messages, wire)
	}

	warnings := applyReasoning(req.Model, req.Gen.Reasoning, maxTokens, &out)
	applyCacheControl(req, &out)
	return out, warnings, nil
}

func convertMessage(msg agentkit.Message) (wireMessage, error) {
	role := string(msg.Role)
	blocks := make([]wireBlock, 0, len(msg.Blocks))
	for _, block := range msg.Blocks {
		switch b := block.(type) {
		case agentkit.TextBlock:
			blocks = append(blocks, wireBlock{Type: "text", Text: b.Text})
		case agentkit.ToolUseBlock:
			blocks = append(blocks, wireBlock{Type: "tool_use", ID: b.ID, Name: b.Name, Input: cloneRaw(b.Input)})
		case agentkit.ToolResultBlock:
			blocks = append(blocks, wireBlock{Type: "tool_result", ToolUseID: b.ToolUseID, Content: b.Content, IsError: b.IsError})
		case agentkit.ReasoningBlock:
			signature, ok := anthropicSignature(b.Opaque)
			if ok {
				blocks = append(blocks, wireBlock{Type: "thinking", Thinking: b.Summary, Signature: signature})
			}
		default:
			panic(fmt.Sprintf("unknown block type %T", block))
		}
	}
	return wireMessage{Role: role, Content: blocks}, nil
}

func applyReasoning(model string, value agentkit.ReasoningValue, maxTokens int, out *messageRequest) []agentkit.Warning {
	value, warning := checkedReasoning(model, value)
	if value.IsUnset() {
		return warning
	}
	if value.Disabled() {
		out.Thinking = map[string]any{"type": "disabled"}
		return warning
	}
	if level, ok := value.Level(); ok {
		out.Thinking = map[string]any{"type": "adaptive"}
		out.Output = map[string]any{"effort": level}
		return warning
	}
	if budget, ok := value.Budget(); ok {
		if model == ModelHaiku45 && maxTokens > 0 && budget >= maxTokens {
			budget = maxTokens - 1
		}
		out.Thinking = map[string]any{"type": "enabled", "budget_tokens": budget}
	}
	return warning
}

func checkedReasoning(model string, value agentkit.ReasoningValue) (agentkit.ReasoningValue, []agentkit.Warning) {
	if value.IsUnset() {
		return value, nil
	}
	entry, ok := registry[model]
	if !ok || entry.Reasoning.Accepts(value) {
		return value, nil
	}
	code := agentkit.WarnReasoningUnsupported
	if value.Disabled() && !entry.Reasoning.CanDisable {
		code = agentkit.WarnReasoningCannotDisable
	}
	return entry.Reasoning.Default, []agentkit.Warning{{
		Setting: "reasoning",
		Code:    code,
		Detail:  "requested " + describeReasoning(value) + "; applied " + describeReasoning(entry.Reasoning.Default),
	}}
}

func describeReasoning(value agentkit.ReasoningValue) string {
	if value.IsUnset() {
		return "unset"
	}
	if value.Disabled() {
		return "disabled"
	}
	if level, ok := value.Level(); ok {
		return "level " + level
	}
	if budget, ok := value.Budget(); ok {
		return fmt.Sprintf("budget %d", budget)
	}
	return "unknown"
}

func applyCacheControl(req *agentkit.Request, out *messageRequest) {
	if stablePrefixTokens(req) < cacheMinimum(req.Model) {
		return
	}
	cc := &cacheControl{Type: "ephemeral"}
	if len(req.Messages) > 1 {
		msg := &out.Messages[len(req.Messages)-2]
		if len(msg.Content) > 0 {
			msg.Content[len(msg.Content)-1].CacheControl = cc
			return
		}
	}
	if len(out.Tools) > 0 {
		out.Tools[len(out.Tools)-1].CacheControl = cc
		return
	}
	if len(out.System) > 0 {
		out.System[len(out.System)-1].CacheControl = cc
	}
}

func cacheMinimum(model string) int {
	if model == ModelSonnet46 {
		return 2048
	}
	return 4096
}

func stablePrefixTokens(req *agentkit.Request) int {
	chars := len(req.System)
	for _, tool := range req.Tools {
		chars += len(tool.Name()) + len(tool.Description()) + len(tool.JSONSchema())
	}
	for _, msg := range req.Messages[:max(0, len(req.Messages)-1)] {
		for _, block := range msg.Blocks {
			switch b := block.(type) {
			case agentkit.TextBlock:
				chars += len(b.Text)
			case agentkit.ToolUseBlock:
				chars += len(b.ID) + len(b.Name) + len(b.Input)
			case agentkit.ToolResultBlock:
				chars += len(b.ToolUseID) + len(b.Name) + len(b.Content)
			case agentkit.ReasoningBlock:
				chars += len(b.Opaque) + len(b.Summary)
			}
		}
	}
	return chars / 4
}

type streamContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Signature string          `json:"signature"`
}

type streamEvent struct {
	Type        string          `json:"type"`
	Index       int             `json:"index"`
	Delta       streamDelta     `json:"delta"`
	Content     streamContent   `json:"content_block"`
	Message     streamMessage   `json:"message"`
	Usage       anthropicUsage  `json:"usage"`
	Error       anthropicError  `json:"error"`
	RawType     string          `json:"-"`
	RawEnvelope json.RawMessage `json:"-"`
}

type streamDelta struct {
	Type        string         `json:"type"`
	Text        string         `json:"text"`
	PartialJSON string         `json:"partial_json"`
	Thinking    string         `json:"thinking"`
	Signature   string         `json:"signature"`
	StopReason  string         `json:"stop_reason"`
	Usage       anthropicUsage `json:"usage"`
}

type streamMessage struct {
	Usage anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int64                `json:"input_tokens"`
	OutputTokens             int64                `json:"output_tokens"`
	CacheReadInputTokens     int64                `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64                `json:"cache_creation_input_tokens"`
	CacheCreation            anthropicCacheCreate `json:"cache_creation"`
}

type anthropicCacheCreate struct {
	Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type partialBlock struct {
	typ       string
	id        string
	name      string
	text      strings.Builder
	input     strings.Builder
	thinking  strings.Builder
	signature strings.Builder
}

func parseStream(raw []byte) (agentkit.Message, agentkit.FinishReason, agentkit.Usage, error) {
	frames, err := sse.ReadAll(strings.NewReader(string(raw)))
	if err != nil {
		return agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, classifyTransport(err)
	}
	var (
		blocks []agentkit.Block
		open   = map[int]*partialBlock{}
		usage  agentkit.Usage
		finish = agentkit.FinishOther
	)
	for _, frame := range frames {
		var ev streamEvent
		if err := json.Unmarshal(frame.Data, &ev); err != nil {
			return agentkit.Message{Role: agentkit.RoleAssistant, Blocks: blocks}, finish, usage, classifyTransport(err)
		}
		ev.RawType = frame.Type
		ev.RawEnvelope = cloneRaw(frame.Data)
		if frame.Type == "error" || ev.Type == "error" {
			rawErr := frame.Data
			return agentkit.Message{Role: agentkit.RoleAssistant, Blocks: blocks}, finish, usage, classifyStreamError(rawErr, ev.Error)
		}
		switch ev.Type {
		case "message_start":
			mergeUsage(&usage, ev.Message.Usage)
		case "content_block_start":
			open[ev.Index] = &partialBlock{typ: ev.Content.Type, id: ev.Content.ID, name: ev.Content.Name}
			switch ev.Content.Type {
			case "text":
				open[ev.Index].text.WriteString(ev.Content.Text)
			case "thinking":
				open[ev.Index].thinking.WriteString(ev.Content.Text)
				open[ev.Index].signature.WriteString(ev.Content.Signature)
			}
		case "content_block_delta":
			block := open[ev.Index]
			if block == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				block.text.WriteString(ev.Delta.Text)
			case "input_json_delta":
				block.input.WriteString(ev.Delta.PartialJSON)
			case "thinking_delta":
				block.thinking.WriteString(ev.Delta.Thinking)
			case "signature_delta":
				block.signature.WriteString(ev.Delta.Signature)
			}
		case "content_block_stop":
			block := open[ev.Index]
			delete(open, ev.Index)
			if block == nil {
				continue
			}
			assembled, err := block.assemble()
			if err != nil {
				return agentkit.Message{Role: agentkit.RoleAssistant, Blocks: blocks}, finish, usage, err
			}
			if assembled != nil {
				blocks = append(blocks, assembled)
			}
		case "message_delta":
			mergeUsage(&usage, ev.Usage)
			if ev.Delta.StopReason != "" {
				finish = finishReason(ev.Delta.StopReason)
			}
			if ev.Delta.Usage.OutputTokens != 0 {
				mergeUsage(&usage, ev.Delta.Usage)
			}
		case "message_stop":
			if finish == agentkit.FinishOther {
				finish = agentkit.FinishStop
			}
		}
	}
	finalizeUsage(&usage)
	return agentkit.Message{Role: agentkit.RoleAssistant, Blocks: blocks}, finish, usage, nil
}

func (b *partialBlock) assemble() (agentkit.Block, error) {
	switch b.typ {
	case "text":
		return agentkit.TextBlock{Text: b.text.String()}, nil
	case "tool_use":
		input := json.RawMessage(strings.TrimSpace(b.input.String()))
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		if !json.Valid(input) {
			return nil, fmt.Errorf("%w: invalid Anthropic tool input JSON", agentkit.ErrInvalidRequest)
		}
		return agentkit.ToolUseBlock{ID: b.id, Name: b.name, Input: cloneRaw(input)}, nil
	case "thinking":
		signature := b.signature.String()
		if signature == "" {
			return agentkit.ReasoningBlock{Summary: b.thinking.String()}, nil
		}
		opaque, _ := json.Marshal(map[string]string{"signature": signature})
		return agentkit.ReasoningBlock{Opaque: opaque, Summary: b.thinking.String()}, nil
	default:
		return nil, nil
	}
}

func mergeUsage(usage *agentkit.Usage, au anthropicUsage) {
	if au.InputTokens != 0 {
		usage.InputUncached = au.InputTokens
	}
	if au.CacheReadInputTokens != 0 {
		usage.CacheReadInput = au.CacheReadInputTokens
	}
	if au.CacheCreationInputTokens != 0 {
		usage.CacheWriteInput = au.CacheCreationInputTokens
	}
	if au.CacheCreation.Ephemeral5mInputTokens != 0 {
		usage.CacheWrite5m = au.CacheCreation.Ephemeral5mInputTokens
	}
	if au.CacheCreation.Ephemeral1hInputTokens != 0 {
		usage.CacheWrite1h = au.CacheCreation.Ephemeral1hInputTokens
	}
	if au.OutputTokens != 0 {
		usage.Output = au.OutputTokens
	}
}

func finalizeUsage(usage *agentkit.Usage) {
	if usage.CacheWriteInput == 0 {
		usage.CacheWriteInput = usage.CacheWrite5m + usage.CacheWrite1h
	}
	if usage.CacheWriteInput != 0 && usage.CacheWrite5m == 0 && usage.CacheWrite1h == 0 {
		usage.CacheWrite5m = usage.CacheWriteInput
	}
	usage.ReasoningOutput = 0
	usage.Total = usage.InputUncached + usage.CacheReadInput + usage.CacheWriteInput + usage.Output + usage.ReasoningOutput
}

func finishReason(reason string) agentkit.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence":
		return agentkit.FinishStop
	case "tool_use":
		return agentkit.FinishToolUse
	case "max_tokens":
		return agentkit.FinishMaxTokens
	case "refusal":
		return agentkit.FinishContentFilter
	default:
		return agentkit.FinishOther
	}
}

func classifyTransport(err error) error {
	if err == nil {
		return nil
	}
	category := agentkit.ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = agentkit.ErrTimeout
	}
	return &agentkit.Error{Category: category, Provider: "anthropic", Message: err.Error(), Err: err}
}

func classifyHTTP(resp *http.Response, raw []byte) error {
	var envelope struct {
		Type  string         `json:"type"`
		Error anthropicError `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	typ := envelope.Error.Type
	msg := envelope.Error.Message
	if typ == "" {
		typ = envelope.Type
	}
	category := classifyStatusType(resp.StatusCode, typ, msg)
	return &agentkit.Error{
		Category:   category,
		Provider:   "anthropic",
		StatusCode: resp.StatusCode,
		Type:       typ,
		Message:    msg,
		RequestID:  resp.Header.Get("request-id"),
		RetryAfter: httpx.RetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		Raw:        cloneRaw(raw),
	}
}

func classifyStreamError(raw []byte, ae anthropicError) error {
	return &agentkit.Error{
		Category: classifyStatusType(0, ae.Type, ae.Message),
		Provider: "anthropic",
		Type:     ae.Type,
		Message:  ae.Message,
		Raw:      cloneRaw(raw),
	}
}

func classifyStatusType(status int, typ, message string) error {
	switch {
	case status == http.StatusUnauthorized || typ == "authentication_error":
		return agentkit.ErrAuthentication
	case status == http.StatusForbidden || typ == "permission_error":
		return agentkit.ErrPermission
	case status == http.StatusNotFound:
		return agentkit.ErrNotFound
	case status == http.StatusTooManyRequests || typ == "rate_limit_error":
		return agentkit.ErrRateLimited
	case status == 529 || typ == "overloaded_error":
		return agentkit.ErrOverloaded
	case status == http.StatusInternalServerError || typ == "api_error":
		return agentkit.ErrServerError
	case status == http.StatusGatewayTimeout || typ == "timeout_error":
		return agentkit.ErrTimeout
	case status == http.StatusPaymentRequired || typ == "billing_error":
		return agentkit.ErrBilling
	case typ == "context_length_exceeded" || strings.Contains(strings.ToLower(message), "context"):
		return agentkit.ErrContextLength
	case status == http.StatusBadRequest || status == http.StatusRequestEntityTooLarge || typ == "invalid_request_error":
		return agentkit.ErrInvalidRequest
	default:
		return agentkit.ErrUnknown
	}
}

func anthropicSignature(raw json.RawMessage) (string, bool) {
	var envelope struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Signature == "" {
		return "", false
	}
	return envelope.Signature, true
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
