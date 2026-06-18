package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestGoogleSendBuildsRequestParsesToolTurnAndUsage(t *testing.T) {
	var calls int32
	var sawAuth bool
	var sawLossySchema bool
	var sawThinking bool
	var sawReplay bool
	var sawToolResult bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		var body map[string]any
		decodeRequest(t, r, &body)

		switch call {
		case 1:
			// R-H3PK-QFG3
			if r.Header.Get("X-Goog-Api-Key") != "test-key" {
				t.Fatalf("missing Gemini API key header: %q", r.Header.Get("X-Goog-Api-Key"))
			}
			sawAuth = true
			if r.URL.Path != "/v1beta/models/gemini-2.5-flash:streamGenerateContent" || r.URL.Query().Get("alt") != "sse" {
				t.Fatalf("unexpected endpoint: path=%s rawQuery=%s", r.URL.Path, r.URL.RawQuery)
			}

			gen := field[map[string]any](t, body, "generationConfig")
			// R-P5U3-5CFZ
			if gen["temperature"] != 0.2 || gen["topP"] != 0.8 || gen["maxOutputTokens"] != float64(256) {
				t.Fatalf("generation settings not mapped: %#v", gen)
			}
			thinking := field[map[string]any](t, gen, "thinkingConfig")
			// R-P71Z-J46O
			if thinking["thinkingBudget"] != float64(8192) || thinking["includeThoughts"] != true {
				t.Fatalf("reasoning effort not mapped for Gemini 2.5: %#v", thinking)
			}
			sawThinking = true

			tools := field[[]any](t, body, "tools")
			decls := field[[]any](t, tools[0].(map[string]any), "functionDeclarations")
			params := field[map[string]any](t, decls[0].(map[string]any), "parameters")
			// R-X3VB-65U3
			if containsKey(params, "$ref") || containsKey(params, "oneOf") || containsKey(params, "additionalProperties") {
				t.Fatalf("Gemini schema conversion retained unsupported JSON Schema keywords: %#v", params)
			}
			if params["type"] != "OBJECT" {
				t.Fatalf("schema type was not converted to Gemini/OpenAPI form: %#v", params)
			}
			sawLossySchema = true

			writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"checking","thought":true,"thoughtSignature":"sig-tool-1"},{"functionCall":{"name":"lookup","args":{"city":"Austin"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"cachedContentTokenCount":3,"candidatesTokenCount":5,"thoughtsTokenCount":2,"totalTokenCount":27}}`)
		case 2:
			contents := field[[]any](t, body, "contents")
			if !requestContains(contents, "thoughtSignature", "sig-tool-1") {
				t.Fatalf("Gemini thought signature was not replayed on the bound functionCall: %#v", contents)
			}
			sawReplay = true
			if !requestContains(contents, "functionResponse", "lookup") || !requestContains(contents, "content", "sunny") {
				t.Fatalf("tool result was not serialized as a Gemini functionResponse: %#v", contents)
			}
			sawToolResult = true
			writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":1,"candidatesTokenCount":4,"totalTokenCount":14}}`)
		default:
			t.Fatalf("unexpected request %d", call)
		}
	}))
	defer server.Close()

	temp := 0.2
	topP := 0.8
	tool := agentkit.RawTool("lookup", "look up weather", json.RawMessage(`{
		"type":"object",
		"properties":{
			"city":{"type":"string"},
			"legacy":{"$ref":"#/$defs/legacy"},
			"choice":{"oneOf":[{"type":"string"},{"type":"number"}]}
		},
		"additionalProperties":false,
		"$defs":{"legacy":{"type":"string"}}
	}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		var payload struct {
			City string `json:"city"`
		}
		if err := json.Unmarshal(input, &payload); err != nil {
			return "", err
		}
		if payload.City != "Austin" {
			return "", fmt.Errorf("unexpected city %q", payload.City)
		}
		return "sunny", nil
	})

	conv := &agentkit.Conversation{
		Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelFlash25,
		Gen: agentkit.GenSettings{
			Temperature: &temp,
			TopP:        &topP,
			MaxTokens:   256,
			Reasoning:   agentkit.EffortHigh,
		},
		Tools: []agentkit.Tool{tool},
	}

	events := drainStream(t, conv.Send(context.Background(), "weather"))

	toolUseIndex, toolResultIndex := -1, -1
	for i, event := range events {
		switch event := event.(type) {
		case agentkit.ToolUse:
			toolUseIndex = i
			// R-C8UE-VJ67
			if event.Name != "lookup" || string(event.Input) != `{"city":"Austin"}` {
				t.Fatalf("tool use was not complete assembled JSON: %#v", event)
			}
		case agentkit.ToolResult:
			toolResultIndex = i
			// R-C8UE-VJ67
			if event.Name != "lookup" || event.Output != "sunny" || event.IsError {
				t.Fatalf("tool result was not fed back after tool use: %#v", event)
			}
		}
	}
	if toolUseIndex < 0 || toolResultIndex != toolUseIndex+1 {
		t.Fatalf("ToolUse and ToolResult were not adjacent and ordered: indexes %d/%d in %#v", toolUseIndex, toolResultIndex, events)
	}

	firstAssistant := conv.History[1]
	var reasoning agentkit.ReasoningBlock
	var use agentkit.ToolUseBlock
	for _, block := range firstAssistant.Blocks {
		switch block := block.(type) {
		case agentkit.ReasoningBlock:
			reasoning = block
		case agentkit.ToolUseBlock:
			use = block
		}
	}
	// R-IPGC-I69W
	if use.ID == "" || reasoning.BoundToID != use.ID {
		t.Fatalf("reasoning block was not bound to its Gemini tool call: reasoning=%#v use=%#v", reasoning, use)
	}
	// R-XW08-D4YL
	if len(reasoning.Opaque) == 0 {
		t.Fatalf("Gemini reasoning block did not capture non-empty opaque thoughtSignature")
	}

	usage := conv.TotalUsage()
	// R-Y810-TECF
	wantUsage := agentkit.Usage{InputUncached: 26, CacheReadInput: 4, Output: 9, ReasoningOutput: 2, Total: 41}
	if usage != wantUsage {
		t.Fatalf("usage mapping mismatch: got %#v want %#v", usage, wantUsage)
	}
	// R-Y98X-7634
	if sumUsage(usage) != usage.Total {
		t.Fatalf("usage buckets do not sum to total: %#v", usage)
	}
	// R-YAGT-KXTT
	if usage.CacheWriteInput != 0 || usage.CacheWrite5m != 0 || usage.CacheWrite1h != 0 {
		t.Fatalf("Gemini should not populate cache-write buckets: %#v", usage)
	}
	// R-YBOP-YPKI
	if usage.ReasoningOutput != 2 {
		t.Fatalf("Gemini thoughtsTokenCount did not populate ReasoningOutput: %#v", usage)
	}
	// R-YCWM-CHB7
	if usage.InputUncached != 26 || usage.CacheReadInput != 4 {
		t.Fatalf("cached tokens were not subtracted from Gemini prompt tokens: %#v", usage)
	}
	if !sawAuth || !sawLossySchema || !sawThinking || !sawReplay || !sawToolResult {
		t.Fatalf("server assertions did not all run")
	}
}

func TestGoogleReasoningOffDegradesWithWarningOnPro(t *testing.T) {
	var sawClamped bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeRequest(t, r, &body)
		gen := field[map[string]any](t, body, "generationConfig")
		thinking := field[map[string]any](t, gen, "thinkingConfig")
		if thinking["thinkingBudget"] != float64(1024) || thinking["includeThoughts"] != true {
			t.Fatalf("EffortOff was not clamped to minimal thinking on Gemini Pro: %#v", thinking)
		}
		sawClamped = true
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelPro25,
		Gen:      agentkit.GenSettings{Reasoning: agentkit.EffortOff},
	}
	stream := conv.Send(context.Background(), "hello")
	drainStream(t, stream)
	// R-P89V-WVXD
	warnings := stream.Warnings()
	if !sawClamped || len(warnings) != 1 || warnings[0].Setting != "reasoning_effort" {
		t.Fatalf("EffortOff on Gemini Pro did not degrade with warning: %#v", warnings)
	}
}

func TestGoogleErrorClassificationRawAndRetryInfo(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		want       error
		retryAfter time.Duration
	}{
		{"authentication", http.StatusUnauthorized, `{"error":{"code":401,"message":"bad key","status":"UNAUTHENTICATED"}}`, agentkit.ErrAuthentication, 0},
		{"permission", http.StatusForbidden, `{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`, agentkit.ErrPermission, 0},
		{"invalid", http.StatusBadRequest, `{"error":{"code":400,"message":"bad request","status":"INVALID_ARGUMENT"}}`, agentkit.ErrInvalidRequest, 0},
		{"context", http.StatusBadRequest, `{"error":{"code":400,"message":"token limit exceeded","status":"INVALID_ARGUMENT"}}`, agentkit.ErrContextLength, 0},
		{"not_found", http.StatusNotFound, `{"error":{"code":404,"message":"missing","status":"NOT_FOUND"}}`, agentkit.ErrNotFound, 0},
		{"rate_limited", http.StatusTooManyRequests, `{"error":{"code":429,"message":"slow down","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"31s"}]}}`, agentkit.ErrRateLimited, 31 * time.Second},
		{"overloaded", http.StatusServiceUnavailable, `{"error":{"code":503,"message":"unavailable","status":"UNAVAILABLE"}}`, agentkit.ErrOverloaded, 0},
		{"server", http.StatusInternalServerError, `{"error":{"code":500,"message":"internal","status":"INTERNAL"}}`, agentkit.ErrServerError, 0},
		{"timeout", http.StatusGatewayTimeout, `{"error":{"code":504,"message":"deadline","status":"DEADLINE_EXCEEDED"}}`, agentkit.ErrTimeout, 0},
		{"billing", http.StatusBadRequest, `{"error":{"code":400,"message":"billing disabled","status":"FAILED_PRECONDITION"}}`, agentkit.ErrBilling, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Goog-Request-Id", "req-google")
				w.WriteHeader(tc.statusCode)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			rt := New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{Model: ModelFlash25})
			err := rt.Err()
			// R-BUR1-XAK8
			if !errors.Is(err, tc.want) {
				t.Fatalf("errors.Is(%v) = false for %v", tc.want, err)
			}
			var providerErr *agentkit.Error
			// R-BX6U-OU1M
			if !errors.As(err, &providerErr) || providerErr.Provider != "google" || providerErr.StatusCode != tc.statusCode || providerErr.RequestID != "req-google" || string(providerErr.Raw) != tc.body {
				t.Fatalf("provider error detail mismatch: %#v raw=%s", providerErr, providerErr.Raw)
			}
			// R-BYER-2LSB
			if providerErr.RetryAfter != tc.retryAfter {
				t.Fatalf("RetryAfter = %s, want %s", providerErr.RetryAfter, tc.retryAfter)
			}
		})
	}
}

func TestGoogleDropsForeignReasoningFromWireRequest(t *testing.T) {
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawRequest = true
		// R-055A-NI1P
		if strings.Contains(string(raw), "encrypted_content") || strings.Contains(string(raw), "foreign") {
			t.Fatalf("foreign reasoning leaked into Gemini request: %s", raw)
		}
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
	}))
	defer server.Close()

	rt := New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{
		Model: ModelFlash25,
		Messages: []agentkit.Message{{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{
				agentkit.TextBlock{Text: "prior"},
				agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"encrypted_content":"foreign"}`), Summary: "drop me"},
			},
		}},
	})
	for range rt.Events() {
	}
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	if !sawRequest {
		t.Fatalf("server did not receive request")
	}
}

func TestGooglePricingRegistryAndTierSelection(t *testing.T) {
	provider := New("key")
	expected := map[string]agentkit.Pricing{
		ModelFlash25: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 300, CacheReadInput: 30, Output: 2500}}},
		ModelPro25: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 1250, CacheReadInput: 125, Output: 10000}, {
			MinInputTokens: 200001, InputUncached: 2500, CacheReadInput: 250, Output: 15000,
		}}},
		ModelFlash35: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 1500, CacheReadInput: 150, Output: 9000}}},
		ModelLite31:  {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 250, CacheReadInput: 25, Output: 1500}}},
		ModelPro31Preview: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 2000, CacheReadInput: 200, Output: 12000}, {
			MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 400, Output: 18000,
		}}},
	}

	for model, want := range expected {
		got, ok := provider.Pricing(model)
		// R-V1KQ-IKI6
		if !ok {
			t.Fatalf("exported model %s did not resolve to pricing", model)
		}
		// R-VDY4-AP7H
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("pricing for %s = %#v, want %#v", model, got, want)
		}
	}

	pro25, _ := provider.Pricing(ModelPro25)
	low := pro25.Cost(agentkit.Usage{InputUncached: 200000, Output: 1})
	high := pro25.Cost(agentkit.Usage{InputUncached: 200001, Output: 1})
	// R-P89V-WVXD also covers the Gemini 2.5 Pro half in the warning test.
	// R-V2SM-WC8V
	if low != agentkit.Cost(200000*1250+10000) || high != agentkit.Cost(200001*2500+15000) {
		t.Fatalf("Gemini tier selection mismatch: low=%d high=%d", low, high)
	}
}

func TestGoogleSerializesPortableHistoryAfterProviderSwitch(t *testing.T) {
	first := &scriptedProvider{}
	var sawPriorToolBlocks bool
	var sawNoForeignReasoning bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeRequest(t, r, &body)
		contents := field[[]any](t, body, "contents")
		// R-H65D-HYXH
		if len(contents) < 4 {
			t.Fatalf("provider switch did not preserve prior history in the next backend request: %#v", contents)
		}
		// R-00IP-I9D7
		if !requestContains(contents, "text", "first done") {
			t.Fatalf("second provider did not receive coherent prior assistant text: %#v", contents)
		}
		// R-ILSN-CV1T
		if !requestContains(contents, "functionCall", "lookup") || !requestContains(contents, "functionResponse", "lookup") || !requestContains(contents, "id", "abc_123") {
			t.Fatalf("portable tool-use/tool-result blocks were not serialized for Gemini: %#v", contents)
		}
		sawPriorToolBlocks = true
		// R-IO8G-4EJ7
		if requestContains(contents, "encrypted_content", "foreign") {
			t.Fatalf("foreign reasoning was sent across provider switch: %#v", contents)
		}
		sawNoForeignReasoning = true
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"second done"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: first,
		Model:    "first-model",
		Tools: []agentkit.Tool{agentkit.RawTool("lookup", "lookup", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
			return "tool output", nil
		})},
	}
	drainStream(t, conv.Send(context.Background(), "first"))
	if first.calls != 2 {
		t.Fatalf("scripted first provider calls = %d, want 2", first.calls)
	}

	conv.Provider = New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	conv.Model = ModelFlash25
	drainStream(t, conv.Send(context.Background(), "second"))
	if !sawPriorToolBlocks || !sawNoForeignReasoning {
		t.Fatalf("switch assertions did not run")
	}
}

type scriptedProvider struct {
	calls int
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) Pricing(model string) (agentkit.Pricing, bool) {
	return agentkit.Pricing{Tiers: []agentkit.RateTier{{MinInputTokens: 0}}}, true
}

func (p *scriptedProvider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	p.calls++
	switch p.calls {
	case 1:
		msg := agentkit.Message{Role: agentkit.RoleAssistant, Blocks: []agentkit.Block{
			agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"encrypted_content":"foreign"}`), Summary: "foreign", BoundToID: "abc_123"},
			agentkit.ToolUseBlock{ID: "abc_123", Name: "lookup", Input: json.RawMessage(`{"q":"x"}`)},
		}}
		return agentkit.NewRoundTrip(nil, msg, agentkit.FinishToolUse, agentkit.Usage{}, nil, nil)
	default:
		msg := agentkit.Message{Role: agentkit.RoleAssistant, Blocks: []agentkit.Block{agentkit.TextBlock{Text: "first done"}}}
		return agentkit.NewRoundTrip(nil, msg, agentkit.FinishStop, agentkit.Usage{}, nil, nil)
	}
}

