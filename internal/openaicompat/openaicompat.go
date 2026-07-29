// Package openaicompat implements the shared OpenAI Chat-Completions wire
// adapter used by first-class providers that expose that protocol.
package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/sse"
)

// ErrorClassifier maps provider-specific status/code/message details to an
// AgentKit sentinel category.
type ErrorClassifier func(status int, code, message string) error

// ReasoningEncoder lowers a provider-neutral reasoning value to top-level
// Chat-Completions request fields. The returned JSON must be an object.
type ReasoningEncoder func(agentkit.ReasoningValue) (json.RawMessage, error)

// CostExtractor extracts a provider-reported nano-USD cost from a raw usage
// object. ok is false when the provider did not report a cost.
type CostExtractor func(json.RawMessage) (cost agentkit.Cost, ok bool)

// Config describes one first-class OpenAI-compatible provider.
type Config struct {
	Identity                 agentkit.Identity
	BaseURL                  string
	APIKey                   string
	HTTPClient               *http.Client
	Now                      func() time.Time
	Classify                 ErrorClassifier
	WarnForcedToolChoiceAuto bool
	ReasoningEncoder         ReasoningEncoder
	CostExtractor            CostExtractor
}

// Provider implements agentkit.Provider for an OpenAI Chat-Completions wire.
type Provider struct {
	cfg Config
}

// New constructs an OpenAI-compatible provider.
func New(cfg Config) *Provider {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Provider{cfg: cfg}
}

// Identity identifies the provider package and credential mode.
func (p *Provider) Identity() agentkit.Identity {
	return p.cfg.Identity
}

// RoundTrip performs one Chat-Completions model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if p == nil || p.cfg.APIKey == "" || req == nil || p.cfg.BaseURL == "" {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig, 0, false)
	}

	body, warnings, err := p.buildRequest(req)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, err, 0, false)
	}

	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, p.cfg.BaseURL+"/chat/completions", body)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.transportError(err), 0, false)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := httpx.Client(p.cfg.HTTPClient).Do(httpReq)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.transportError(err), 0, false)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.transportError(err), 0, false)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.httpError(resp, raw), 0, false)
	}

	frames, err := sse.ReadAll(strings.NewReader(string(raw)))
	if err != nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, p.transportError(err), 0, false)
	}
	assembled, err := p.assemble(frames)
	if err != nil {
		return agentkit.NewRoundTrip(assembled.message, assembled.finish, assembled.usage, warnings, err, assembled.reportedCost, assembled.reportedCostOK)
	}
	return agentkit.NewRoundTrip(assembled.message, assembled.finish, assembled.usage, warnings, nil, assembled.reportedCost, assembled.reportedCostOK)
}

type chatRequest struct {
	Model           string                     `json:"model"`
	Messages        []chatMessage              `json:"messages"`
	Stream          bool                       `json:"stream"`
	StreamOptions   streamOptions              `json:"stream_options"`
	Tools           []toolDef                  `json:"tools,omitempty"`
	ToolChoice      string                     `json:"tool_choice,omitempty"`
	Temperature     *float64                   `json:"temperature,omitempty"`
	TopP            *float64                   `json:"top_p,omitempty"`
	MaxTokens       int                        `json:"max_tokens,omitempty"`
	Thinking        *thinkingConf              `json:"thinking,omitempty"`
	Reasoning       string                     `json:"reasoning_effort,omitempty"`
	ReasoningFields map[string]json.RawMessage `json:"-"`
	ProviderOptions map[string]json.RawMessage `json:"-"`
}

