// Package google implements the AgentKit provider SPI for Google's Gemini API.
package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/sse"
)

const (
	ModelFlash25 = "gemini-2.5-flash"
	ModelPro25   = "gemini-2.5-pro"
	ModelFlash35 = "gemini-3.5-flash"
	ModelLite31  = "gemini-3.1-flash-lite"
	// ModelPro31Preview is Google's preview-channel 3.x Pro reasoning model.
	ModelPro31Preview = "gemini-3.1-pro-preview"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com"

var modelPricing = map[string]agentkit.Pricing{
	ModelFlash25: {Tiers: []agentkit.RateTier{{
		MinInputTokens: 0, InputUncached: 300, CacheReadInput: 30, Output: 2500,
	}}},
	ModelPro25: {Tiers: []agentkit.RateTier{{
		MinInputTokens: 0, InputUncached: 1250, CacheReadInput: 125, Output: 10000,
	}, {
		MinInputTokens: 200001, InputUncached: 2500, CacheReadInput: 250, Output: 15000,
	}}},
	ModelFlash35: {Tiers: []agentkit.RateTier{{
		MinInputTokens: 0, InputUncached: 1500, CacheReadInput: 150, Output: 9000,
	}}},
	ModelLite31: {Tiers: []agentkit.RateTier{{
		MinInputTokens: 0, InputUncached: 250, CacheReadInput: 25, Output: 1500,
	}}},
	ModelPro31Preview: {Tiers: []agentkit.RateTier{{
		MinInputTokens: 0, InputUncached: 2000, CacheReadInput: 200, Output: 12000,
	}, {
		MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 400, Output: 18000,
	}}},
}

// Option configures a Gemini provider.
type Option func(*Provider)

// WithBaseURL overrides the Gemini API base URL.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) {
		p.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient overrides the HTTP client used by the provider.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// Provider implements agentkit.Provider for Gemini.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// New constructs a Gemini provider handle.
func New(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Name() string {
	return "google"
}

func (p *Provider) Pricing(model string) (agentkit.Pricing, bool) {
	pricing, ok := modelPricing[model]
	return pricing, ok
}

func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if req == nil {
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig)
	}

	body, warnings := p.requestBody(req)
	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, p.url(req.Model), body)
	if err != nil {
		return roundTripError(providerError(0, nil, "", "", "", err, 0))
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("X-Goog-Api-Key", p.apiKey)

	resp, err := httpx.Client(p.client).Do(httpReq)
	if err != nil {
		return roundTripError(transportError(err))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return roundTripError(transportError(err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return roundTripError(classifyError(resp.StatusCode, raw, resp.Header))
	}

	events, message, finish, usage, err := parseResponse(resp.Header.Get("Content-Type"), raw)
	if err != nil {
		return roundTripError(providerError(resp.StatusCode, raw, "", err.Error(), requestID(resp.Header), nil, 0))
	}
	return agentkit.NewRoundTrip(yieldEvents(events), message, finish, usage, warnings, nil)
}

func (p *Provider) url(model string) string {
	return p.baseURL + "/v1beta/models/" + url.PathEscape(model) + ":streamGenerateContent?alt=sse"
}

func (p *Provider) requestBody(req *agentkit.Request) (map[string]any, []agentkit.Warning) {
	body := map[string]any{
		"contents": contentsFromMessages(req.Messages),
	}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": req.System}},
		}
	}
	if len(req.Tools) > 0 {
		body["tools"] = []map[string]any{{
			"functionDeclarations": functionDeclarations(req.Tools),
		}}
	}

	gen, warnings := generationConfig(req.Model, req.Gen)
	if len(gen) > 0 {
		body["generationConfig"] = gen
	}
	return body, warnings
}

func contentsFromMessages(messages []agentkit.Message) []map[string]any {
	contents := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		parts := partsFromMessage(message)
		if len(parts) == 0 {
			continue
		}
		role := "user"
		if message.Role == agentkit.RoleAssistant {
			role = "model"
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": parts,
		})
	}
	return contents
}

