package zai

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
	"sync"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestZaiSendUsesBakedBaseURLAndAssemblesToolTurn(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.String(); got != defaultBaseURL+"/chat/completions" {
			t.Errorf("URL = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, body)
		n := len(requests)
		mu.Unlock()

		var payload string
		switch n {
		case 1:
			payload = zaiToolTurnSSE()
		case 2:
			payload = zaiTextSSE("done", 7, 0, 3)
		default:
			t.Errorf("unexpected request count: %d", n)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(payload)),
			Request:    r,
		}, nil
	})}

	temperature := 0.2
	topP := 0.9
	var called bool
	tool := agentkit.RawTool("weather", "get weather", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		called = true
		if string(input) != `{"city":"Paris"}` {
			t.Fatalf("tool input = %s", input)
		}
		return "sunny", nil
	})
	c := &agentkit.Conversation{
		Provider: New("test-key", WithHTTPClient(client)),
		Model:    ModelGLM52,
		System:   "Be terse.",
		Gen: agentkit.GenSettings{
			Temperature: &temperature,
			TopP:        &topP,
			MaxTokens:   128,
			Reasoning:   agentkit.Level("max"),
		},
		Tools: []agentkit.Tool{tool},
	}

	// R-H4XH-476S, R-P9HS-ANO2, R-XW08-D4YL, R-C8UE-VJ67,
	// R-P5U3-5CFZ, R-P71Z-J46O, R-T40A-VZQ7, R-ELUQ-VJIQ
	stream := c.Send(context.Background(), "weather?")
	var toolUseIndex, toolResultIndex int = -1, -1
	var toolUse agentkit.ToolUse
	i := 0
	for ev := range stream.Events() {
		switch ev := ev.(type) {
		case agentkit.ToolUse:
			toolUseIndex = i
			toolUse = ev
		case agentkit.ToolResult:
			toolResultIndex = i
			if ev.ID != toolUse.ID || ev.Name != "weather" || ev.Output != "sunny" || ev.IsError {
				t.Fatalf("unexpected tool result: %#v", ev)
			}
		}
		i++
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if !called {
		t.Fatal("tool was not called")
	}
	if toolUseIndex < 0 || toolResultIndex != toolUseIndex+1 {
		t.Fatalf("tool use/result order = %d/%d", toolUseIndex, toolResultIndex)
	}
	if string(toolUse.Input) != `{"city":"Paris"}` {
		t.Fatalf("assembled tool input = %s", toolUse.Input)
	}
	warnings := stream.Warnings()
	if len(warnings) == 0 || warnings[0].Setting != "tool_choice" {
		t.Fatalf("warnings = %#v, want tool_choice degradation", warnings)
	}

	if len(c.History) != 4 {
		t.Fatalf("history length = %d", len(c.History))
	}
	reasoning, ok := c.History[1].Blocks[0].(agentkit.ReasoningBlock)
	if !ok {
		t.Fatalf("first assistant block = %T", c.History[1].Blocks[0])
	}
	if len(reasoning.Opaque) == 0 || !strings.Contains(string(reasoning.Opaque), "think-zai-secret") {
		t.Fatalf("reasoning opaque = %s", reasoning.Opaque)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	first := requests[0]
	if first["temperature"] != temperature || first["top_p"] != topP || first["max_tokens"] != float64(128) {
		t.Fatalf("generation settings missing from request: %#v", first)
	}
	if first["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %#v", first["tool_choice"])
	}
	if thinking, _ := first["thinking"].(map[string]any); thinking["type"] != "enabled" {
		t.Fatalf("thinking = %#v", first["thinking"])
	}
	if first["reasoning_effort"] != "max" {
		t.Fatalf("reasoning_effort = %#v", first["reasoning_effort"])
	}
	secondMessages, _ := requests[1]["messages"].([]any)
	if !messageContains(secondMessages, "assistant", "reasoning_content", "think-zai-secret") {
		t.Fatalf("second request did not replay Z.ai reasoning: %#v", secondMessages)
	}
	if !messageContains(secondMessages, "tool", "content", "sunny") {
		t.Fatalf("second request did not include tool output: %#v", secondMessages)
	}
}

