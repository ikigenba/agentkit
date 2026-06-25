// Package google implements the AgentKit provider SPI for Google's Gemini API.
package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/retry"
	"github.com/ikigenba/agentkit/internal/sse"
)

const (
	ModelFlash25 = "gemini-2.5-flash"
	ModelPro25   = "gemini-2.5-pro"
	ModelFlash35 = "gemini-3.5-flash"
	ModelLite31  = "gemini-3.1-flash-lite"
	// ModelPro31Preview is Google's preview-channel 3.x Pro reasoning model.
	ModelPro31Preview = "gemini-3.1-pro-preview"

	EmbedModelGemini001 = "gemini-embedding-001"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com"

// Reasoning exposes Gemini's static native reasoning vocabulary.
var Reasoning agentkit.ReasoningInspector = reasoningInspector{}

// Embeddings exposes Gemini's static embedding model vocabulary.
var Embeddings agentkit.EmbeddingInspector = embeddingInspector{}

type modelEntry struct {
	Pricing   agentkit.Pricing
	Reasoning agentkit.ReasoningSpec
}

var googleEmbeddingPricing = map[string]agentkit.EmbeddingPricing{
	EmbedModelGemini001: {InputToken: 150},
}

var googleEmbeddingSpecs = map[string]agentkit.EmbeddingSpec{
	EmbedModelGemini001: {
		NativeDimension: 3072,
		MinDimension:    128,
		MaxDimension:    3072,
		MaxInputTokens:  2048,
	},
}

var modelRegistry = map[string]modelEntry{
	ModelFlash25: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 300, CacheReadInput: 30, Output: 2500,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking budget", Kind: agentkit.ReasoningRange,
			Min: 0, Max: 24576,
			Sentinels:  []agentkit.Sentinel{{Value: 0, Meaning: "off"}, {Value: -1, Meaning: "dynamic"}},
			Default:    agentkit.Budget(-1),
			CanDisable: true,
		},
	},
	ModelPro25: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 1250, CacheReadInput: 125, Output: 10000,
		}, {
			MinInputTokens: 200001, InputUncached: 2500, CacheReadInput: 250, Output: 15000,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking budget", Kind: agentkit.ReasoningRange,
			Min: 128, Max: 32768,
			Sentinels: []agentkit.Sentinel{{Value: -1, Meaning: "dynamic"}},
			Default:   agentkit.Budget(-1),
		},
	},
	ModelFlash35: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 1500, CacheReadInput: 150, Output: 9000,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking level", Kind: agentkit.ReasoningEnum,
			Levels: []string{"minimal", "low", "medium", "high"}, Default: agentkit.Level("medium"),
		},
	},
	ModelLite31: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 250, CacheReadInput: 25, Output: 1500,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking level", Kind: agentkit.ReasoningEnum,
			Levels: []string{"minimal", "low", "medium", "high"}, Default: agentkit.Level("medium"),
		},
	},
	ModelPro31Preview: {
		Pricing: agentkit.Pricing{Tiers: []agentkit.RateTier{{
			MinInputTokens: 0, InputUncached: 2000, CacheReadInput: 200, Output: 12000,
		}, {
			MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 400, Output: 18000,
		}}},
		Reasoning: agentkit.ReasoningSpec{
			Term: "thinking level", Kind: agentkit.ReasoningEnum,
			Levels: []string{"low", "medium", "high"}, Default: agentkit.Level("high"),
		},
	},
}

type reasoningInspector struct{}

func (reasoningInspector) ReasoningSpec(model string) (agentkit.ReasoningSpec, bool) {
	entry, ok := modelRegistry[model]
	if !ok {
		return agentkit.ReasoningSpec{}, false
	}
	return cloneReasoningSpec(entry.Reasoning), true
}

func (reasoningInspector) SupportedReasoning() map[string]agentkit.ReasoningSpec {
	out := make(map[string]agentkit.ReasoningSpec, len(modelRegistry))
	for model, entry := range modelRegistry {
		out[model] = cloneReasoningSpec(entry.Reasoning)
	}
	return out
}

