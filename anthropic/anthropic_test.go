package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestNewProviderSendsAuthenticatedRequestToInjectedServer(t *testing.T) {
	// R-H3PK-QFG3
	// R-WKTI-LIIE
	var provider agentkit.Provider = New("test-key")
	if provider.Name() != "anthropic" {
		t.Fatalf("Name() = %q, want anthropic", provider.Name())
	}

	var gotPath, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-API-Key")
		writeSSEFile(t, w, "testdata/final_turn.sse")
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelSonnet46,
	}
	stream := conv.Send(context.Background(), "hello")
	drain(stream)

	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("request path = %q, want /v1/messages", gotPath)
	}
	if gotKey != "test-key" {
		t.Fatalf("X-API-Key = %q, want test-key", gotKey)
	}
}

func TestAnthropicDependencyIsolation(t *testing.T) {
	// R-01HL-I6TM
	cmd := exec.Command("go", "list", "-deps", "github.com/ikigenba/agentkit/anthropic")
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps failed: %v", err)
	}
	for _, forbidden := range []string{"google.golang.org/genai", "github.com/openai/", "github.com/anthropics/"} {
		if bytes.Contains(out, []byte(forbidden)) {
			t.Fatalf("dependency list contains %q:\n%s", forbidden, out)
		}
	}
}