func partsFromMessage(message agentkit.Message) []map[string]any {
	parts := make([]map[string]any, 0, len(message.Blocks))
	signatures := googleReasoningByTool(message.Blocks)
	for _, block := range message.Blocks {
		switch block := block.(type) {
		case agentkit.TextBlock:
			if block.Text != "" {
				parts = append(parts, map[string]any{"text": block.Text})
			}
		case agentkit.ToolUseBlock:
			call := map[string]any{
				"name": block.Name,
				"args": rawObject(block.Input),
				"id":   block.ID,
			}
			part := map[string]any{"functionCall": call}
			if sig := signatures[block.ID]; sig != "" {
				part["thoughtSignature"] = sig
			}
			parts = append(parts, part)
		case agentkit.ToolResultBlock:
			parts = append(parts, map[string]any{"functionResponse": map[string]any{
				"name": block.Name,
				"id":   block.ToolUseID,
				"response": map[string]any{
					"content":  block.Content,
					"is_error": block.IsError,
				},
			}})
		}
	}
	return parts
}

func googleReasoningByTool(blocks []agentkit.Block) map[string]string {
	signatures := make(map[string]string)
	for _, block := range blocks {
		reasoning, ok := block.(agentkit.ReasoningBlock)
		if !ok || reasoning.BoundToID == "" {
			continue
		}
		if sig := decodeThoughtSignature(reasoning.Opaque); sig != "" {
			signatures[reasoning.BoundToID] = sig
		}
	}
	return signatures
}

func rawObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]any{}
	}
	if v == nil {
		return map[string]any{}
	}
	return v
}

func functionDeclarations(tools []agentkit.Tool) []map[string]any {
	sorted := append([]agentkit.Tool(nil), tools...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name() < sorted[j].Name()
	})

	decls := make([]map[string]any, 0, len(sorted))
	for _, tool := range sorted {
		decls = append(decls, map[string]any{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  convertSchema(tool.JSONSchema()),
		})
	}
	return decls
}

func convertSchema(raw json.RawMessage) map[string]any {
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return map[string]any{"type": "OBJECT"}
	}
	converted, ok := convertSchemaValue(schema).(map[string]any)
	if !ok || len(converted) == 0 {
		return map[string]any{"type": "OBJECT"}
	}
	return converted
}

func convertSchemaValue(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}

	out := make(map[string]any)
	if typ, ok := m["type"].(string); ok && typ != "" {
		out["type"] = strings.ToUpper(typ)
	}
	copyString(out, m, "description")
	copyString(out, m, "format")
	if nullable, ok := m["nullable"].(bool); ok {
		out["nullable"] = nullable
	}
	if required, ok := stringSlice(m["required"]); ok {
		out["required"] = required
	}
	if enum, ok := stringSlice(m["enum"]); ok {
		out["enum"] = enum
	}
	if props, ok := m["properties"].(map[string]any); ok {
		converted := make(map[string]any, len(props))
		for name, prop := range props {
			converted[name] = convertSchemaValue(prop)
		}
		out["properties"] = converted
	}
	if items, ok := m["items"]; ok {
		out["items"] = convertSchemaValue(items)
	} else if strings.EqualFold(fmt.Sprint(m["type"]), "array") {
		out["items"] = map[string]any{"type": "STRING"}
	}
	if anyOf, ok := m["anyOf"].([]any); ok {
		out["anyOf"] = convertSchemaArray(anyOf)
	}
	if len(out) == 0 {
		out["type"] = "OBJECT"
	}
	return out
}

func convertSchemaArray(values []any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, convertSchemaValue(value))
	}
	return out
}

func copyString(out, in map[string]any, key string) {
	if s, ok := in[key].(string); ok && s != "" {
		out[key] = s
	}
}

