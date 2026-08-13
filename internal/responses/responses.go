// Package responses implements the shared, non-public Responses API wire.
package responses

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
	"github.com/ikigenba/agentkit/internal/openaicompat"
	"github.com/ikigenba/agentkit/internal/sse"
)

// Config describes one provider's Responses endpoint and authentication.
type Config struct {
	Identity      agentkit.Identity
	BaseURL       string
	Path          string
	ExtraHeaders  http.Header
	Bearer        func(context.Context) (string, error)
	HTTPClient    *http.Client
	Classify      func(status int, code, message string) error
	CostExtractor func(usage json.RawMessage) (agentkit.Cost, bool)
	Now           func() time.Time

	// AllowBareReasoning encodes EnableReasoning as omission. OpenAI leaves it
	// false; providers whose models have no effort field may enable it.
	AllowBareReasoning bool
}

// Provider performs calls against a Configured Responses API endpoint.
type Provider struct{ cfg Config }

// New constructs a shared Responses provider.
func New(cfg Config) *Provider {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	cfg.ExtraHeaders = cfg.ExtraHeaders.Clone()
	return &Provider{cfg: cfg}
}

// Identity identifies the configured provider and authentication mode.
func (p *Provider) Identity() agentkit.Identity {
	if p == nil {
		return agentkit.Identity{}
	}
	return p.cfg.Identity
}