func drainStream(t *testing.T, stream *agentkit.Stream) []agentkit.Event {
	t.Helper()
	var events []agentkit.Event
	for event := range stream.Events() {
		events = append(events, event)
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	return events
}

func decodeRequest(t *testing.T, r *http.Request, dst any) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("method = %s, want POST", r.Method)
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func writeSSE(t *testing.T, w http.ResponseWriter, data string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		t.Fatalf("write sse: %v", err)
	}
}

func field[T any](t *testing.T, m map[string]any, name string) T {
	t.Helper()
	value, ok := m[name]
	if !ok {
		var zero T
		t.Fatalf("missing field %s in %#v", name, m)
		return zero
	}
	typed, ok := value.(T)
	if !ok {
		var zero T
		t.Fatalf("field %s has type %T, want %T", name, value, zero)
	}
	return typed
}

func containsKey(v any, key string) bool {
	switch v := v.(type) {
	case map[string]any:
		for k, value := range v {
			if k == key || containsKey(value, key) {
				return true
			}
		}
	case []any:
		for _, value := range v {
			if containsKey(value, key) {
				return true
			}
		}
	}
	return false
}

func requestContains(v any, key, want string) bool {
	switch v := v.(type) {
	case map[string]any:
		for k, value := range v {
			if k == key && valueContains(value, want) {
				return true
			}
			if requestContains(value, key, want) {
				return true
			}
		}
	case []any:
		for _, value := range v {
			if requestContains(value, key, want) {
				return true
			}
		}
	}
	return false
}

func valueContains(v any, want string) bool {
	switch v := v.(type) {
	case string:
		return v == want
	case map[string]any:
		for _, value := range v {
			if valueContains(value, want) {
				return true
			}
		}
	case []any:
		for _, value := range v {
			if valueContains(value, want) {
				return true
			}
		}
	}
	return false
}