func (r chatRequest) MarshalJSON() ([]byte, error) {
	type wireRequest chatRequest
	raw, err := json.Marshal(wireRequest(r))
	if err != nil {
		return nil, err
	}
	if len(r.ReasoningFields) == 0 && len(r.ProviderOptions) == 0 {
		return raw, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	for key, value := range r.ReasoningFields {
		body[key] = value
	}
	for key, value := range r.ProviderOptions {
		body[key] = value
	}
	return json.Marshal(body)
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type thinkingConf struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type toolDef struct {
	Type     string          `json:"type"`
	Function toolFunctionDef `json:"function"`
}

type toolFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (p *Provider) buildRequest(req *agentkit.Request) (chatRequest, []agentkit.Warning, error) {
	out := chatRequest{
		Model:         req.Model,
		Stream:        true,
		StreamOptions: streamOptions{IncludeUsage: true},
		Messages:      make([]chatMessage, 0, len(req.Messages)+1),
		Temperature:   req.Gen.Temperature,
		TopP:          req.Gen.TopP,
	}
	if req.Gen.MaxTokens > 0 {
		out.MaxTokens = req.Gen.MaxTokens
	}
	var warnings []agentkit.Warning
	if err := p.applyReasoning(req.Gen.Reasoning, &out); err != nil {
		return chatRequest{}, warnings, err
	}
	if len(req.ProviderOptions) != 0 {
		if err := json.Unmarshal(req.ProviderOptions, &out.ProviderOptions); err != nil || out.ProviderOptions == nil {
			return chatRequest{}, warnings, fmt.Errorf("OpenAI-compatible provider options must be a JSON object: %w", agentkit.ErrInvalidConfig)
		}
	}
	if req.System != "" {
		out.Messages = append(out.Messages, chatMessage{Role: "system", Content: req.System})
	}
	for _, tool := range req.Tools {
		out.Tools = append(out.Tools, toolDef{
			Type: "function",
			Function: toolFunctionDef{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.JSONSchema(),
			},
		})
	}
	if len(out.Tools) > 0 {
		out.ToolChoice = "auto"
		if p.cfg.WarnForcedToolChoiceAuto {
			warnings = append(warnings, agentkit.Warning{
				Setting: "tool_choice",
				Code:    agentkit.WarnToolChoiceForced,
				Detail:  "requested forced tool choice; applied auto because provider supports only auto",
			})
		}
	}
	for _, message := range req.Messages {
		converted, err := convertMessage(message)
		if err != nil {
			return chatRequest{}, warnings, err
		}
		out.Messages = append(out.Messages, converted...)
	}
	return out, warnings, nil
}

func (p *Provider) applyReasoning(value agentkit.ReasoningValue, out *chatRequest) error {
	if p.cfg.ReasoningEncoder != nil {
		raw, err := p.cfg.ReasoningEncoder(value)
		if err != nil {
			return err
		}
		if len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, &out.ReasoningFields); err != nil || out.ReasoningFields == nil {
			return fmt.Errorf("OpenAI-compatible reasoning encoder must return a JSON object: %w", agentkit.ErrInvalidConfig)
		}
		return nil
	}
	if value.IsUnset() {
		return nil
	}
	if value.Disabled() {
		out.Thinking = &thinkingConf{Type: "disabled"}
		return nil
	}
	if level, ok := value.Level(); ok {
		out.Thinking = &thinkingConf{Type: "enabled"}
		out.Reasoning = level
		return nil
	}
	if _, ok := value.Budget(); ok {
		return fmt.Errorf("OpenAI-compatible Chat Completions API cannot encode token-budget reasoning: %w", agentkit.ErrInvalidConfig)
	}
	return fmt.Errorf("unknown OpenAI-compatible reasoning value: %w", agentkit.ErrInvalidConfig)
}

func convertMessage(message agentkit.Message) ([]chatMessage, error) {
	var text strings.Builder
	var reasoning string
	var calls []toolCall
	var out []chatMessage

	flushAssistant := func() {
		if text.Len() == 0 && reasoning == "" && len(calls) == 0 {
			return
		}
		out = append(out, chatMessage{
			Role:             "assistant",
			Content:          text.String(),
			ReasoningContent: reasoning,
			ToolCalls:        calls,
		})
		text.Reset()
		reasoning = ""
		calls = nil
	}

	for _, block := range message.Blocks {
		switch b := block.(type) {
		case agentkit.TextBlock:
			text.WriteString(b.Text)
		case agentkit.ReasoningBlock:
			if value, ok := reasoningContent(b.Opaque); ok {
				reasoning += value
			}
		case agentkit.ToolUseBlock:
			calls = append(calls, toolCall{
				ID:   b.ID,
				Type: "function",
				Function: toolFunction{
					Name:      b.Name,
					Arguments: toolArgumentsString(b.Input),
				},
			})
		case agentkit.ToolResultBlock:
			if message.Role == agentkit.RoleAssistant {
				return nil, agentkit.ErrInvalidConfig
			}
			if text.Len() > 0 {
				out = append(out, chatMessage{Role: string(message.Role), Content: text.String()})
				text.Reset()
			}
			out = append(out, chatMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: b.Content})
		default:
			panic(fmt.Sprintf("unknown block type %T", block))
		}
	}
	if message.Role == agentkit.RoleAssistant {
		flushAssistant()
		return out, nil
	}
	if text.Len() > 0 {
		out = append(out, chatMessage{Role: string(message.Role), Content: text.String()})
	}
	return out, nil
}