func stringSlice(v any) ([]string, bool) {
	values, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func generationConfig(model string, gen agentkit.GenSettings) (map[string]any, []agentkit.Warning) {
	cfg := make(map[string]any)
	if gen.Temperature != nil {
		cfg["temperature"] = *gen.Temperature
	}
	if gen.TopP != nil {
		cfg["topP"] = *gen.TopP
	}
	if gen.MaxTokens > 0 {
		cfg["maxOutputTokens"] = gen.MaxTokens
	}
	if gen.Reasoning != agentkit.EffortDefault {
		thinking, warning := thinkingConfig(model, gen.Reasoning)
		cfg["thinkingConfig"] = thinking
		if warning != nil {
			return cfg, []agentkit.Warning{*warning}
		}
	}
	return cfg, nil
}

func thinkingConfig(model string, effort agentkit.ReasoningEffort) (map[string]any, *agentkit.Warning) {
	if isGemini3(model) {
		if effort == agentkit.EffortOff {
			if alwaysOnReasoning(model) {
				return map[string]any{"thinkingLevel": "minimal", "includeThoughts": true}, reasoningWarning("off", "minimal")
			}
			return map[string]any{"thinkingBudget": 0, "includeThoughts": false}, nil
		}
		return map[string]any{"thinkingLevel": thinkingLevel(effort), "includeThoughts": true}, nil
	}

	if effort == agentkit.EffortOff {
		if alwaysOnReasoning(model) {
			return map[string]any{"thinkingBudget": 1024, "includeThoughts": true}, reasoningWarning("off", "minimal")
		}
		return map[string]any{"thinkingBudget": 0, "includeThoughts": false}, nil
	}
	return map[string]any{"thinkingBudget": thinkingBudget(effort), "includeThoughts": true}, nil
}

func isGemini3(model string) bool {
	return strings.HasPrefix(model, "gemini-3.")
}

func alwaysOnReasoning(model string) bool {
	return model == ModelPro25 || model == ModelPro31Preview
}

func thinkingLevel(effort agentkit.ReasoningEffort) string {
	switch effort {
	case agentkit.EffortMinimal:
		return "minimal"
	case agentkit.EffortLow:
		return "low"
	case agentkit.EffortMedium:
		return "medium"
	case agentkit.EffortHigh, agentkit.EffortMax:
		return "high"
	default:
		return "medium"
	}
}

func thinkingBudget(effort agentkit.ReasoningEffort) int {
	switch effort {
	case agentkit.EffortMinimal:
		return 1024
	case agentkit.EffortLow:
		return 2048
	case agentkit.EffortMedium:
		return 4096
	case agentkit.EffortHigh:
		return 8192
	case agentkit.EffortMax:
		return 24576
	default:
		return 4096
	}
}

func reasoningWarning(requested, applied string) *agentkit.Warning {
	return &agentkit.Warning{
		Setting: "reasoning_effort",
		Detail:  "requested " + requested + ", applied " + applied,
	}
}

type generateContentResponse struct {
	Candidates     []candidate    `json:"candidates"`
	PromptFeedback promptFeedback `json:"promptFeedback"`
	UsageMetadata  usageMetadata  `json:"usageMetadata"`
}

type promptFeedback struct {
	BlockReason string `json:"blockReason"`
}

type candidate struct {
	Content      content `json:"content"`
	FinishReason string  `json:"finishReason"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             *string       `json:"text"`
	Thought          bool          `json:"thought"`
	ThoughtSignature string        `json:"thoughtSignature"`
	FunctionCall     *functionCall `json:"functionCall"`
}

type functionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type usageMetadata struct {
	PromptTokenCount        int64 `json:"promptTokenCount"`
	CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
	CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int64 `json:"thoughtsTokenCount"`
	TotalTokenCount         int64 `json:"totalTokenCount"`
}

func parseResponse(contentType string, raw []byte) ([]agentkit.Event, agentkit.Message, agentkit.FinishReason, agentkit.Usage, error) {
	responses, err := decodeResponses(contentType, raw)
	if err != nil {
		return nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, err
	}

	var events []agentkit.Event
	message := agentkit.Message{Role: agentkit.RoleAssistant}
	finish := agentkit.FinishStop
	var usage agentkit.Usage

	for _, response := range responses {
		if response.PromptFeedback.BlockReason != "" {
			finish = agentkit.FinishContentFilter
		}
		if len(response.Candidates) > 0 {
			candidate := response.Candidates[0]
			finish = mergeFinish(finish, candidate.FinishReason)
			parsedEvents, blocks := parseParts(candidate.Content.Parts)
			events = append(events, parsedEvents...)
			message.Blocks = append(message.Blocks, blocks...)
		}
		usage = addUsage(usage, mapUsage(response.UsageMetadata))
	}
	if hasToolUse(message) {
		finish = agentkit.FinishToolUse
	}
	if usage.Total != sumUsage(usage) {
		return nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, fmt.Errorf("gemini usage total %d does not match bucket sum %d", usage.Total, sumUsage(usage))
	}
	return events, message, finish, usage, nil
}

func decodeResponses(contentType string, raw []byte) ([]generateContentResponse, error) {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") || bytes.Contains(raw, []byte("data:")) {
		frames, err := sse.ReadAll(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		responses := make([]generateContentResponse, 0, len(frames))
		for _, frame := range frames {
			if len(bytes.TrimSpace(frame.Data)) == 0 || bytes.Equal(bytes.TrimSpace(frame.Data), []byte("[DONE]")) {
				continue
			}
			var response generateContentResponse
			if err := json.Unmarshal(frame.Data, &response); err != nil {
				return nil, err
			}
			responses = append(responses, response)
		}
		return responses, nil
	}

	var response generateContentResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	return []generateContentResponse{response}, nil
}

func parseParts(parts []part) ([]agentkit.Event, []agentkit.Block) {
	var events []agentkit.Event
	var blocks []agentkit.Block
	var pending []agentkit.ReasoningBlock

	flushPending := func(boundToID string) {
		for _, reasoning := range pending {
			reasoning.BoundToID = boundToID
			blocks = append(blocks, reasoning)
		}
		pending = nil
	}

	for _, part := range parts {
		signatureIndex := -1
		if part.ThoughtSignature != "" {
			pending = append(pending, agentkit.ReasoningBlock{
				Opaque: encodeThoughtSignature(part.ThoughtSignature),
			})
			signatureIndex = len(pending) - 1
		}

		if part.Thought {
			summary := ""
			if part.Text != nil {
				summary = *part.Text
				if summary != "" {
					events = append(events, agentkit.ReasoningDelta{Text: summary})
				}
			}
			if signatureIndex >= 0 && summary != "" {
				pending[signatureIndex].Summary = summary
			}
			continue
		}
		if part.Text != nil {
			text := *part.Text
			if text != "" {
				events = append(events, agentkit.TextDelta{Text: text})
				blocks = append(blocks, agentkit.TextBlock{Text: text})
			}
			continue
		}
		if part.FunctionCall != nil {
			id := agentkit.NewToolUseID()
			flushPending(id)
			input := part.FunctionCall.Args
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			blocks = append(blocks, agentkit.ToolUseBlock{
				ID:    id,
				Name:  part.FunctionCall.Name,
				Input: append(json.RawMessage(nil), input...),
			})
		}
	}
	flushPending("")
	return events, blocks
}

func encodeThoughtSignature(signature string) json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"thoughtSignature": signature})
	return raw
}

func decodeThoughtSignature(raw json.RawMessage) string {
	var payload struct {
		ThoughtSignature string `json:"thoughtSignature"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return payload.ThoughtSignature
}

func mergeFinish(current agentkit.FinishReason, reason string) agentkit.FinishReason {
	if current == agentkit.FinishContentFilter {
		return current
	}
	switch reason {
	case "", "STOP":
		return agentkit.FinishStop
	case "MAX_TOKENS":
		return agentkit.FinishMaxTokens
	case "SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
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

func mapUsage(metadata usageMetadata) agentkit.Usage {
	cached := metadata.CachedContentTokenCount
	input := metadata.PromptTokenCount - cached
	if input < 0 {
		input = 0
	}
	return agentkit.Usage{
		InputUncached:   input,
		CacheReadInput:  cached,
		Output:          metadata.CandidatesTokenCount,
		ReasoningOutput: metadata.ThoughtsTokenCount,
		Total:           metadata.TotalTokenCount,
	}
}

func addUsage(a, b agentkit.Usage) agentkit.Usage {
	return agentkit.Usage{
		InputUncached:   a.InputUncached + b.InputUncached,
		CacheReadInput:  a.CacheReadInput + b.CacheReadInput,
		CacheWriteInput: a.CacheWriteInput + b.CacheWriteInput,
		CacheWrite5m:    a.CacheWrite5m + b.CacheWrite5m,
		CacheWrite1h:    a.CacheWrite1h + b.CacheWrite1h,
		Output:          a.Output + b.Output,
		ReasoningOutput: a.ReasoningOutput + b.ReasoningOutput,
		Total:           a.Total + b.Total,
	}
}

func sumUsage(usage agentkit.Usage) int64 {
	return usage.InputUncached + usage.CacheReadInput + usage.CacheWriteInput + usage.Output + usage.ReasoningOutput
}

func yieldEvents(events []agentkit.Event) iter.Seq[agentkit.Event] {
	return func(yield func(agentkit.Event) bool) {
		for _, event := range events {
			if !yield(event) {
				return
			}
		}
	}
}

func roundTripError(err error) *agentkit.RoundTrip {
	return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, err)
}

type googleErrorBody struct {
	Error struct {
		Code    int              `json:"code"`
		Message string           `json:"message"`
		Status  string           `json:"status"`
		Details []map[string]any `json:"details"`
	} `json:"error"`
}

func classifyError(status int, raw []byte, header http.Header) error {
	var body googleErrorBody
	_ = json.Unmarshal(raw, &body)
	message := body.Error.Message
	typ := body.Error.Status
	category := categoryFor(status, typ, message)
	providerErr := providerError(status, raw, typ, message, requestID(header), nil, retryInfo(body))
	providerErr.Category = category
	return providerErr
}

func categoryFor(status int, typ, message string) error {
	lower := strings.ToLower(message)
	if status == http.StatusUnauthorized || typ == "UNAUTHENTICATED" {
		return agentkit.ErrAuthentication
	}
	if status == http.StatusForbidden || typ == "PERMISSION_DENIED" {
		return agentkit.ErrPermission
	}
	if status == http.StatusNotFound || typ == "NOT_FOUND" {
		return agentkit.ErrNotFound
	}
	if status == http.StatusTooManyRequests || typ == "RESOURCE_EXHAUSTED" {
		return agentkit.ErrRateLimited
	}
	if status == http.StatusServiceUnavailable || typ == "UNAVAILABLE" {
		return agentkit.ErrOverloaded
	}
	if status == http.StatusGatewayTimeout || typ == "DEADLINE_EXCEEDED" {
		return agentkit.ErrTimeout
	}
	if status == http.StatusInternalServerError || typ == "INTERNAL" {
		return agentkit.ErrServerError
	}
	if typ == "FAILED_PRECONDITION" {
		return agentkit.ErrBilling
	}
	if status == http.StatusBadRequest || typ == "INVALID_ARGUMENT" {
		if strings.Contains(lower, "context") || strings.Contains(lower, "token limit") {
			return agentkit.ErrContextLength
		}
		return agentkit.ErrInvalidRequest
	}
	return agentkit.ErrUnknown
}

func providerError(status int, raw []byte, typ, message, requestID string, err error, retryAfter time.Duration) *agentkit.Error {
	category := agentkit.ErrUnknown
	if err != nil {
		category = agentkit.ErrNetwork
	}
	return &agentkit.Error{
		Category:   category,
		Provider:   "google",
		StatusCode: status,
		Type:       typ,
		Message:    message,
		RequestID:  requestID,
		RetryAfter: retryAfter,
		Raw:        append(json.RawMessage(nil), raw...),
		Err:        err,
	}
}

func transportError(err error) error {
	category := agentkit.ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = agentkit.ErrTimeout
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			category = agentkit.ErrTimeout
		}
	}
	return &agentkit.Error{
		Category: category,
		Provider: "google",
		Err:      err,
		Message:  err.Error(),
	}
}

func retryInfo(body googleErrorBody) time.Duration {
	for _, detail := range body.Error.Details {
		if detail["@type"] != "type.googleapis.com/google.rpc.RetryInfo" {
			continue
		}
		if delay, ok := detail["retryDelay"].(string); ok {
			if d, err := time.ParseDuration(delay); err == nil {
				return d
			}
		}
	}
	return 0
}

func requestID(header http.Header) string {
	if id := header.Get("X-Goog-Request-Id"); id != "" {
		return id
	}
	return header.Get("X-Request-Id")
}
