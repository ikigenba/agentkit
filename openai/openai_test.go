package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

type unknownBlock struct {
	agentkit.TextBlock
}

func TestProviderSendBuildsResponsesRequestsAndReplaysReasoning(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
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

		w.Header().Set("Content-Type", "text/event-stream")
		switch n {
		case 1:
			fmt.Fprint(w, openAIToolTurnSSE())
		case 2:
			fmt.Fprint(w, textOnlySSE("done", 7, 0, 3, 0))
		default:
			t.Errorf("unexpected request count: %d", n)
		}
	}))
	defer server.Close()

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
		Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelGPT55,
		System:   "Be terse.",
		Gen: agentkit.GenSettings{
			Temperature: &temperature,
			TopP:        &topP,
			MaxTokens:   128,
			Reasoning:   agentkit.Level("low"),
		},
		Tools: []agentkit.Tool{tool},
	}

	// R-H3PK-QFG3, R-XR4M-U1ZT, R-XW08-D4YL, R-C8UE-VJ67,
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
	if ok := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(toolUse.ID); !ok {
		t.Fatalf("tool ID has provider charset leakage: %q", toolUse.ID)
	}
	if string(toolUse.Input) != `{"city":"Paris"}` {
		t.Fatalf("assembled tool input = %s", toolUse.Input)
	}

	if len(c.History) != 4 {
		t.Fatalf("history length = %d", len(c.History))
	}
	reasoning, ok := c.History[1].Blocks[0].(agentkit.ReasoningBlock)
	if !ok {
		t.Fatalf("first assistant block = %T", c.History[1].Blocks[0])
	}
	if len(reasoning.Opaque) == 0 || !strings.Contains(string(reasoning.Opaque), "enc-openai-secret") {
		t.Fatalf("reasoning opaque = %s", reasoning.Opaque)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	for _, body := range requests {
		if body["store"] != false {
			t.Fatalf("store = %#v", body["store"])
		}
		if include := fmt.Sprint(body["include"]); include != "[reasoning.encrypted_content]" {
			t.Fatalf("include = %#v", body["include"])
		}
	}
	first := requests[0]
	if first["temperature"] != temperature || first["top_p"] != topP || first["max_output_tokens"] != float64(128) {
		t.Fatalf("generation settings missing from request: %#v", first)
	}
	if reasoningReq, _ := first["reasoning"].(map[string]any); reasoningReq["effort"] != "low" {
		t.Fatalf("reasoning = %#v", first["reasoning"])
	}
	secondInput, _ := requests[1]["input"].([]any)
	if !inputContains(secondInput, "reasoning", "encrypted_content", "enc-openai-secret") {
		t.Fatalf("second request did not replay OpenAI encrypted reasoning: %#v", secondInput)
	}
	// R-OMKB-AY19
	if !inputReasoningSummaryText(secondInput, "enc-openai-secret", "checking") {
		t.Fatalf("second request did not replay OpenAI reasoning summary text: %#v", secondInput)
	}
	if !inputContains(secondInput, "function_call_output", "output", "sunny") {
		t.Fatalf("second request did not include tool output: %#v", secondInput)
	}
}

func TestOpenAIBuildRequestPanicsOnUnknownOutboundBlockType(t *testing.T) {
	// R-4YSE-6YBS
	provider := New("test-key")
	req := &agentkit.Request{
		Model: ModelGPT55,
		Messages: []agentkit.Message{{
			Role:   agentkit.RoleUser,
			Blocks: []agentkit.Block{unknownBlock{}},
		}},
	}

	assertUnknownBlockPanic(t, func() {
		_, _, _ = provider.buildRequest(req)
	})
}

func assertUnknownBlockPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic for unknown block type")
		}
		msg, ok := got.(string)
		if !ok {
			t.Fatalf("panic = %T(%v), want string", got, got)
		}
		if !strings.Contains(msg, "unknown block type") || !strings.Contains(msg, "unknownBlock") {
			t.Fatalf("panic = %q, want unknown block type message", msg)
		}
	}()
	fn()
}