func TestZaiReplayedToolCallArgumentsAreJSONString(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}

		mu.Lock()
		requests = append(requests, body)
		n := len(requests)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		switch n {
		case 1:
			fmt.Fprint(w, zaiToolTurnSSE())
		case 2:
			if got, ok := replayedToolArguments(body); !ok || got != `{"city":"Paris"}` {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"code":"1210","message":"Invalid API parameter (type=1210)"}}`)
				return
			}
			fmt.Fprint(w, zaiTextSSE("done", 7, 0, 3))
		default:
			t.Errorf("unexpected request count: %d", n)
		}
	}))
	defer server.Close()

	tool := agentkit.RawTool("weather", "get weather", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		return "sunny", nil
	})
	c := &agentkit.Conversation{
		Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelGLM52,
		Tools:    []agentkit.Tool{tool},
	}

	// R-ZCMP-ARG8
	stream := c.Send(context.Background(), "weather?")
	for range stream.Events() {
	}
	if err := stream.Err(); err != nil {
		if errors.Is(err, agentkit.ErrInvalidRequest) || strings.Contains(err.Error(), "type=1210") {
			t.Fatalf("replayed tool arguments were rejected by strict endpoint: %v", err)
		}
		t.Fatalf("stream error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	got, ok := replayedToolArguments(requests[1])
	if !ok {
		t.Fatalf("second request did not encode assistant tool_call arguments as a JSON string: %#v", requests[1])
	}
	if got != `{"city":"Paris"}` {
		t.Fatalf("replayed arguments = %q, want %q", got, `{"city":"Paris"}`)
	}
}

func TestZaiNativeReasoningToggleLoweringAndDefaultWarning(t *testing.T) {
	tests := []struct {
		name        string
		reasoning   agentkit.ReasoningValue
		wantDisable bool
		wantWarning bool
	}{
		{
			// R-T40A-VZQ7
			// R-ELUQ-VJIQ
			name:        "disable reaches toggle model",
			reasoning:   agentkit.DisableReasoning(),
			wantDisable: true,
		},
		{
			// R-B7YX-J342
			name:        "unsupported level defaults to unset",
			reasoning:   agentkit.Level("max"),
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, zaiTextSSE("ok", 1, 0, 1))
			}))
			defer server.Close()

			conv := &agentkit.Conversation{
				Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
				Model:    ModelGLM47,
				Gen:      agentkit.GenSettings{Reasoning: tt.reasoning},
			}
			stream := conv.Send(context.Background(), "hello")
			for range stream.Events() {
			}
			if err := stream.Err(); err != nil {
				t.Fatalf("stream error: %v", err)
			}

			thinking, hasThinking := body["thinking"].(map[string]any)
			if tt.wantDisable {
				if !hasThinking || thinking["type"] != "disabled" {
					t.Fatalf("thinking = %#v, want disabled", body["thinking"])
				}
			} else if hasThinking || body["reasoning_effort"] != nil {
				t.Fatalf("unsupported toggle level was not defaulted to unset: %#v", body)
			}
			warnings := stream.Warnings()
			if tt.wantWarning {
				if len(warnings) != 1 || warnings[0].Setting != "reasoning" || warnings[0].Code != agentkit.WarnReasoningUnsupported {
					t.Fatalf("warnings = %#v, want reasoning unsupported", warnings)
				}
			} else if len(warnings) != 0 {
				t.Fatalf("warnings = %#v, want none", warnings)
			}
		})
	}
}

func TestZaiDropsForeignReasoningFromWireRequest(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, zaiTextSSE("ok", 3, 0, 2))
	}))
	defer server.Close()

	p := New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model: ModelGLM46,
		Messages: []agentkit.Message{{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{
				agentkit.TextBlock{Text: "prior"},
				agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"encrypted_content":"openai"}`)},
			},
		}},
	})
	for range rt.Events() {
	}

	// R-055A-NI1P
	if err := rt.Err(); err != nil {
		t.Fatalf("round trip error: %v", err)
	}
	messages, _ := request["messages"].([]any)
	if messageContains(messages, "assistant", "reasoning_content", "openai") {
		t.Fatalf("foreign reasoning leaked to Z.ai request: %#v", messages)
	}
}

func TestZaiUsageMappingDisjointBucketsAndNativeTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, zaiTextSSE("ok", 100, 25, 40))
	}))
	defer server.Close()

	p := New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model:    ModelGLM51,
		Messages: []agentkit.Message{{Role: agentkit.RoleUser, Blocks: []agentkit.Block{agentkit.TextBlock{Text: "hi"}}}},
	})
	for range rt.Events() {
	}

	// R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7
	if err := rt.Err(); err != nil {
		t.Fatalf("round trip error: %v", err)
	}
	want := agentkit.Usage{
		InputUncached:   75,
		CacheReadInput:  25,
		CacheWriteInput: 0,
		CacheWrite5m:    0,
		CacheWrite1h:    0,
		Output:          40,
		ReasoningOutput: 0,
		Total:           140,
	}
	if got := rt.Usage(); got != want {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, zaiBadUsageSSE())
	})
	rt = p.RoundTrip(context.Background(), &agentkit.Request{Model: ModelGLM51})
	for range rt.Events() {
	}
	if err := rt.Err(); err == nil {
		t.Fatal("native total mismatch did not error")
	}
}

