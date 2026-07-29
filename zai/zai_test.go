package zai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestZaiSendUsesBakedBaseURLAndAssemblesToolTurn(t *testing.T) {
	// R-CQO3-7EE9
	// R-CRVZ-L64Y
	var _ agentkit.Provider = New(APIKey("compile-time-credential-check"))
	var _ func(Credential, ...Option) *Provider = New

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
		Provider: New(APIKey("test-key"), WithHTTPClient(client)),
		Model:    "glm-5.2",
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
	// R-P5U3-5CFZ, R-T40A-VZQ7, R-ELUQ-VJIQ
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
		Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "glm-5.2",
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

func TestZaiReasoningLoweringDependsOnlyOnValueShape(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		reasoning    agentkit.ReasoningValue
		wantThinking string
		wantEffort   string
	}{
		{
			name:         "native level is sent unchanged for arbitrary model",
			model:        "never-cataloged-level-model",
			reasoning:    agentkit.Level("vendor-native-ultra"),
			wantThinking: "enabled",
			wantEffort:   "vendor-native-ultra",
		},
		{
			name:         "disable uses wire off form",
			model:        "never-cataloged-disable-model",
			reasoning:    agentkit.DisableReasoning(),
			wantThinking: "disabled",
		},
		{
			name:  "unset omits reasoning fields",
			model: "never-cataloged-unset-model",
		},
	}

	// R-CVJO-QHD1
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
				Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
				Model:    tt.model,
				Pricing:  &agentkit.Pricing{},
				Gen:      agentkit.GenSettings{Reasoning: tt.reasoning},
			}
			stream := conv.Send(context.Background(), "hello")
			for range stream.Events() {
			}
			if err := stream.Err(); err != nil {
				t.Fatalf("stream error: %v", err)
			}

			thinking, hasThinking := body["thinking"].(map[string]any)
			if tt.wantThinking == "" {
				if hasThinking || body["reasoning_effort"] != nil {
					t.Fatalf("unset reasoning fields were sent: %#v", body)
				}
			} else if !hasThinking || thinking["type"] != tt.wantThinking {
				t.Fatalf("thinking = %#v, want type %q", body["thinking"], tt.wantThinking)
			}
			if tt.wantEffort == "" {
				if _, ok := body["reasoning_effort"]; ok {
					t.Fatalf("reasoning_effort = %#v, want omitted", body["reasoning_effort"])
				}
			} else if got := body["reasoning_effort"]; got != tt.wantEffort {
				t.Fatalf("reasoning_effort = %#v, want %q", got, tt.wantEffort)
			}
			if len(stream.Warnings()) != 0 {
				t.Fatalf("warnings = %#v, want none", stream.Warnings())
			}
		})
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model: "any-model",
		Gen:   agentkit.GenSettings{Reasoning: agentkit.Budget(8000)},
	})
	if !errors.Is(rt.Err(), agentkit.ErrInvalidConfig) || requests != 0 {
		t.Fatalf("budget error = %v, requests = %d; want ErrInvalidConfig without request", rt.Err(), requests)
	}
}

func TestZaiVendorRejectionsPreserveModelAndReasoningValue(t *testing.T) {
	// R-CT3V-YXVN
	// R-CUBS-CPMC
	const model = "brand-new-model-not-in-agentkit"
	const effort = "vendor-native-rejected-effort"
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"code":"1210","message":"unknown model or reasoning effort"}}`)
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    model,
		Gen:      agentkit.GenSettings{Reasoning: agentkit.Level(effort)},
		Retry:    agentkit.RetryPolicy{MaxAttempts: 1},
	}
	stream := conv.Send(context.Background(), "hello")
	for range stream.Events() {
	}
	if body["model"] != model || body["reasoning_effort"] != effort {
		t.Fatalf("wire body = %#v, want unchanged model and reasoning effort", body)
	}
	if len(stream.Warnings()) != 0 {
		t.Fatalf("warnings = %#v, want none", stream.Warnings())
	}
	var providerErr *agentkit.Error
	if !errors.As(stream.Err(), &providerErr) || providerErr.Provider != "zai" || !errors.Is(providerErr, agentkit.ErrInvalidRequest) {
		t.Fatalf("error = %#v, want typed Z.ai invalid-request error", stream.Err())
	}
}

func TestZaiProviderOptionsShallowMergeIntoWireBody(t *testing.T) {
	// R-CXZH-I0UF
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, zaiTextSSE("ok", 1, 0, 1))
	}))
	defer server.Close()

	p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	requests := []*agentkit.Request{
		{Model: "custom", ProviderOptions: json.RawMessage(`{"stream":false,"provider":{"order":["zai"]},"vendor_flag":"verbatim"}`)},
		{Model: "plain"},
	}
	for _, req := range requests {
		if err := p.RoundTrip(context.Background(), req).Err(); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("bodies = %d, want 2", len(bodies))
	}
	if bodies[0]["stream"] != false || bodies[0]["vendor_flag"] != "verbatim" {
		t.Fatalf("merged body = %#v", bodies[0])
	}
	provider, ok := bodies[0]["provider"].(map[string]any)
	if !ok || fmt.Sprint(provider["order"]) != "[zai]" {
		t.Fatalf("provider option = %#v, want verbatim nested object", bodies[0]["provider"])
	}
	if bodies[1]["stream"] != true {
		t.Fatalf("empty options changed adapter body: %#v", bodies[1])
	}
	if _, ok := bodies[1]["vendor_flag"]; ok {
		t.Fatalf("empty options leaked prior merged key: %#v", bodies[1])
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

	p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model: "glm-4.6",
		Messages: []agentkit.Message{{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{
				agentkit.TextBlock{Text: "prior"},
				agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"encrypted_content":"openai"}`)},
			},
		}},
	})

	// R-055A-NI1P
	if err := rt.Err(); err != nil {
		t.Fatalf("round trip error: %v", err)
	}
	messages, _ := request["messages"].([]any)
	if messageContains(messages, "assistant", "reasoning_content", "openai") {
		t.Fatalf("foreign reasoning leaked to Z.ai request: %#v", messages)
	}
}

func TestZaiEnableReasoningUsesNativeThinkingForm(t *testing.T) {
	// R-DCOZ-8W8U
	var body map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, zaiTextSSE("ok", 1, 0, 1))
	}))
	defer server.Close()

	rt := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{
		Model: "glm-future",
		Gen:   agentkit.GenSettings{Reasoning: agentkit.EnableReasoning()},
	})
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if got, want := string(body["thinking"]), `{"type":"enabled"}`; got != want {
		t.Fatalf("thinking = %s, want %s", got, want)
	}
}

func TestZaiUsageMappingDisjointBucketsAndNativeTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, zaiTextSSE("ok", 100, 25, 40))
	}))
	defer server.Close()

	p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model:    "glm-5.1",
		Messages: []agentkit.Message{{Role: agentkit.RoleUser, Blocks: []agentkit.Block{agentkit.TextBlock{Text: "hi"}}}},
	})

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
	rt = p.RoundTrip(context.Background(), &agentkit.Request{Model: "glm-5.1"})
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

			p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: "glm-4.7"})
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