func TestProviderReplaysEmptyReasoningSummaryArrayOnSecondSend(t *testing.T) {
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

		if n == 2 {
			input, _ := body["input"].([]any)
			summary, ok := inputReasoningSummary(input, "enc-empty-summary")
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"message":"Missing required parameter: 'input[1].summary'","type":"invalid_request_error"}}`)
				return
			}
			parts, ok := summary.([]any)
			if !ok || len(parts) != 0 {
				t.Errorf("second request summary = %#v, want empty array", summary)
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		switch n {
		case 1:
			fmt.Fprint(w, emptySummaryReasoningSSE())
		case 2:
			fmt.Fprint(w, textOnlySSE("done", 6, 0, 2, 0))
		default:
			t.Errorf("unexpected request count: %d", n)
		}
	}))
	defer server.Close()

	c := &agentkit.Conversation{
		Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelGPT55,
		Gen:      agentkit.GenSettings{Reasoning: agentkit.Level("low")},
	}

	first := c.Send(context.Background(), "think first")
	for range first.Events() {
	}
	if err := first.Err(); err != nil {
		t.Fatalf("first turn error: %v", err)
	}

	// R-OMKB-AY19
	second := c.Send(context.Background(), "continue")
	for range second.Events() {
	}
	if err := second.Err(); err != nil {
		t.Fatalf("second turn error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	secondInput, _ := requests[1]["input"].([]any)
	summary, ok := inputReasoningSummary(secondInput, "enc-empty-summary")
	if !ok {
		t.Fatalf("second request omitted reasoning summary: %#v", secondInput)
	}
	parts, ok := summary.([]any)
	if !ok || len(parts) != 0 {
		t.Fatalf("second request summary = %#v, want empty array", summary)
	}
}

func TestProviderWarnsAndDefaultsNativeReasoningAtBuildTime(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		reasoning    agentkit.ReasoningValue
		wantEffort   string
		wantWarnings []agentkit.WarningCode
		requirement  string
	}{
		{
			// R-T587-9RGW
			name:        "unset omits reasoning",
			model:       ModelGPT55,
			wantEffort:  "",
			requirement: "R-T587-9RGW",
		},
		{
			// R-B7YX-J342
			name:         "wrong kind defaults",
			model:        ModelGPT55,
			reasoning:    agentkit.Budget(8000),
			wantEffort:   "medium",
			wantWarnings: []agentkit.WarningCode{agentkit.WarnReasoningUnsupported},
			requirement:  "R-B7YX-J342",
		},
		{
			// R-B96T-WUUR
			name:         "carried over native value validates against request model",
			model:        ModelGPT55,
			reasoning:    agentkit.Level("max"),
			wantEffort:   "medium",
			wantWarnings: []agentkit.WarningCode{agentkit.WarnReasoningUnsupported},
			requirement:  "R-B96T-WUUR",
		},
		{
			// R-P89V-WVXD
			name:         "cannot disable defaults",
			model:        ModelGPT55Pro,
			reasoning:    agentkit.DisableReasoning(),
			wantEffort:   "high",
			wantWarnings: []agentkit.WarningCode{agentkit.WarnReasoningCannotDisable},
			requirement:  "R-P89V-WVXD",
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
				fmt.Fprint(w, textOnlySSE("ok", 1, 0, 1, 0))
			}))
			defer server.Close()

			conv := &agentkit.Conversation{
				Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
				Model:    tt.model,
				Gen:      agentkit.GenSettings{Reasoning: tt.reasoning},
			}
			stream := conv.Send(context.Background(), "hello")
			for range stream.Events() {
			}
			if err := stream.Err(); err != nil {
				t.Fatalf("stream error: %v", err)
			}

			reasoning, hasReasoning := body["reasoning"].(map[string]any)
			if tt.wantEffort == "" {
				// R-T587-9RGW
				if hasReasoning {
					t.Fatalf("%s: request carried reasoning: %#v", tt.requirement, body["reasoning"])
				}
			} else if !hasReasoning || reasoning["effort"] != tt.wantEffort {
				t.Fatalf("%s: reasoning = %#v, want effort %q", tt.requirement, body["reasoning"], tt.wantEffort)
			}

			warnings := stream.Warnings()
			if len(warnings) != len(tt.wantWarnings) {
				t.Fatalf("%s: warnings = %#v", tt.requirement, warnings)
			}
			for i, want := range tt.wantWarnings {
				if warnings[i].Setting != "reasoning" || warnings[i].Code != want {
					t.Fatalf("%s: warning[%d] = %#v, want reasoning/%v", tt.requirement, i, warnings[i], want)
				}
			}
		})
	}
}

func TestProviderReplaysFunctionCallArgumentsAsJSONString(t *testing.T) {
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

		if n == 2 {
			input, _ := body["input"].([]any)
			got := inputFunctionCallArguments(input)
			want := []string{`{"path":"PING"}`, `{"text":"hello"}`}
			if !reflect.DeepEqual(got, want) {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"message":"Invalid type for 'input[1].arguments': expected a string, but got an object instead","type":"invalid_request_error"}}`)
				return
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		switch n {
		case 1:
			fmt.Fprint(w, multiToolTurnSSE())
		case 2:
			fmt.Fprint(w, textOnlySSE("done", 8, 0, 2, 0))
		default:
			t.Errorf("unexpected request count: %d", n)
		}
	}))
	defer server.Close()

	var calls []string
	pathTool := agentkit.RawTool("read_path", "read path", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		calls = append(calls, "read_path")
		if string(input) != `{"path":"PING"}` {
			t.Fatalf("path tool input = %s", input)
		}
		return "pong", nil
	})
	echoTool := agentkit.RawTool("echo_text", "echo text", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		calls = append(calls, "echo_text")
		if string(input) != `{"text":"hello"}` {
			t.Fatalf("echo tool input = %s", input)
		}
		return "hello", nil
	})
	c := &agentkit.Conversation{
		Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelGPT55,
		Tools:    []agentkit.Tool{pathTool, echoTool},
	}

	// R-UJNS-PFLL
	stream := c.Send(context.Background(), "run both")
	for range stream.Events() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"read_path", "echo_text"}) {
		t.Fatalf("tool calls = %#v", calls)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	secondInput, _ := requests[1]["input"].([]any)
	got := inputFunctionCallArguments(secondInput)
	want := []string{`{"path":"PING"}`, `{"text":"hello"}`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("function_call arguments = %#v, want %#v", got, want)
	}
	if !inputContains(secondInput, "function_call_output", "output", "pong") ||
		!inputContains(secondInput, "function_call_output", "output", "hello") {
		t.Fatalf("second request missing tool outputs: %#v", secondInput)
	}
}