func cloneReasoningSpec(spec agentkit.ReasoningSpec) agentkit.ReasoningSpec {
	spec.Levels = append([]string(nil), spec.Levels...)
	spec.Sentinels = append([]agentkit.Sentinel(nil), spec.Sentinels...)
	return spec
}

type embeddingInspector struct{}

func (embeddingInspector) EmbeddingSpec(model string) (agentkit.EmbeddingSpec, bool) {
	spec, ok := googleEmbeddingSpecs[model]
	return spec, ok
}

func (embeddingInspector) SupportedEmbeddings() map[string]agentkit.EmbeddingSpec {
	out := make(map[string]agentkit.EmbeddingSpec, len(googleEmbeddingSpecs))
	for model, spec := range googleEmbeddingSpecs {
		out[model] = spec
	}
	return out
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

// NewEmbedder constructs a Google embeddings provider.
func NewEmbedder(apiKey string, opts ...Option) agentkit.EmbeddingProvider {
	p := New(apiKey, opts...)
	return &embeddingProvider{
		apiKey:  p.apiKey,
		baseURL: p.baseURL,
		client:  p.client,
		clock:   retry.RealClock{},
	}
}

func (p *Provider) Name() string {
	return "google"
}

func (p *Provider) Pricing(model string) (agentkit.Pricing, bool) {
	entry, ok := modelRegistry[model]
	return entry.Pricing, ok
}

func (p *Provider) UntranslatableSchemaConstructs(schema json.RawMessage) []string {
	return untranslatableSchemaConstructs(schema)
}

func (p *Provider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	if req == nil {
		return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig)
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

	message, finish, usage, err := parseResponse(resp.Header.Get("Content-Type"), raw)
	if err != nil {
		return roundTripError(providerError(resp.StatusCode, raw, "", err.Error(), requestID(resp.Header), nil, 0))
	}
	return agentkit.NewRoundTrip(message, finish, usage, warnings, nil)
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
		case agentkit.ReasoningBlock:
		default:
			panic(fmt.Sprintf("unknown block type %T", block))
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
	converted, ok := convertSchemaValue(schema, schema, nil).(map[string]any)
	if !ok || len(converted) == 0 {
		return map[string]any{"type": "OBJECT"}
	}
	return converted
}

func convertSchemaValue(v any, root any, stack map[string]bool) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}

	if ref, ok := m["$ref"].(string); ok {
		if resolved, ok := resolveLocalRef(root, ref); ok && !stack[ref] {
			nextStack := cloneRefStack(stack)
			nextStack[ref] = true
			return convertSchemaValue(resolved, root, nextStack)
		}
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
			converted[name] = convertSchemaValue(prop, root, stack)
		}
		out["properties"] = converted
	}
	if items, ok := m["items"]; ok {
		out["items"] = convertSchemaValue(items, root, stack)
	} else if strings.EqualFold(fmt.Sprint(m["type"]), "array") {
		out["items"] = map[string]any{"type": "STRING"}
	}
	if anyOf, ok := m["anyOf"].([]any); ok {
		out["anyOf"] = convertSchemaArray(anyOf, root, stack)
	} else if oneOf, ok := m["oneOf"].([]any); ok {
		out["anyOf"] = convertSchemaArray(oneOf, root, stack)
	}
	if len(out) == 0 {
		out["type"] = "OBJECT"
	}
	return out
}

func convertSchemaArray(values []any, root any, stack map[string]bool) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, convertSchemaValue(value, root, stack))
	}
	return out
}

func untranslatableSchemaConstructs(raw json.RawMessage) []string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	collectUntranslatableSchemaConstructs(v, v, nil, seen)
	keywords := make([]string, 0, len(seen))
	for keyword := range seen {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	return keywords
}

func collectUntranslatableSchemaConstructs(v any, root any, stack map[string]bool, seen map[string]struct{}) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			switch k {
			case "$ref":
				ref, _ := child.(string)
				resolved, ok := resolveLocalRef(root, ref)
				if ref == "" || !ok || stack[ref] {
					seen["$ref"] = struct{}{}
					continue
				}
				nextStack := cloneRefStack(stack)
				nextStack[ref] = true
				collectUntranslatableSchemaConstructs(resolved, root, nextStack, seen)
				continue
			case "additionalProperties":
				seen[k] = struct{}{}
			case "$defs", "definitions":
				continue
			}
			collectUntranslatableSchemaConstructs(child, root, stack, seen)
		}
	case []any:
		for _, child := range v {
			collectUntranslatableSchemaConstructs(child, root, stack, seen)
		}
	}
}