func toolArgumentsString(raw json.RawMessage) string {
	if strings.TrimSpace(string(raw)) == "" || !json.Valid(raw) {
		return "{}"
	}
	return string(raw)
}

func reasoningContent(raw json.RawMessage) (string, bool) {
	var envelope struct {
		ReasoningContent string `json:"reasoning_content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.ReasoningContent == "" {
		return "", false
	}
	return envelope.ReasoningContent, true
}

type assembledRoundTrip struct {
	message        agentkit.Message
	finish         agentkit.FinishReason
	usage          agentkit.Usage
	reportedCost   agentkit.Cost
	reportedCostOK bool
}

type streamChunk struct {
	Choices []choice        `json:"choices"`
	Usage   json.RawMessage `json:"usage"`
	Error   errorPayload    `json:"error"`
}

type choice struct {
	Delta        delta  `json:"delta"`
	FinishReason string `json:"finish_reason"`
}

type delta struct {
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content"`
	ToolCalls        []toolCallDelta `json:"tool_calls"`
}

type toolCallDelta struct {
	Index    int               `json:"index"`
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function toolFunctionDelta `json:"function"`
}

type toolFunctionDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type usagePayload struct {
	PromptTokens        int64              `json:"prompt_tokens"`
	CompletionTokens    int64              `json:"completion_tokens"`
	TotalTokens         int64              `json:"total_tokens"`
	PromptTokensDetails promptTokenDetails `json:"prompt_tokens_details"`
}

type promptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type errorPayload struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

type partialTool struct {
	id   string
	name string
	args strings.Builder
}

func (p *Provider) assemble(frames []sse.Event) (assembledRoundTrip, error) {
	out := assembledRoundTrip{
		message: agentkit.Message{Role: agentkit.RoleAssistant},
		finish:  agentkit.FinishStop,
	}
	tools := make(map[int]*partialTool)
	var visible strings.Builder
	var reasoning strings.Builder

	for _, frame := range frames {
		if string(frame.Data) == "[DONE]" {
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal(frame.Data, &chunk); err != nil {
			return out, p.transportError(err)
		}
		if chunk.Error.Message != "" || len(chunk.Error.Code) != 0 {
			raw := cloneRaw(frame.Data)
			return out, p.errorFromPayload(0, raw, chunk.Error, "", "")
		}
		if len(chunk.Usage) != 0 && string(chunk.Usage) != "null" {
			var native usagePayload
			if err := json.Unmarshal(chunk.Usage, &native); err != nil {
				return out, p.transportError(err)
			}
			usage, err := p.mapUsage(native)
			if err != nil {
				return out, err
			}
			out.usage = usage
			if p.cfg.CostExtractor != nil {
				out.reportedCost, out.reportedCostOK = p.cfg.CostExtractor(chunk.Usage)
			}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				visible.WriteString(choice.Delta.Content)
			}
			if choice.Delta.ReasoningContent != "" {
				reasoning.WriteString(choice.Delta.ReasoningContent)
			}
			for _, call := range choice.Delta.ToolCalls {
				tool := tools[call.Index]
				if tool == nil {
					tool = &partialTool{}
					tools[call.Index] = tool
				}
				if call.ID != "" {
					tool.id = call.ID
				}
				if call.Function.Name != "" {
					tool.name = call.Function.Name
				}
				tool.args.WriteString(call.Function.Arguments)
			}
			if choice.FinishReason != "" {
				out.finish = finishFromReason(choice.FinishReason)
			}
		}
	}

	if reasoning.Len() > 0 {
		opaque, _ := json.Marshal(map[string]string{"reasoning_content": reasoning.String()})
		out.message.Blocks = append(out.message.Blocks, agentkit.ReasoningBlock{
			Opaque:  opaque,
			Summary: reasoning.String(),
		})
	}
	if visible.Len() > 0 {
		out.message.Blocks = append(out.message.Blocks, agentkit.TextBlock{Text: visible.String()})
	}
	if len(tools) > 0 {
		indexes := make([]int, 0, len(tools))
		for index := range tools {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		for _, index := range indexes {
			tool := tools[index]
			out.message.Blocks = append(out.message.Blocks, tool.block())
		}
		out.finish = agentkit.FinishToolUse
	}
	if hasToolUse(out.message) {
		out.finish = agentkit.FinishToolUse
	}
	return out, nil
}

func (t *partialTool) block() agentkit.ToolUseBlock {
	input := json.RawMessage(strings.TrimSpace(t.args.String()))
	if len(input) == 0 || !json.Valid(input) {
		input = json.RawMessage(`{}`)
	}
	return agentkit.ToolUseBlock{
		ID:    agentkit.NewToolUseID(),
		Name:  t.name,
		Input: cloneRaw(input),
	}
}

func finishFromReason(reason string) agentkit.FinishReason {
	switch reason {
	case "stop":
		return agentkit.FinishStop
	case "tool_calls", "function_call":
		return agentkit.FinishToolUse
	case "length":
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

func (p *Provider) mapUsage(native usagePayload) (agentkit.Usage, error) {
	cached := native.PromptTokensDetails.CachedTokens
	if cached > native.PromptTokens {
		return agentkit.Usage{}, &agentkit.Error{
			Category: agentkit.ErrUnknown,
			Provider: p.cfg.Identity.Provider,
			Auth:     p.cfg.Identity.Auth,
			Message:  "provider usage cached tokens exceed prompt tokens",
		}
	}
	usage := agentkit.Usage{
		InputUncached:   native.PromptTokens - cached,
		CacheReadInput:  cached,
		CacheWriteInput: 0,
		CacheWrite5m:    0,
		CacheWrite1h:    0,
		Output:          native.CompletionTokens,
		ReasoningOutput: 0,
	}
	usage.Total = usage.InputUncached + usage.CacheReadInput + usage.CacheWriteInput + usage.Output + usage.ReasoningOutput
	if native.TotalTokens != 0 && native.TotalTokens != usage.Total {
		return agentkit.Usage{}, &agentkit.Error{
			Category: agentkit.ErrUnknown,
			Provider: p.cfg.Identity.Provider,
			Auth:     p.cfg.Identity.Auth,
			Message:  "provider usage total does not equal mapped buckets",
		}
	}
	return usage, nil
}

func (p *Provider) transportError(err error) error {
	category := agentkit.ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = agentkit.ErrTimeout
	}
	return &agentkit.Error{
		Category: category,
		Provider: p.cfg.Identity.Provider,
		Auth:     p.cfg.Identity.Auth,
		Message:  err.Error(),
		Err:      err,
	}
}

func (p *Provider) httpError(resp *http.Response, raw []byte) error {
	var envelope struct {
		Error errorPayload `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return p.errorFromPayload(resp.StatusCode, raw, envelope.Error, resp.Header.Get("x-request-id"), resp.Header.Get("Retry-After"))
}

func (p *Provider) errorFromPayload(status int, raw []byte, payload errorPayload, requestID, retryAfter string) error {
	code := rawString(payload.Code)
	message := payload.Message
	if message == "" {
		message = string(raw)
	}
	category := agentkit.ErrUnknown
	if p.cfg.Classify != nil {
		category = p.cfg.Classify(status, code, message)
	}
	return &agentkit.Error{
		Category:   category,
		Provider:   p.cfg.Identity.Provider,
		Auth:       p.cfg.Identity.Auth,
		StatusCode: status,
		Type:       code,
		Message:    message,
		RequestID:  requestID,
		RetryAfter: httpx.RetryAfter(retryAfter, p.cfg.Now()),
		Raw:        cloneRaw(raw),
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
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return string(raw)
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