func TestAnthropicGoldenSSEReplayIsDeterministic(t *testing.T) {
	// R-WM1E-ZA93
	first := goldenSnapshotForFixture(t, "testdata/tool_turn.sse")
	second := goldenSnapshotForFixture(t, "testdata/tool_turn.sse")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same fixture produced different snapshots\nfirst=%#v\nsecond=%#v", first, second)
	}

	const goldenPath = "testdata/tool_turn.golden.json"
	got := mustJSON(t, first)
	if *updateGolden {
		if err := os.WriteFile(goldenPath, got, 0o666); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestAnthropicUsageMappingFromRecordedResponse(t *testing.T) {
	// R-Y810-TECF
	// R-Y98X-7634
	// R-YAGT-KXTT
	// R-YBOP-YPKI
	// R-YCWM-CHB7
	snapshot := goldenSnapshotForFixture(t, "testdata/tool_turn.sse")
	want := agentkit.Usage{
		InputUncached:   10,
		CacheReadInput:  3,
		CacheWriteInput: 7,
		CacheWrite5m:    4,
		CacheWrite1h:    3,
		Output:          12,
		ReasoningOutput: 0,
		Total:           32,
	}
	if snapshot.Usage != want {
		t.Fatalf("usage = %#v, want %#v", snapshot.Usage, want)
	}
	if snapshot.Usage.Total != snapshot.Usage.InputUncached+snapshot.Usage.CacheReadInput+snapshot.Usage.CacheWriteInput+snapshot.Usage.Output+snapshot.Usage.ReasoningOutput {
		t.Fatalf("usage total does not equal summing buckets: %#v", snapshot.Usage)
	}
	if snapshot.Usage.CacheWrite5m+snapshot.Usage.CacheWrite1h != snapshot.Usage.CacheWriteInput {
		t.Fatalf("cache write split does not sum to total: %#v", snapshot.Usage)
	}
}

func TestAnthropicFragmentsToolJSONAndReplaysReasoningOpaque(t *testing.T) {
	// R-C8UE-VJ67
	// R-IN0J-QMSI
	// R-XW08-D4YL
	requests := make([]map[string]any, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, decodeRequest(t, r))
		if len(requests) == 1 {
			writeSSEFile(t, w, "testdata/tool_turn.sse")
			return
		}
		writeSSEFile(t, w, "testdata/final_turn.sse")
	}))
	defer server.Close()

	tool := agentkit.NewTool("weather", "get weather", func(_ context.Context, in struct {
		City string `json:"city"`
	}) (string, error) {
		if in.City != "Tokyo" {
			t.Fatalf("tool city = %q, want Tokyo", in.City)
		}
		return "21 C", nil
	})
	conv := &agentkit.Conversation{
		Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelSonnet46,
		Tools:    []agentkit.Tool{tool},
	}

	stream := conv.Send(context.Background(), "weather?")
	events := drain(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	useIdx, resultIdx := -1, -1
	for i, ev := range events {
		switch ev := ev.(type) {
		case agentkit.ToolUse:
			useIdx = i
			if string(ev.Input) != `{"city":"Tokyo"}` {
				t.Fatalf("ToolUse input = %s, want complete JSON", ev.Input)
			}
		case agentkit.ToolResult:
			resultIdx = i
			if ev.Output != "21 C" || ev.IsError {
				t.Fatalf("ToolResult = %#v, want successful result", ev)
			}
		}
	}
	if useIdx < 0 || resultIdx < 0 || useIdx > resultIdx {
		t.Fatalf("ToolUse/ToolResult order indexes = %d/%d, want use before result", useIdx, resultIdx)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	var opaque json.RawMessage
	for _, block := range conv.History[1].Blocks {
		if reasoning, ok := block.(agentkit.ReasoningBlock); ok {
			opaque = reasoning.Opaque
		}
	}
	if len(opaque) == 0 {
		t.Fatalf("assistant reasoning opaque is empty")
	}
	if !requestContainsSignature(requests[1], "sig-anthropic-1") {
		t.Fatalf("second request did not replay Anthropic signature:\n%s", mustJSON(t, requests[1]))
	}
}

func TestAnthropicDropsForeignReasoningBlocksFromRequest(t *testing.T) {
	// R-055A-NI1P
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoded := decodeRequest(t, r)
		body = mustJSON(t, decoded)
		writeSSEFile(t, w, "testdata/final_turn.sse")
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelSonnet46,
		History: []agentkit.Message{{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{
				agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"encrypted_content":"foreign"}`), Summary: "foreign"},
				agentkit.TextBlock{Text: "kept"},
			},
		}},
	}
	drain(conv.Send(context.Background(), "continue"))
	if bytes.Contains(body, []byte("encrypted_content")) || bytes.Contains(body, []byte(`"type":"thinking"`)) {
		t.Fatalf("foreign reasoning leaked into request:\n%s", body)
	}
	if !bytes.Contains(body, []byte("kept")) {
		t.Fatalf("non-reasoning history was dropped:\n%s", body)
	}
}

func TestAnthropicRequestMapsGenerationSettingsAndWarnings(t *testing.T) {
	t.Run("sampling and honored reasoning settings", func(t *testing.T) {
		// R-P5U3-5CFZ
		// R-P71Z-J46O
		// R-PBXL-275G
		temp, topP := 0.2, 0.9
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body = decodeRequest(t, r)
			writeSSEFile(t, w, "testdata/final_turn.sse")
		}))
		defer server.Close()

		conv := &agentkit.Conversation{
			Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
			Model:    ModelSonnet46,
			Gen: agentkit.GenSettings{
				Temperature: &temp,
				TopP:        &topP,
				MaxTokens:   123,
				Reasoning:   agentkit.EffortMax,
			},
		}
		stream := conv.Send(context.Background(), "hello")
		drain(stream)
		if err := stream.Err(); err != nil {
			t.Fatalf("Err() = %v, want nil", err)
		}
		assertNumber(t, body["temperature"], temp)
		assertNumber(t, body["top_p"], topP)
		assertNumber(t, body["max_tokens"], float64(123))
		output := body["output_config"].(map[string]any)
		if output["effort"] != "max" {
			t.Fatalf("output_config.effort = %v, want max", output["effort"])
		}
		if len(stream.Warnings()) != 0 {
			t.Fatalf("Warnings() = %#v, want empty", stream.Warnings())
		}
	})

	t.Run("zero sampling settings are omitted", func(t *testing.T) {
		// R-P5U3-5CFZ
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body = decodeRequest(t, r)
			writeSSEFile(t, w, "testdata/final_turn.sse")
		}))
		defer server.Close()

		conv := &agentkit.Conversation{Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())), Model: ModelSonnet46}
		drain(conv.Send(context.Background(), "hello"))
		for _, key := range []string{"temperature", "top_p", "thinking", "output_config"} {
			if _, ok := body[key]; ok {
				t.Fatalf("request contains %q when unset: %#v", key, body)
			}
		}
	})

	t.Run("opus off degrades with warning", func(t *testing.T) {
		// R-P89V-WVXD
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body = decodeRequest(t, r)
			writeSSEFile(t, w, "testdata/final_turn.sse")
		}))
		defer server.Close()

		conv := &agentkit.Conversation{
			Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
			Model:    ModelOpus48,
			Gen:      agentkit.GenSettings{Reasoning: agentkit.EffortOff},
		}
		stream := conv.Send(context.Background(), "hello")
		drain(stream)
		if err := stream.Err(); err != nil {
			t.Fatalf("Err() = %v, want nil", err)
		}
		warnings := stream.Warnings()
		if len(warnings) != 1 || warnings[0].Setting != "reasoning_effort" {
			t.Fatalf("Warnings() = %#v, want reasoning_effort degradation", warnings)
		}
		output := body["output_config"].(map[string]any)
		if output["effort"] != "low" {
			t.Fatalf("degraded effort = %v, want low", output["effort"])
		}
	})
}

func TestAnthropicDefaultCacheBreakpointOnStablePrefix(t *testing.T) {
	// R-W2LC-R90N
	longText := strings.Repeat("stable ", 1300)
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeRequest(t, r)
		writeSSEFile(t, w, "testdata/final_turn.sse")
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    ModelSonnet46,
		System:   "stable system",
		History: []agentkit.Message{{
			Role:   agentkit.RoleAssistant,
			Blocks: []agentkit.Block{agentkit.TextBlock{Text: longText}},
		}},
	}
	drain(conv.Send(context.Background(), "new user suffix"))

	raw := mustJSON(t, body)
	if got := bytes.Count(raw, []byte(`"cache_control"`)); got != 1 {
		t.Fatalf("cache_control count = %d, want exactly 1 in request:\n%s", got, raw)
	}
	messages := body["messages"].([]any)
	prior := messages[0].(map[string]any)
	content := prior["content"].([]any)
	last := content[len(content)-1].(map[string]any)
	if _, ok := last["cache_control"]; !ok {
		t.Fatalf("last stable-prefix block lacks cache_control: %#v", last)
	}
	current := messages[len(messages)-1].(map[string]any)
	if bytes.Contains(mustJSON(t, current), []byte(`cache_control`)) {
		t.Fatalf("current user suffix received cache_control: %#v", current)
	}
}

func TestAnthropicErrorClassificationAndRawCapture(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		typ        string
		message    string
		want       error
		retryAfter string
	}{
		{name: "authentication", status: 401, typ: "authentication_error", message: "bad key", want: agentkit.ErrAuthentication},
		{name: "permission", status: 403, typ: "permission_error", message: "denied", want: agentkit.ErrPermission},
		{name: "invalid", status: 400, typ: "invalid_request_error", message: "bad request", want: agentkit.ErrInvalidRequest},
		{name: "not found", status: 404, typ: "not_found_error", message: "missing", want: agentkit.ErrNotFound},
		{name: "rate", status: 429, typ: "rate_limit_error", message: "slow down", want: agentkit.ErrRateLimited, retryAfter: "2"},
		{name: "overloaded", status: 529, typ: "overloaded_error", message: "overloaded", want: agentkit.ErrOverloaded},
		{name: "server", status: 500, typ: "api_error", message: "server", want: agentkit.ErrServerError},
		{name: "timeout", status: 504, typ: "timeout_error", message: "timeout", want: agentkit.ErrTimeout},
		{name: "billing", status: 402, typ: "billing_error", message: "billing", want: agentkit.ErrBilling},
		{name: "context", status: 400, typ: "invalid_request_error", message: "context window exceeded", want: agentkit.ErrContextLength},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// R-BUR1-XAK8
			raw := []byte(`{"type":"error","error":{"type":"` + tt.typ + `","message":"` + tt.message + `"}}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.Header().Set("request-id", "req_123")
				w.WriteHeader(tt.status)
				_, _ = w.Write(raw)
			}))
			defer server.Close()

			conv := &agentkit.Conversation{
				Provider: New("key", WithBaseURL(server.URL), WithHTTPClient(server.Client())),
				Model:    ModelSonnet46,
				Retry:    agentkit.RetryPolicy{MaxAttempts: 1},
			}
			stream := conv.Send(context.Background(), "hello")
			drain(stream)

			if !errors.Is(stream.Err(), tt.want) {
				t.Fatalf("Err() = %v, want errors.Is(..., %v)", stream.Err(), tt.want)
			}
			var akErr *agentkit.Error
			if !errors.As(stream.Err(), &akErr) {
				t.Fatalf("Err() does not satisfy errors.As(*agentkit.Error): %v", stream.Err())
			}
			// R-BX6U-OU1M
			if !bytes.Equal(akErr.Raw, raw) || akErr.Provider != "anthropic" || akErr.StatusCode != tt.status || akErr.RequestID != "req_123" {
				t.Fatalf("agentkit.Error = %#v; raw=%s", akErr, akErr.Raw)
			}
			// R-BYER-2LSB
			if tt.retryAfter != "" && akErr.RetryAfter != 2*time.Second {
				t.Fatalf("RetryAfter = %v, want 2s", akErr.RetryAfter)
			}
			if tt.retryAfter == "" && akErr.RetryAfter != 0 {
				t.Fatalf("RetryAfter = %v, want 0", akErr.RetryAfter)
			}
		})
	}
}