func cloneRefStack(stack map[string]bool) map[string]bool {
	next := make(map[string]bool, len(stack)+1)
	for ref, ok := range stack {
		next[ref] = ok
	}
	return next
}

func resolveLocalRef(root any, ref string) (any, bool) {
	if ref == "" || ref[0] != '#' {
		return nil, false
	}
	if ref == "#" {
		return root, true
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	cur := root
	for _, token := range strings.Split(ref[2:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[token]
		if !ok {
			return nil, false
		}
	}
	return cur, true
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
	value, warnings := checkedReasoning(model, gen.Reasoning)
	if !value.IsUnset() {
		cfg["thinkingConfig"] = thinkingConfig(value)
	}
	return cfg, warnings
}

func thinkingConfig(value agentkit.ReasoningValue) map[string]any {
	if value.Disabled() {
		return map[string]any{"thinkingBudget": 0, "includeThoughts": false}
	}
	if level, ok := value.Level(); ok {
		return map[string]any{"thinkingLevel": level, "includeThoughts": true}
	}
	if budget, ok := value.Budget(); ok {
		return map[string]any{"thinkingBudget": budget, "includeThoughts": budget != 0}
	}
	return nil
}

func checkedReasoning(model string, value agentkit.ReasoningValue) (agentkit.ReasoningValue, []agentkit.Warning) {
	if value.IsUnset() {
		return value, nil
	}
	entry, ok := modelRegistry[model]
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

func parseResponse(contentType string, raw []byte) (agentkit.Message, agentkit.FinishReason, agentkit.Usage, error) {
	responses, err := decodeResponses(contentType, raw)
	if err != nil {
		return agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, err
	}

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
			blocks := parseParts(candidate.Content.Parts)
			message.Blocks = append(message.Blocks, blocks...)
		}
		usage = addUsage(usage, mapUsage(response.UsageMetadata))
	}
	if hasToolUse(message) {
		finish = agentkit.FinishToolUse
	}
	if usage.Total != sumUsage(usage) {
		return agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, fmt.Errorf("gemini usage total %d does not match bucket sum %d", usage.Total, sumUsage(usage))
	}
	return message, finish, usage, nil
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

func parseParts(parts []part) []agentkit.Block {
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
			}
			if signatureIndex >= 0 && summary != "" {
				pending[signatureIndex].Summary = summary
			}
			continue
		}
		if part.Text != nil {
			text := *part.Text
			if text != "" {
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
	return blocks
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

const maxGoogleEmbeddingInputsPerRequest = 100

type embeddingProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
	clock   retry.Clock
}

func (p *embeddingProvider) Name() string {
	return "google"
}

func (p *embeddingProvider) Pricing(model string) (agentkit.EmbeddingPricing, bool) {
	if p == nil {
		return agentkit.EmbeddingPricing{}, false
	}
	pricing, ok := googleEmbeddingPricing[model]
	return pricing, ok
}

func (p *embeddingProvider) Embed(ctx context.Context, req *agentkit.EmbedRequest) *agentkit.EmbedRoundTrip {
	if p == nil || p.apiKey == "" || p.baseURL == "" || req == nil {
		return agentkit.NewEmbedRoundTrip(nil, agentkit.EmbeddingUsage{}, nil, agentkit.ErrInvalidConfig)
	}
	spec, ok := googleEmbeddingSpecs[req.Model]
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
	for start := 0; start < len(req.Inputs); start += maxGoogleEmbeddingInputsPerRequest {
		end := start + maxGoogleEmbeddingInputsPerRequest
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

func (p *embeddingProvider) embedChunkWithRetry(ctx context.Context, req *agentkit.EmbedRequest, inputs []string) ([][]float32, agentkit.EmbeddingUsage, error) {
	clock := p.clock
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

func (p *embeddingProvider) embedChunk(ctx context.Context, req *agentkit.EmbedRequest, inputs []string) ([][]float32, agentkit.EmbeddingUsage, error) {
	body := googleBatchEmbedRequest{
		Requests: make([]googleEmbedContentRequest, 0, len(inputs)),
	}
	for _, input := range inputs {
		item := googleEmbedContentRequest{
			Model:        "models/" + req.Model,
			Content:      googleEmbedContent{Parts: []googleEmbedPart{{Text: input}}},
			AutoTruncate: false,
		}
		if taskType := googleEmbeddingTaskType(req.Role); taskType != "" {
			item.TaskType = taskType
		}
		if req.Dimensions != 0 {
			item.OutputDimensionality = req.Dimensions
		}
		body.Requests = append(body.Requests, item)
	}

	httpReq, err := httpx.JSONRequest(ctx, http.MethodPost, p.embeddingURL(req.Model), body)
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, transportError(err)
	}
	httpReq.Header.Set("X-Goog-Api-Key", p.apiKey)

	resp, err := httpx.Client(p.client).Do(httpReq)
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, transportError(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, transportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, agentkit.EmbeddingUsage{}, classifyError(resp.StatusCode, raw, resp.Header)
	}

	var payload googleBatchEmbedResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, agentkit.EmbeddingUsage{}, transportError(err)
	}
	vectors, err := googleEmbeddingVectors(payload.Embeddings, len(inputs))
	if err != nil {
		return nil, agentkit.EmbeddingUsage{}, err
	}
	usage := agentkit.EmbeddingUsage{
		InputTokens: payload.UsageMetadata.PromptTokenCount,
		Total:       payload.UsageMetadata.PromptTokenCount,
	}
	return vectors, usage, nil
}

func (p *embeddingProvider) embeddingURL(model string) string {
	return p.baseURL + "/v1beta/models/" + url.PathEscape(model) + ":batchEmbedContents"
}

type googleBatchEmbedRequest struct {
	Requests []googleEmbedContentRequest `json:"requests"`
}

type googleEmbedContentRequest struct {
	Model                string             `json:"model"`
	Content              googleEmbedContent `json:"content"`
	TaskType             string             `json:"taskType,omitempty"`
	OutputDimensionality int                `json:"outputDimensionality,omitempty"`
	AutoTruncate         bool               `json:"autoTruncate"`
}

type googleEmbedContent struct {
	Parts []googleEmbedPart `json:"parts"`
}

type googleEmbedPart struct {
	Text string `json:"text"`
}

type googleBatchEmbedResponse struct {
	Embeddings    []googleEmbedding `json:"embeddings"`
	UsageMetadata struct {
		PromptTokenCount int64 `json:"promptTokenCount"`
	} `json:"usageMetadata"`
}

type googleEmbedding struct {
	Values []float32 `json:"values"`
}

func googleEmbeddingTaskType(role agentkit.InputType) string {
	switch role {
	case agentkit.InputQuery:
		return "RETRIEVAL_QUERY"
	case agentkit.InputDocument:
		return "RETRIEVAL_DOCUMENT"
	default:
		return ""
	}
}

func googleEmbeddingVectors(embeddings []googleEmbedding, count int) ([][]float32, error) {
	if len(embeddings) != count {
		return nil, &agentkit.Error{Category: agentkit.ErrUnknown, Provider: "google", Message: "provider embedding count does not match input count"}
	}
	vectors := make([][]float32, count)
	for i, embedding := range embeddings {
		vectors[i] = append([]float32(nil), embedding.Values...)
	}
	return vectors, nil
}

func addEmbeddingUsage(a, b agentkit.EmbeddingUsage) agentkit.EmbeddingUsage {
	return agentkit.EmbeddingUsage{InputTokens: a.InputTokens + b.InputTokens, Total: a.Total + b.Total}
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

func roundTripError(err error) *agentkit.RoundTrip {
	return agentkit.NewRoundTrip(agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, err)
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
		if strings.Contains(lower, "context") ||
			strings.Contains(lower, "token limit") ||
			strings.Contains(lower, "too many tokens") ||
			strings.Contains(lower, "too long") ||
			strings.Contains(lower, "exceed") {
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