// RoundTrip performs one Responses API model call.
func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if p == nil || req == nil {
		return failed(nil, agentkit.ErrInvalidConfig)
	}
	if p.cfg.Bearer == nil {
		return failed(nil, p.label(agentkit.ErrMissingCredential))
	}
	bearer, err := p.cfg.Bearer(ctx)
	if err != nil {
		return failed(nil, p.transportError(err))
	}
	if bearer == "" {
		return failed(nil, p.label(agentkit.ErrInvalidConfig))
	}
	body, warnings, err := BuildRequest(req, p.cfg.AllowBareReasoning)
	if err != nil {
		return failed(warnings, p.label(err))
	}
	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, p.cfg.BaseURL+p.cfg.Path, body)
	if err != nil {
		return failed(warnings, p.transportError(err))
	}
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	httpReq.Header.Set("Accept", "text/event-stream")
	for name, values := range p.cfg.ExtraHeaders {
		for _, value := range values {
			httpReq.Header.Add(name, value)
		}
	}
	resp, err := httpx.Client(p.cfg.HTTPClient).Do(httpReq)
	if err != nil {
		return failed(warnings, p.transportError(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return failed(warnings, p.transportError(err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return failed(warnings, p.httpError(resp, raw))
	}
	frames, err := sse.ReadAll(strings.NewReader(string(raw)))
	if err != nil {
		return failed(warnings, p.transportError(err))
	}
	assembled, err := assemble(frames, p.cfg.CostExtractor)
	if err != nil {
		return agentkit.NewRoundTrip(assembled.message, assembled.finish, assembled.usage, warnings, p.label(err), 0, false)
	}
	warnings = append(warnings, assembled.warnings...)
	return agentkit.NewRoundTrip(assembled.message, assembled.finish, assembled.usage, warnings, nil, assembled.cost, assembled.reportedCost)
}

func failed(warnings []agentkit.Warning, err error) *agentkit.RoundTrip {
	return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, warnings, err, 0, false)
}

// Request is the Responses request body.
type Request struct {
	Model           string                     `json:"model"`
	Stream          bool                       `json:"stream"`
	Store           bool                       `json:"store"`
	Include         []string                   `json:"include"`
	Instructions    string                     `json:"instructions,omitempty"`
	Input           []inputItem                `json:"input"`
	Tools           []ToolDef                  `json:"tools,omitempty"`
	Temperature     *float64                   `json:"temperature,omitempty"`
	TopP            *float64                   `json:"top_p,omitempty"`
	MaxOutputTokens int                        `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningConf             `json:"reasoning,omitempty"`
	ProviderOptions map[string]json.RawMessage `json:"-"`
}

func (r Request) MarshalJSON() ([]byte, error) {
	type wire Request
	raw, err := json.Marshal(wire(r))
	if err != nil || len(r.ProviderOptions) == 0 {
		return raw, err
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
type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type summaryPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
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

// ToolDef is a Responses function tool definition.
type ToolDef struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// BuildRequest lowers an AgentKit request into the shared Responses wire.
func BuildRequest(req *agentkit.Request, allowBareReasoning bool) (Request, []agentkit.Warning, error) {
	out := Request{Model: req.Model, Stream: true, Store: false, Include: []string{"reasoning.encrypted_content"}, Input: make([]inputItem, 0, len(req.Messages))}
	out.Instructions = req.System
	out.Temperature = req.Gen.Temperature
	out.TopP = req.Gen.TopP
	if req.Gen.MaxTokens > 0 {
		out.MaxOutputTokens = req.Gen.MaxTokens
	}
	warnings, err := applyReasoning(req.Gen.Reasoning, &out, allowBareReasoning)
	if err != nil {
		return Request{}, warnings, err
	}
	if len(req.ProviderOptions) != 0 {
		if err := json.Unmarshal(req.ProviderOptions, &out.ProviderOptions); err != nil || out.ProviderOptions == nil {
			return Request{}, warnings, fmt.Errorf("Responses provider options must be a JSON object: %w", agentkit.ErrInvalidConfig)
		}
	}
	tools := append([]agentkit.Tool(nil), req.Tools...)
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
	for _, tool := range tools {
		out.Tools = append(out.Tools, ToolDef{Type: "function", Name: tool.Name(), Description: tool.Description(), Parameters: openaicompat.RenderSchema(tool.JSONSchema())})
	}
	for _, message := range req.Messages {
		items, err := messageInputItems(message)
		if err != nil {
			return Request{}, warnings, err
		}
		out.Input = append(out.Input, items...)
	}
	return out, warnings, nil
}

func applyReasoning(value agentkit.ReasoningValue, out *Request, allowBare bool) ([]agentkit.Warning, error) {
	if value.IsUnset() {
		return nil, nil
	}
	if value.Enabled() {
		if allowBare {
			return nil, nil
		}
		return nil, fmt.Errorf("Responses API has no bare reasoning-on form: %w", agentkit.ErrInvalidConfig)
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
		return nil, fmt.Errorf("Responses API cannot encode token-budget reasoning: %w", agentkit.ErrInvalidConfig)
	}
	return nil, fmt.Errorf("unknown Responses reasoning value: %w", agentkit.ErrInvalidConfig)
}

func messageInputItems(message agentkit.Message) ([]inputItem, error) {
	items := make([]inputItem, 0, len(message.Blocks))
	var textParts []contentPart
	flushText := func() {
		if len(textParts) == 0 {
			return
		}
		items = append(items, inputItem{Role: string(message.Role), Content: textParts})
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
			items = append(items, inputItem{Type: "function_call", CallID: block.ID, Name: block.Name, Arguments: string(block.Input)})
		case agentkit.ToolResultBlock:
			flushText()
			items = append(items, inputItem{Type: "function_call_output", CallID: block.ToolUseID, Output: block.Content})
		case agentkit.ReasoningBlock:
			flushText()
			if item, ok := reasoningItem(block); ok {
				items = append(items, item)
			}
		default:
			panic(fmt.Sprintf("unknown block type %T", block))
		}
	}
	flushText()
	return items, nil
}

func reasoningItem(block agentkit.ReasoningBlock) (inputItem, bool) {
	var payload struct {
		Type             string `json:"type"`
		EncryptedContent string `json:"encrypted_content"`
	}
	if json.Unmarshal(block.Opaque, &payload) != nil || payload.Type != "reasoning" || payload.EncryptedContent == "" {
		return inputItem{}, false
	}
	item := inputItem{Type: "reasoning", EncryptedContent: payload.EncryptedContent, Summary: []summaryPart{}}
	if block.Summary != "" {
		item.Summary = []summaryPart{{Type: "summary_text", Text: block.Summary}}
	}
	return item, true
}

type assembledRoundTrip struct {
	message      agentkit.Message
	finish       agentkit.FinishReason
	usage        agentkit.Usage
	warnings     []agentkit.Warning
	cost         agentkit.Cost
	reportedCost bool
}
type responseEvent struct {
	Type        string          `json:"type"`
	Delta       string          `json:"delta"`
	ItemID      string          `json:"item_id"`
	OutputIndex int             `json:"output_index"`
	Arguments   string          `json:"arguments"`
	Response    responsePayload `json:"response"`
	Item        outputItem      `json:"item"`
}
type responsePayload struct {
	Status            string          `json:"status"`
	IncompleteDetails incompleteInfo  `json:"incomplete_details"`
	Usage             json.RawMessage `json:"usage"`
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
	EncryptedContent string        `json:"encrypted_content"`
	Summary          []summaryPart `json:"summary"`
}

// UsagePayload is the common Responses usage shape.
type UsagePayload struct {
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

func assemble(frames []sse.Event, extractCost func(json.RawMessage) (agentkit.Cost, bool)) (assembledRoundTrip, error) {
	out := assembledRoundTrip{message: agentkit.Message{Role: agentkit.RoleAssistant}, finish: agentkit.FinishStop}
	functions := make(map[string]*partialFunction)
	var reasonings []agentkit.ReasoningBlock
	var visibleText, reasoningSummary strings.Builder
	for _, frame := range frames {
		if string(frame.Data) == "[DONE]" {
			continue
		}
		var ev responseEvent
		if err := json.Unmarshal(frame.Data, &ev); err != nil {
			return out, transportError(err)
		}
		typ := ev.Type
		if typ == "" {
			typ = frame.Type
		}
		switch typ {
		case "response.output_text.delta":
			visibleText.WriteString(ev.Delta)
		case "response.reasoning_summary_text.delta":
			reasoningSummary.WriteString(ev.Delta)
		case "response.output_item.added":
			if ev.Item.Type == "function_call" {
				functions[itemKey(ev)] = &partialFunction{callID: ev.Item.CallID, name: ev.Item.Name}
			}
		case "response.function_call_arguments.delta":
			ensureFunction(functions, itemKey(ev)).args.WriteString(ev.Delta)
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
					}{"reasoning", ev.Item.EncryptedContent})
					if err != nil {
						return out, transportError(err)
					}
					reasonings = append(reasonings, agentkit.ReasoningBlock{Opaque: opaque, Summary: summaryText(ev.Item.Summary, reasoningSummary.String())})
				}
			}
		case "response.completed":
			var native UsagePayload
			if len(ev.Response.Usage) != 0 {
				if err := json.Unmarshal(ev.Response.Usage, &native); err != nil {
					return out, transportError(err)
				}
			}
			usage, err := MapUsage(native)
			if err != nil {
				return out, err
			}
			out.usage = usage
			out.finish = finishFromResponse(ev.Response)
			if extractCost != nil {
				out.cost, out.reportedCost = extractCost(ev.Response.Usage)
			}
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
		out.message.Blocks = append(blocks, out.message.Blocks...)
	}
	for _, fn := range functions {
		out.message.Blocks = append(out.message.Blocks, functionBlock(fn))
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
	return agentkit.ToolUseBlock{ID: agentkit.NewToolUseID(), Name: fn.name, Input: append(json.RawMessage(nil), input...)}
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

// MapUsage maps common Responses usage into disjoint AgentKit buckets.
func MapUsage(native UsagePayload) (agentkit.Usage, error) {
	cached := native.InputTokensDetails.CachedTokens
	reasoning := native.OutputTokensDetails.ReasoningTokens
	if cached > native.InputTokens || reasoning > native.OutputTokens {
		return agentkit.Usage{}, &agentkit.Error{Category: agentkit.ErrUnknown, Message: "provider usage details exceed native totals"}
	}
	usage := agentkit.Usage{InputUncached: native.InputTokens - cached, CacheReadInput: cached, Output: native.OutputTokens - reasoning, ReasoningOutput: reasoning}
	usage.Total = usage.InputUncached + usage.CacheReadInput + usage.CacheWriteInput + usage.Output + usage.ReasoningOutput
	if native.TotalTokens != 0 && native.TotalTokens != usage.Total {
		return agentkit.Usage{}, &agentkit.Error{Category: agentkit.ErrUnknown, Message: "provider usage total does not equal mapped buckets"}
	}
	return usage, nil
}
func finishFromResponse(resp responsePayload) agentkit.FinishReason {
	if resp.IncompleteDetails.Reason != "" {
		return finishFromIncomplete(resp.IncompleteDetails)
	}
	if resp.Status == "incomplete" {
		return agentkit.FinishMaxTokens
	}
	return agentkit.FinishStop
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

func transportError(err error) error {
	category := agentkit.ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = agentkit.ErrTimeout
	}
	return &agentkit.Error{Category: category, Err: err, Message: err.Error()}
}
func (p *Provider) transportError(err error) error { return p.label(transportError(err)) }
func (p *Provider) label(err error) error {
	var providerErr *agentkit.Error
	if !errors.As(err, &providerErr) {
		if errors.Is(err, agentkit.ErrInvalidConfig) || errors.Is(err, agentkit.ErrMissingCredential) {
			return err
		}
		return err
	}
	copy := *providerErr
	copy.Provider, copy.Auth = p.cfg.Identity.Provider, p.cfg.Identity.Auth
	return &copy
}
func (p *Provider) httpError(resp *http.Response, raw []byte) error {
	var envelope struct {
		Error struct {
			Message string          `json:"message"`
			Type    string          `json:"type"`
			Code    json.RawMessage `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	code := rawString(envelope.Error.Code)
	message := envelope.Error.Message
	if message == "" {
		message = string(raw)
	}
	typ := envelope.Error.Type
	if typ == "" {
		typ = code
	} else if code != "" {
		typ += ":" + code
	}
	category := agentkit.ErrUnknown
	if p.cfg.Classify != nil {
		classifierCode := code
		if classifierCode == "" {
			classifierCode = envelope.Error.Type
		}
		category = p.cfg.Classify(resp.StatusCode, classifierCode, message)
	}
	return &agentkit.Error{Category: category, Provider: p.cfg.Identity.Provider, Auth: p.cfg.Identity.Auth, StatusCode: resp.StatusCode, Type: typ, Message: message, RequestID: resp.Header.Get("x-request-id"), RetryAfter: httpx.RetryAfter(resp.Header.Get("Retry-After"), p.cfg.Now()), Raw: append(json.RawMessage(nil), raw...)}
}
func rawString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