func TestZaiErrorMappingPreservesRawRetryAfterAndCodes(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		category   error
		retryAfter string
		wantDelay  time.Duration
	}{
		{"auth-code", http.StatusBadRequest, `{"error":{"code":"1001","message":"bad key"}}`, agentkit.ErrAuthentication, "", 0},
		{"permission-status", http.StatusForbidden, `{"error":{"code":"9999","message":"forbidden"}}`, agentkit.ErrPermission, "", 0},
		{"invalid-status", http.StatusBadRequest, `{"error":{"code":"9999","message":"bad"}}`, agentkit.ErrInvalidRequest, "", 0},
		{"not-found-status", http.StatusNotFound, `{"error":{"code":"9999","message":"missing"}}`, agentkit.ErrNotFound, "", 0},
		{"rate-code", http.StatusBadRequest, `{"error":{"code":"1302","message":"slow"}}`, agentkit.ErrRateLimited, "3", 3 * time.Second},
		{"billing-code", http.StatusBadRequest, `{"error":{"code":"1110","message":"balance"}}`, agentkit.ErrBilling, "", 0},
		{"context-message", http.StatusBadRequest, `{"error":{"code":"9999","message":"context length exceeded"}}`, agentkit.ErrContextLength, "", 0},
		{"server-code", http.StatusBadRequest, `{"error":{"code":"1230","message":"boom"}}`, agentkit.ErrServerError, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-request-id", "req_123")
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			p := New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: ModelGLM47})
			for range rt.Events() {
			}
			err := rt.Err()
			// R-BUR1-XAK8, R-BX6U-OU1M, R-BYER-2LSB, R-BZMN-GDJ0
			if !errors.Is(err, tt.category) {
				t.Fatalf("errors.Is(%v) = false for %v", tt.category, err)
			}
			var providerErr *agentkit.Error
			if !errors.As(err, &providerErr) {
				t.Fatalf("errors.As(*agentkit.Error) failed for %v", err)
			}
			if providerErr.Provider != "zai" || providerErr.StatusCode != tt.status || providerErr.RequestID != "req_123" {
				t.Fatalf("provider error details = %#v", providerErr)
			}
			if string(providerErr.Raw) != tt.body {
				t.Fatalf("raw = %s, want %s", providerErr.Raw, tt.body)
			}
			if providerErr.RetryAfter != tt.wantDelay {
				t.Fatalf("retry-after = %s, want %s", providerErr.RetryAfter, tt.wantDelay)
			}
		})
	}
}

func TestZaiModelRegistryPricing(t *testing.T) {
	p := New("test-key")
	expected := map[string]agentkit.Pricing{
		ModelGLM52: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 1400, CacheReadInput: 260, Output: 4400}}},
		ModelGLM51: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 1400, CacheReadInput: 260, Output: 4400}}},
		ModelGLM47: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 600, CacheReadInput: 110, Output: 2200}}},
		ModelGLM46: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 600, CacheReadInput: 110, Output: 2200}}},
	}

	// R-VDY4-AP7H, R-V1KQ-IKI6
	for model, want := range expected {
		got, ok := p.Pricing(model)
		if !ok {
			t.Fatalf("Pricing(%q) returned ok=false", model)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Pricing(%q) = %#v, want %#v", model, got, want)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func zaiToolTurnSSE() string {
	return strings.Join([]string{
		zaiSSEData(`{"choices":[{"delta":{"reasoning_content":"think-zai-secret"}}]}`),
		zaiSSEData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_provider","type":"function","function":{"name":"weather","arguments":"{\"city\":"}}]}}]}`),
		zaiSSEData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]}}]}`),
		zaiSSEData(`{"choices":[{"finish_reason":"tool_calls","delta":{}}]}`),
		zaiSSEData(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":0}}}`),
		"data: [DONE]\n\n",
	}, "")
}

func zaiTextSSE(text string, prompt, cached, completion int64) string {
	total := prompt + completion
	return strings.Join([]string{
		zaiSSEData(fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, text)),
		zaiSSEData(`{"choices":[{"finish_reason":"stop","delta":{}}]}`),
		zaiSSEData(fmt.Sprintf(`{"choices":[],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}}}`, prompt, completion, total, cached)),
		"data: [DONE]\n\n",
	}, "")
}

func zaiBadUsageSSE() string {
	return strings.Join([]string{
		zaiSSEData(`{"choices":[{"delta":{"content":"ok"}}]}`),
		zaiSSEData(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":99}}`),
		"data: [DONE]\n\n",
	}, "")
}

func zaiSSEData(data string) string {
	return "data: " + data + "\n\n"
}

func messageContains(messages []any, role, field, value string) bool {
	for _, item := range messages {
		object, ok := item.(map[string]any)
		if !ok || object["role"] != role {
			continue
		}
		if fmt.Sprint(object[field]) == value {
			return true
		}
	}
	return false
}

func replayedToolArguments(request map[string]any) (string, bool) {
	messages, ok := request["messages"].([]any)
	if !ok {
		return "", false
	}
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok || message["role"] != "assistant" {
			continue
		}
		toolCalls, ok := message["tool_calls"].([]any)
		if !ok || len(toolCalls) == 0 {
			continue
		}
		call, ok := toolCalls[0].(map[string]any)
		if !ok {
			return "", false
		}
		function, ok := call["function"].(map[string]any)
		if !ok {
			return "", false
		}
		arguments, ok := function["arguments"].(string)
		return arguments, ok
	}
	return "", false
}