func TestAnthropicRegistryAndPricingTable(t *testing.T) {
	provider := New("key")
	models := []string{ModelOpus48, ModelSonnet46, ModelHaiku45}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			// R-V1KQ-IKI6
			if _, ok := provider.Pricing(model); !ok {
				t.Fatalf("Pricing(%q) ok=false, want true", model)
			}
		})
	}

	// R-VDY4-AP7H
	want := map[string]agentkit.RateTier{
		ModelOpus48:   {MinInputTokens: 0, InputUncached: 5000, CacheReadInput: 500, CacheWrite5m: 6250, CacheWrite1h: 10000, Output: 25000},
		ModelSonnet46: {MinInputTokens: 0, InputUncached: 3000, CacheReadInput: 300, CacheWrite5m: 3750, CacheWrite1h: 6000, Output: 15000},
		ModelHaiku45:  {MinInputTokens: 0, InputUncached: 1000, CacheReadInput: 100, CacheWrite5m: 1250, CacheWrite1h: 2000, Output: 5000},
	}
	for model, wantTier := range want {
		pricing, _ := provider.Pricing(model)
		if len(pricing.Tiers) != 1 || pricing.Tiers[0] != wantTier {
			t.Fatalf("Pricing(%q) = %#v, want one tier %#v", model, pricing, wantTier)
		}
	}
}