func TestProviderDropsForeignReasoningFromWireRequest(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, textOnlySSE("ok", 3, 0, 2, 0))
	}))
	defer server.Close()

	p := New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model: ModelGPT54Mini,
		Messages: []agentkit.Message{{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{
				agentkit.TextBlock{Text: "prior"},
				agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"signature":"anthropic"}`)},
			},
		}},
	})

	// R-055A-NI1P
	if err := rt.Err(); err != nil {
		t.Fatalf("round trip error: %v", err)
	}
	input, _ := request["input"].([]any)
	if inputContains(input, "reasoning", "encrypted_content", "anthropic") {
		t.Fatalf("foreign reasoning leaked to OpenAI request: %#v", input)
	}
}

func TestUsageMappingDisjointBucketsAndNativeTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, textOnlySSE("ok", 100, 25, 40, 7))
	}))
	defer server.Close()

	p := New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model:    ModelGPT54,
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
		Output:          33,
		ReasoningOutput: 7,
		Total:           140,
	}
	if got := rt.Usage(); got != want {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}

	badTotal := usagePayload{InputTokens: 10, OutputTokens: 5, TotalTokens: 99}
	if _, err := mapUsage(badTotal); err == nil {
		t.Fatal("native total mismatch did not error")
	}
}

func TestOpenAIErrorMappingPreservesRawAndRetryAfter(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		category   error
		retryAfter string
		wantDelay  time.Duration
	}{
		{"auth", http.StatusUnauthorized, `{"error":{"message":"bad key","type":"invalid_api_key"}}`, agentkit.ErrAuthentication, "", 0},
		{"permission", http.StatusForbidden, `{"error":{"message":"forbidden","type":"permission_error"}}`, agentkit.ErrPermission, "", 0},
		{"invalid", http.StatusBadRequest, `{"error":{"message":"bad","type":"invalid_request_error"}}`, agentkit.ErrInvalidRequest, "", 0},
		{"not-found", http.StatusNotFound, `{"error":{"message":"missing","type":"not_found_error"}}`, agentkit.ErrNotFound, "", 0},
		{"rate", http.StatusTooManyRequests, `{"error":{"message":"slow","type":"rate_limit_error"}}`, agentkit.ErrRateLimited, "3", 3 * time.Second},
		{"billing", http.StatusTooManyRequests, `{"error":{"message":"quota","type":"insufficient_quota","code":"insufficient_quota"}}`, agentkit.ErrBilling, "", 0},
		{"context", http.StatusBadRequest, `{"error":{"message":"too long","type":"invalid_request_error","code":"context_length_exceeded"}}`, agentkit.ErrContextLength, "", 0},
		{"content-filter", http.StatusBadRequest, `{"error":{"message":"filtered","type":"content_filter"}}`, agentkit.ErrContentFilter, "", 0},
		{"overloaded", http.StatusServiceUnavailable, `{"error":{"message":"busy","type":"server_overloaded"}}`, agentkit.ErrOverloaded, "", 0},
		{"server", http.StatusInternalServerError, `{"error":{"message":"boom","type":"server_error"}}`, agentkit.ErrServerError, "", 0},
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
			rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: ModelGPT54Nano})
			err := rt.Err()
			// R-BUR1-XAK8, R-BX6U-OU1M, R-BYER-2LSB
			if !errors.Is(err, tt.category) {
				t.Fatalf("errors.Is(%v) = false for %v", tt.category, err)
			}
			var providerErr *agentkit.Error
			if !errors.As(err, &providerErr) {
				t.Fatalf("errors.As(*agentkit.Error) failed for %v", err)
			}
			if providerErr.Provider != "openai" || providerErr.StatusCode != tt.status || providerErr.RequestID != "req_123" {
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

func TestOpenAIModelRegistryPricingAndTierSelection(t *testing.T) {
	p := New("test-key")
	expected := map[string]agentkit.Pricing{
		ModelGPT55Pro: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 30000, CacheReadInput: 30000, Output: 180000}}},
		ModelGPT55: {Tiers: []agentkit.RateTier{
			{MinInputTokens: 0, InputUncached: 5000, CacheReadInput: 500, Output: 30000},
			{MinInputTokens: 272001, InputUncached: 10000, CacheReadInput: 1000, Output: 45000},
		}},
		ModelGPT54: {Tiers: []agentkit.RateTier{
			{MinInputTokens: 0, InputUncached: 2500, CacheReadInput: 250, Output: 15000},
			{MinInputTokens: 272001, InputUncached: 5000, CacheReadInput: 500, Output: 22500},
		}},
		ModelGPT54Mini: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 750, CacheReadInput: 75, Output: 4500}}},
		ModelGPT54Nano: {Tiers: []agentkit.RateTier{{MinInputTokens: 0, InputUncached: 200, CacheReadInput: 20, Output: 1250}}},
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

	// R-V2SM-WC8V
	for _, model := range []string{ModelGPT55, ModelGPT54} {
		pricing, _ := p.Pricing(model)
		base := pricing.Cost(agentkit.Usage{InputUncached: 272001, Output: 1})
		high := pricing.Cost(agentkit.Usage{InputUncached: 272002, Output: 1})
		if high <= base {
			t.Fatalf("%s high-tier cost %d <= base-tier cost %d", model, high, base)
		}
	}
}

func openAIToolTurnSSE() string {
	return strings.Join([]string{
		sseData(`{"type":"response.reasoning_summary_text.delta","delta":"checking"}`),
		sseData(`{"type":"response.output_item.done","item":{"id":"rs_1","type":"reasoning","encrypted_content":"enc-openai-secret","summary":[{"type":"summary_text","text":"checking"}]}}`),
		sseData(`{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_provider","name":"weather"}}`),
		sseData(`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"city\":"}`),
		sseData(`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"Paris\"}"}`),
		sseData(`{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_provider","name":"weather"}}`),
		sseData(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":1}}}}`),
		"data: [DONE]\n\n",
	}, "")
}

func emptySummaryReasoningSSE() string {
	return strings.Join([]string{
		sseData(`{"type":"response.output_item.done","item":{"id":"rs_empty","type":"reasoning","encrypted_content":"enc-empty-summary"}}`),
		sseData(`{"type":"response.output_text.delta","delta":"ready"}`),
		sseData(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":1}}}}`),
		"data: [DONE]\n\n",
	}, "")
}

func multiToolTurnSSE() string {
	return strings.Join([]string{
		sseData(`{"type":"response.output_item.added","item":{"id":"fc_path","type":"function_call","call_id":"call_path","name":"read_path"}}`),
		sseData(`{"type":"response.function_call_arguments.delta","item_id":"fc_path","delta":"{\"path\":"}`),
		sseData(`{"type":"response.function_call_arguments.delta","item_id":"fc_path","delta":"\"PING\"}"}`),
		sseData(`{"type":"response.output_item.done","item":{"id":"fc_path","type":"function_call","call_id":"call_path","name":"read_path"}}`),
		sseData(`{"type":"response.output_item.added","item":{"id":"fc_echo","type":"function_call","call_id":"call_echo","name":"echo_text"}}`),
		sseData(`{"type":"response.function_call_arguments.delta","item_id":"fc_echo","delta":"{\"text\":"}`),
		sseData(`{"type":"response.function_call_arguments.delta","item_id":"fc_echo","delta":"\"hello\"}"}`),
		sseData(`{"type":"response.output_item.done","item":{"id":"fc_echo","type":"function_call","call_id":"call_echo","name":"echo_text"}}`),
		sseData(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`),
		"data: [DONE]\n\n",
	}, "")
}

func textOnlySSE(text string, input, cached, output, reasoning int64) string {
	total := input + output
	return strings.Join([]string{
		sseData(fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q}`, text)),
		sseData(fmt.Sprintf(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d,"input_tokens_details":{"cached_tokens":%d},"output_tokens_details":{"reasoning_tokens":%d}}}}`, input, output, total, cached, reasoning)),
		"data: [DONE]\n\n",
	}, "")
}

func sseData(data string) string {
	return "data: " + data + "\n\n"
}

func inputContains(input []any, typ, field, value string) bool {
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok || object["type"] != typ {
			continue
		}
		if fmt.Sprint(object[field]) == value {
			return true
		}
	}
	return false
}

func inputFunctionCallArguments(input []any) []string {
	var arguments []string
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok || object["type"] != "function_call" {
			continue
		}
		arg, ok := object["arguments"].(string)
		if !ok {
			return nil
		}
		arguments = append(arguments, arg)
	}
	return arguments
}

func inputReasoningSummary(input []any, encrypted string) (any, bool) {
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok || object["type"] != "reasoning" || object["encrypted_content"] != encrypted {
			continue
		}
		summary, ok := object["summary"]
		return summary, ok
	}
	return nil, false
}

func inputReasoningSummaryText(input []any, encrypted, text string) bool {
	summary, ok := inputReasoningSummary(input, encrypted)
	if !ok {
		return false
	}
	parts, ok := summary.([]any)
	if !ok || len(parts) != 1 {
		return false
	}
	part, ok := parts[0].(map[string]any)
	return ok && part["type"] == "summary_text" && part["text"] == text
}