type goldenSnapshot struct {
	Events []goldenEvent  `json:"events"`
	Blocks []goldenBlock  `json:"blocks"`
	Finish string         `json:"finish"`
	Usage  agentkit.Usage `json:"usage"`
}

type goldenEvent struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
}

type goldenBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	JSON    json.RawMessage `json:"json,omitempty"`
	Opaque  json.RawMessage `json:"opaque,omitempty"`
	Summary string          `json:"summary,omitempty"`
}

func goldenSnapshotForFixture(t *testing.T, path string) goldenSnapshot {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	events, message, finish, usage, err := parseStream(raw)
	if err != nil {
		t.Fatalf("parseStream() err = %v", err)
	}
	return goldenSnapshot{Events: goldenEvents(events), Blocks: goldenBlocks(message.Blocks), Finish: finishString(finish), Usage: usage}
}

func goldenEvents(events []agentkit.Event) []goldenEvent {
	out := make([]goldenEvent, 0, len(events))
	for _, ev := range events {
		switch ev := ev.(type) {
		case agentkit.TextDelta:
			out = append(out, goldenEvent{Type: "text_delta", Text: ev.Text})
		case agentkit.ReasoningDelta:
			out = append(out, goldenEvent{Type: "reasoning_delta", Text: ev.Text})
		case agentkit.ToolUse:
			out = append(out, goldenEvent{Type: "tool_use", ID: ev.ID, Name: ev.Name, JSON: ev.Input})
		}
	}
	return out
}

func goldenBlocks(blocks []agentkit.Block) []goldenBlock {
	out := make([]goldenBlock, 0, len(blocks))
	for _, block := range blocks {
		switch block := block.(type) {
		case agentkit.TextBlock:
			out = append(out, goldenBlock{Type: "text", Text: block.Text})
		case agentkit.ToolUseBlock:
			out = append(out, goldenBlock{Type: "tool_use", ID: block.ID, Name: block.Name, JSON: block.Input})
		case agentkit.ReasoningBlock:
			out = append(out, goldenBlock{Type: "reasoning", Opaque: block.Opaque, Summary: block.Summary})
		}
	}
	return out
}

func finishString(finish agentkit.FinishReason) string {
	switch finish {
	case agentkit.FinishStop:
		return "stop"
	case agentkit.FinishToolUse:
		return "tool_use"
	case agentkit.FinishMaxTokens:
		return "max_tokens"
	case agentkit.FinishContentFilter:
		return "content_filter"
	default:
		return "other"
	}
}

func writeSSEFile(t *testing.T, w http.ResponseWriter, path string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	_, _ = w.Write(raw)
}

func decodeRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	defer r.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return body
}

func drain(stream *agentkit.Stream) []agentkit.Event {
	var events []agentkit.Event
	for ev := range stream.Events() {
		events = append(events, ev)
	}
	return events
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return raw
}

func assertNumber(t *testing.T, got any, want float64) {
	t.Helper()
	n, ok := got.(float64)
	if !ok || n != want {
		t.Fatalf("number = %#v, want %v", got, want)
	}
}

func requestContainsSignature(body map[string]any, signature string) bool {
	messages, _ := body["messages"].([]any)
	for _, msg := range messages {
		content, _ := msg.(map[string]any)["content"].([]any)
		for _, item := range content {
			block, _ := item.(map[string]any)
			if block["type"] == "thinking" && block["signature"] == signature {
				return true
			}
		}
	}
	return false
}
