package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
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

type unknownBlock struct {
	agentkit.TextBlock
}

type schemaTool struct {
	agentkit.Tool
	name        string
	description string
	schema      json.RawMessage
}

func (t schemaTool) Name() string                { return t.name }
func (t schemaTool) Description() string         { return t.description }
func (t schemaTool) JSONSchema() json.RawMessage { return t.schema }

func TestNewProviderSendsAuthenticatedRequestToInjectedServer(t *testing.T) {
	// R-CQO3-7EE9
	// R-H3PK-QFG3
	// R-WKTI-LIIE
	// R-LK0H-9AXO
	var provider agentkit.Provider = New(APIKey("test-key"))
	if got, want := provider.Identity(), (agentkit.Identity{Provider: agentkit.ProviderAnthropic, Auth: agentkit.AuthAPIKey}); got != want {
		t.Fatalf("Identity() = %#v, want %#v", got, want)
	}

	var gotPath, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-API-Key")
		writeSSEFile(t, w, "testdata/final_turn.sse")
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "claude-sonnet-4-6",
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

func TestAnthropicArbitraryModelReachesWireAndVendorRejectionIsTyped(t *testing.T) {
	// R-CT3V-YXVN
	const model = "claude-never-cataloged-2099"
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotModel, _ = decodeRequest(t, r)["model"].(string)
		w.Header().Set("request-id", "req_unknown_model")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"unknown model"}}`))
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    model,
		Retry:    agentkit.RetryPolicy{MaxAttempts: 1},
	}
	stream := conv.Send(context.Background(), "hello")
	drain(stream)

	if gotModel != model {
		t.Fatalf("wire model = %q, want %q", gotModel, model)
	}
	var providerErr *agentkit.Error
	if !errors.As(stream.Err(), &providerErr) || providerErr.Provider != "anthropic" || !errors.Is(providerErr, agentkit.ErrInvalidRequest) {
		t.Fatalf("Err() = %#v, want typed anthropic invalid-request error", stream.Err())
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

func TestAnthropicToolUseInputIgnoresStartPlaceholderFragmentsOnly(t *testing.T) {
	// R-OUE3-L8VS
	raw, err := os.ReadFile("testdata/tool_turn.sse")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"input":{}`)) {
		t.Fatalf("fixture does not exercise content_block_start input placeholder")
	}

	message, _, _, err := parseStream(raw)
	if errors.Is(err, agentkit.ErrInvalidRequest) {
		t.Fatalf("parseStream() returned placeholder-concatenation invalid request error: %v", err)
	}
	if err != nil {
		t.Fatalf("parseStream() err = %v, want nil", err)
	}

	for _, block := range message.Blocks {
		toolUse, ok := block.(agentkit.ToolUseBlock)
		if !ok {
			continue
		}
		if !json.Valid(toolUse.Input) {
			t.Fatalf("ToolUseBlock.Input is invalid JSON: %s", toolUse.Input)
		}
		if string(toolUse.Input) != `{"city":"Tokyo"}` {
			t.Fatalf("ToolUseBlock.Input = %s, want fragments-only JSON", toolUse.Input)
		}
		if bytes.HasPrefix(toolUse.Input, []byte(`{}{`)) {
			t.Fatalf("ToolUseBlock.Input retained start placeholder: %s", toolUse.Input)
		}
		return
	}
	t.Fatalf("fixture did not assemble a tool_use block: %#v", message.Blocks)
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
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "claude-sonnet-4-6",
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

func TestAnthropicReplayedThinkingBlockSerializesSummaryInThinkingField(t *testing.T) {
	// R-TQ77-6QLK
	var replayedThinking map[string]any
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body := decodeRequest(t, r)
		if requests == 1 {
			writeSSEFile(t, w, "testdata/tool_turn.sse")
			return
		}
		block := findThinkingBlock(body)
		if block == nil {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"thinking block required"}}`, http.StatusBadRequest)
			return
		}
		replayedThinking = block
		if _, ok := block["text"]; ok {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"messages.1.content.0.thinking.text: Extra inputs are not permitted"}}`, http.StatusBadRequest)
			return
		}
		if block["thinking"] != "Plan lookup" || block["signature"] != "sig-anthropic-1" {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"messages.1.content.0.thinking.thinking: Field required"}}`, http.StatusBadRequest)
			return
		}
		writeSSEFile(t, w, "testdata/final_turn.sse")
	}))
	defer server.Close()

	tool := agentkit.NewTool("weather", "get weather", func(_ context.Context, in struct {
		City string `json:"city"`
	}) (string, error) {
		return "21 C", nil
	})
	conv := &agentkit.Conversation{
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "claude-sonnet-4-6",
		Tools:    []agentkit.Tool{tool},
		Gen:      agentkit.GenSettings{Reasoning: agentkit.Level("low")},
	}

	stream := conv.Send(context.Background(), "weather?")
	drain(stream)
	if err := stream.Err(); err != nil {
		if errors.Is(err, agentkit.ErrInvalidRequest) {
			t.Fatalf("follow-up replay returned invalid request: %v", err)
		}
		t.Fatalf("follow-up replay error = %v, want nil", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if replayedThinking == nil {
		t.Fatalf("second request did not replay a thinking block")
	}
	if got := replayedThinking["thinking"]; got != "Plan lookup" {
		t.Fatalf("thinking = %#v, want Plan lookup; block:\n%s", got, mustJSON(t, replayedThinking))
	}
	if _, ok := replayedThinking["text"]; ok {
		t.Fatalf("thinking block serialized text field:\n%s", mustJSON(t, replayedThinking))
	}
}

func TestAnthropicReplayedEmptyThinkingBlockKeepsThinkingField(t *testing.T) {
	// R-T06O-8SZX
	var rawRequest []byte
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"read request failed"}}`, http.StatusBadRequest)
			return
		}
		rawRequest = append(rawRequest[:0], raw...)
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"malformed request JSON"}}`, http.StatusBadRequest)
			return
		}

		block := findThinkingBlock(body)
		if block == nil {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"thinking block required"}}`, http.StatusBadRequest)
			return
		}
		if _, ok := block["thinking"]; !ok {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"messages.0.content.0.thinking.thinking: Field required"}}`, http.StatusBadRequest)
			return
		}
		if block["thinking"] != "" || block["signature"] != "sig-empty-anthropic" {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"unexpected replayed thinking block"}}`, http.StatusBadRequest)
			return
		}
		if got := bytes.Count(raw, []byte(`"thinking":""`)); got != 1 {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"empty thinking field must appear exactly once"}}`, http.StatusBadRequest)
			return
		}
		if nonThinkingBlockHasThinkingField(body) {
			http.Error(w, `{"error":{"type":"invalid_request_error","message":"non-thinking content block contains thinking field"}}`, http.StatusBadRequest)
			return
		}

		writeSSEFile(t, w, "testdata/final_turn.sse")
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "claude-sonnet-4-6",
		History: []agentkit.Message{{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{
				agentkit.ReasoningBlock{
					Opaque:  json.RawMessage(`{"signature":"sig-empty-anthropic"}`),
					Summary: "",
				},
				agentkit.TextBlock{Text: "visible assistant text"},
			},
		}},
	}

	stream := conv.Send(context.Background(), "continue")
	drain(stream)
	if err := stream.Err(); err != nil {
		if errors.Is(err, agentkit.ErrInvalidRequest) {
			t.Fatalf("empty thinking replay returned invalid request: %v\nrequest:\n%s", err, rawRequest)
		}
		t.Fatalf("empty thinking replay error = %v, want nil", err)
	}
	if len(rawRequest) == 0 {
		t.Fatal("server did not receive request")
	}
	if !bytes.Contains(rawRequest, []byte(`"thinking":""`)) {
		t.Fatalf("raw request omitted empty thinking field:\n%s", rawRequest)
	}
	if nonThinkingBlockHasThinkingField(body) {
		t.Fatalf("non-thinking block gained thinking field:\n%s", mustJSON(t, body))
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
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "claude-sonnet-4-6",
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

func TestAnthropicToolSchemaUsesStrictClosedObjectsAndCanonicalOptionality(t *testing.T) {
	// R-XT52-U8YG
	tool := schemaTool{
		name:        "lookup",
		description: "look up a record",
		schema: json.RawMessage(`{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"$defs": {
				"unused": {"type": "string"}
			},
			"type": "object",
			"properties": {
				"query": {"type": "string"},
				"filter": {
					"type": "object",
					"properties": {
						"enabled": {"type": "boolean"},
						"label": {"type": "string"}
					},
					"required": ["enabled"]
				}
			},
			"required": ["query"]
		}`),
	}
	request, _, err := buildRequest(&agentkit.Request{
		Model: "claude-sonnet-4-6",
		Tools: []agentkit.Tool{tool},
	})
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode request: %v", err)
	}

	wire := body["tools"].([]any)[0].(map[string]any)
	if wire["strict"] != true {
		t.Fatalf("tool strict = %#v, want true", wire["strict"])
	}
	schema := wire["input_schema"].(map[string]any)
	if schema["additionalProperties"] != false {
		t.Fatalf("root additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	filter := schema["properties"].(map[string]any)["filter"].(map[string]any)
	if filter["additionalProperties"] != false {
		t.Fatalf("nested additionalProperties = %#v, want false", filter["additionalProperties"])
	}
	if got, want := schema["required"], []any{"query"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root required = %#v, want %#v", got, want)
	}
	if got, want := filter["required"], []any{"enabled"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("nested required = %#v, want %#v", got, want)
	}
	assertSchemaKeysAbsent(t, schema, "$schema", "$defs")
}

func TestBuildRequestToolsSortedByName(t *testing.T) {
	// R-XY0O-DBX8
	alpha := schemaTool{name: "alpha", description: "first", schema: json.RawMessage(`{"type":"object"}`)}
	zulu := schemaTool{name: "zulu", description: "last", schema: json.RawMessage(`{"type":"object"}`)}

	buildTools := func(tools []agentkit.Tool) ([]byte, []wireTool) {
		t.Helper()
		request, _, err := buildRequest(&agentkit.Request{Model: "claude-test", Tools: tools})
		if err != nil {
			t.Fatalf("buildRequest() error = %v", err)
		}
		raw, err := json.Marshal(request.Tools)
		if err != nil {
			t.Fatalf("marshal tools: %v", err)
		}
		return raw, request.Tools
	}

	forward, tools := buildTools([]agentkit.Tool{alpha, zulu})
	reversed, _ := buildTools([]agentkit.Tool{zulu, alpha})
	if !bytes.Equal(forward, reversed) {
		t.Fatalf("serialized tools differ by input order:\nforward: %s\nreverse: %s", forward, reversed)
	}
	if got := []string{tools[0].Name, tools[1].Name}; !reflect.DeepEqual(got, []string{"alpha", "zulu"}) {
		t.Fatalf("tool order = %#v, want [alpha zulu]", got)
	}
}

func TestAnthropicToolSchemaRendererPreservesEveryCanonicalConstruct(t *testing.T) {
	// R-2UV8-RBKS
	canonical := json.RawMessage(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs": {
			"address": {
				"type": "object",
				"title": "Address",
				"description": "A postal address",
				"properties": {
					"postal_code": {
						"type": "string",
						"pattern": "^[0-9]{3,5}$",
						"minLength": 3,
						"maxLength": 5,
						"format": "date-time",
						"default": "000"
					}
				},
				"required": ["postal_code"]
			}
		},
		"type": "object",
		"title": "Lookup input",
		"description": "Every canonical construct",
		"properties": {
			"address": {"$ref": "#/$defs/address"},
			"tags": {
				"type": "array",
				"items": {"type": "string"},
				"minItems": 1
			},
			"status": {
				"type": "string",
				"enum": ["new", "done"]
			},
			"fixed": {"const": "v1"},
			"choice": {
				"anyOf": [
					{"type": "string"},
					{"type": "integer"}
				]
			},
			"alternative": {
				"oneOf": [
					{"type": "boolean"},
					{"type": "number"}
				]
			},
			"nullable": {"type": ["string", "null"]},
			"nested": {
				"type": "object",
				"properties": {
					"value": {"type": "string"}
				},
				"required": ["value"]
			},
			"optional_note": {"type": "string"}
		},
		"required": ["address", "tags", "status", "fixed", "choice", "alternative", "nullable", "nested"]
	}`)
	request, _, err := buildRequest(&agentkit.Request{
		Model: "claude-sonnet-4-6",
		Tools: []agentkit.Tool{schemaTool{name: "complete", schema: canonical}},
	})
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	got := request.Tools[0].InputSchema
	want := map[string]any{
		"type":                 "object",
		"title":                "Lookup input",
		"description":          "Every canonical construct",
		"additionalProperties": false,
		"properties": map[string]any{
			"address": map[string]any{
				"type":                 "object",
				"title":                "Address",
				"description":          "A postal address",
				"additionalProperties": false,
				"properties": map[string]any{
					"postal_code": map[string]any{
						"type":      "string",
						"pattern":   "^[0-9]{3,5}$",
						"minLength": float64(3),
						"maxLength": float64(5),
						"format":    "date-time",
						"default":   "000",
					},
				},
				"required": []any{"postal_code"},
			},
			"tags": map[string]any{
				"type":     "array",
				"items":    map[string]any{"type": "string"},
				"minItems": float64(1),
			},
			"status": map[string]any{
				"type": "string",
				"enum": []any{"new", "done"},
			},
			"fixed": map[string]any{"const": "v1"},
			"choice": map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "integer"},
				},
			},
			"alternative": map[string]any{
				"anyOf": []any{
					map[string]any{"type": "boolean"},
					map[string]any{"type": "number"},
				},
			},
			"nullable": map[string]any{"type": []any{"string", "null"}},
			"nested": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"value": map[string]any{"type": "string"},
				},
				"required": []any{"value"},
			},
			"optional_note": map[string]any{"type": "string"},
		},
		"required": []any{"address", "tags", "status", "fixed", "choice", "alternative", "nullable", "nested"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rendered schema mismatch\nwant:\n%s\n\ngot:\n%s", mustJSON(t, want), mustJSON(t, got))
	}
	assertSchemaKeysAbsent(t, got, "$schema", "$defs", "$ref", "oneOf")
}

func TestAnthropicRequestMapsGenerationSettingsAndWarnings(t *testing.T) {
	t.Run("sampling and honored reasoning settings", func(t *testing.T) {
		// R-CUBS-CPMC
		// R-CVJO-QHD1
		// R-P5U3-5CFZ
		// R-PBXL-275G
		// R-T40A-VZQ7
		// R-ELUQ-VJIQ
		temp, topP := 0.2, 0.9
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body = decodeRequest(t, r)
			writeSSEFile(t, w, "testdata/final_turn.sse")
		}))
		defer server.Close()

		conv := &agentkit.Conversation{
			Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
			Model:    "claude-sonnet-4-6",
			Pricing:  &agentkit.Pricing{},
			Gen: agentkit.GenSettings{
				Temperature: &temp,
				TopP:        &topP,
				MaxTokens:   123,
				Reasoning:   agentkit.Level("max"),
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
		thinking := body["thinking"].(map[string]any)
		if thinking["type"] != "adaptive" {
			t.Fatalf("thinking.type = %v, want adaptive", thinking["type"])
		}
		if len(stream.Warnings()) != 0 {
			t.Fatalf("Warnings() = %#v, want empty", stream.Warnings())
		}
	})

	t.Run("zero sampling settings are omitted", func(t *testing.T) {
		// R-CVJO-QHD1
		// R-P5U3-5CFZ
		// R-T587-9RGW
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body = decodeRequest(t, r)
			writeSSEFile(t, w, "testdata/final_turn.sse")
		}))
		defer server.Close()

		conv := &agentkit.Conversation{Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())), Model: "claude-sonnet-4-6", Pricing: &agentkit.Pricing{}}
		drain(conv.Send(context.Background(), "hello"))
		for _, key := range []string{"temperature", "top_p", "thinking", "output_config"} {
			if _, ok := body[key]; ok {
				t.Fatalf("request contains %q when unset: %#v", key, body)
			}
		}
	})

	t.Run("budget and disable lower as native forms", func(t *testing.T) {
		tests := []struct {
			name      string
			model     string
			reasoning agentkit.ReasoningValue
			assert    func(t *testing.T, body map[string]any)
		}{
			{
				// R-CUBS-CPMC
				// R-CVJO-QHD1
				// R-T40A-VZQ7
				// R-ELUQ-VJIQ
				name:      "haiku budget",
				model:     "claude-haiku-4-5",
				reasoning: agentkit.Budget(5000),
				assert: func(t *testing.T, body map[string]any) {
					thinking := body["thinking"].(map[string]any)
					if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(5000) {
						t.Fatalf("thinking = %#v, want exact budget 5000", thinking)
					}
				},
			},
			{
				// R-CVJO-QHD1
				// R-T40A-VZQ7
				name:      "disable",
				model:     "claude-sonnet-4-6",
				reasoning: agentkit.DisableReasoning(),
				assert: func(t *testing.T, body map[string]any) {
					thinking := body["thinking"].(map[string]any)
					if thinking["type"] != "disabled" {
						t.Fatalf("thinking = %#v, want disabled", thinking)
					}
					if _, ok := body["output_config"]; ok {
						t.Fatalf("disabled reasoning emitted output_config: %#v", body["output_config"])
					}
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var body map[string]any
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body = decodeRequest(t, r)
					writeSSEFile(t, w, "testdata/final_turn.sse")
				}))
				defer server.Close()

				conv := &agentkit.Conversation{
					Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
					Model:    tt.model,
					Pricing:  &agentkit.Pricing{},
					Gen:      agentkit.GenSettings{Reasoning: tt.reasoning},
				}
				stream := conv.Send(context.Background(), "hello")
				drain(stream)
				if err := stream.Err(); err != nil {
					t.Fatalf("Err() = %v, want nil", err)
				}
				if len(stream.Warnings()) != 0 {
					t.Fatalf("Warnings() = %#v, want empty", stream.Warnings())
				}
				tt.assert(t, body)
			})
		}
	})

	t.Run("enable lowers to exact adaptive form", func(t *testing.T) {
		// R-DCOZ-8W8U
		// R-DF4S-0FQ8
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body = decodeRequest(t, r)
			writeSSEFile(t, w, "testdata/final_turn.sse")
		}))
		defer server.Close()

		conv := &agentkit.Conversation{
			Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
			Model:    "claude-future",
			Pricing:  &agentkit.Pricing{},
			Gen:      agentkit.GenSettings{Reasoning: agentkit.EnableReasoning()},
		}
		stream := conv.Send(context.Background(), "hello")
		drain(stream)
		if err := stream.Err(); err != nil {
			t.Fatalf("Err() = %v, want nil", err)
		}
		got, err := json.Marshal(body["thinking"])
		if err != nil {
			t.Fatalf("marshal thinking: %v", err)
		}
		if want := `{"type":"adaptive"}`; string(got) != want {
			t.Fatalf("thinking = %s, want %s", got, want)
		}
		if _, ok := body["output_config"]; ok {
			t.Fatalf("enabled reasoning emitted level field: %#v", body["output_config"])
		}
	})

	t.Run("native level is sent unchanged without warning", func(t *testing.T) {
		// R-CUBS-CPMC
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body = decodeRequest(t, r)
			writeSSEFile(t, w, "testdata/final_turn.sse")
		}))
		defer server.Close()

		conv := &agentkit.Conversation{
			Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
			Model:    "claude-sonnet-4-6",
			Pricing:  &agentkit.Pricing{},
			Gen:      agentkit.GenSettings{Reasoning: agentkit.Level("xhigh")},
		}
		stream := conv.Send(context.Background(), "hello")
		drain(stream)
		if err := stream.Err(); err != nil {
			t.Fatalf("Err() = %v, want nil", err)
		}
		if len(stream.Warnings()) != 0 {
			t.Fatalf("Warnings() = %#v, want no reasoning warning", stream.Warnings())
		}
		output := body["output_config"].(map[string]any)
		if output["effort"] != "xhigh" {
			t.Fatalf("effort = %v, want exact native value xhigh", output["effort"])
		}
	})
}

func TestAnthropicReasoningVendorRejectionPreservesSentValue(t *testing.T) {
	// R-CUBS-CPMC
	const nativeLevel = "vendor-specific-ultra"
	var gotEffort string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeRequest(t, r)
		output, _ := body["output_config"].(map[string]any)
		gotEffort, _ = output["effort"].(string)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"effort rejected"}}`))
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "any-model",
		Gen:      agentkit.GenSettings{Reasoning: agentkit.Level(nativeLevel)},
		Retry:    agentkit.RetryPolicy{MaxAttempts: 1},
	}
	stream := conv.Send(context.Background(), "hello")
	drain(stream)
	if gotEffort != nativeLevel {
		t.Fatalf("wire effort = %q, want %q", gotEffort, nativeLevel)
	}
	if len(stream.Warnings()) != 0 {
		t.Fatalf("Warnings() = %#v, want none", stream.Warnings())
	}
	var providerErr *agentkit.Error
	if !errors.As(stream.Err(), &providerErr) || providerErr.Provider != "anthropic" || !errors.Is(providerErr, agentkit.ErrInvalidRequest) {
		t.Fatalf("Err() = %#v, want typed anthropic invalid-request error", stream.Err())
	}
}

func TestAnthropicProviderOptionsShallowMergeIntoWireBody(t *testing.T) {
	// R-CXZH-I0UF
	tests := []struct {
		name    string
		options json.RawMessage
		assert  func(*testing.T, map[string]any)
	}{
		{
			name:    "non-empty options override and add top-level keys",
			options: json.RawMessage(`{"stream":false,"metadata":{"trace":"opaque"},"vendor_flag":"verbatim"}`),
			assert: func(t *testing.T, body map[string]any) {
				if body["stream"] != false || body["vendor_flag"] != "verbatim" {
					t.Fatalf("merged body = %#v", body)
				}
				metadata, ok := body["metadata"].(map[string]any)
				if !ok || metadata["trace"] != "opaque" {
					t.Fatalf("metadata = %#v, want opaque nested value", body["metadata"])
				}
			},
		},
		{
			name: "empty options merge nothing",
			assert: func(t *testing.T, body map[string]any) {
				if body["stream"] != true {
					t.Fatalf("stream = %#v, want adapter default true", body["stream"])
				}
				if _, ok := body["vendor_flag"]; ok {
					t.Fatalf("empty options added vendor_flag: %#v", body)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body = decodeRequest(t, r)
				writeSSEFile(t, w, "testdata/final_turn.sse")
			}))
			defer server.Close()
			provider := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			trip := provider.RoundTrip(context.Background(), &agentkit.Request{
				Model:           "any-model",
				ProviderOptions: tt.options,
			})
			if err := trip.Err(); err != nil {
				t.Fatalf("Err() = %v, want nil", err)
			}
			tt.assert(t, body)
		})
	}
}

func TestAnthropicBuildRequestPanicsOnUnknownOutboundBlockType(t *testing.T) {
	// R-4YSE-6YBS
	req := &agentkit.Request{
		Model: "claude-sonnet-4-6",
		Messages: []agentkit.Message{{
			Role:   agentkit.RoleUser,
			Blocks: []agentkit.Block{unknownBlock{}},
		}},
	}

	stablePrefixTokens(req)
	assertUnknownBlockPanic(t, func() {
		_, _, _ = buildRequest(req)
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
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "claude-sonnet-4-6",
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
				Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
				Model:    "claude-sonnet-4-6",
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

func TestAnthropicStreamErrorEventClassifiesFromEnvelopeType(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		message string
		want    error
	}{
		{name: "overloaded", typ: "overloaded_error", message: "temporarily overloaded", want: agentkit.ErrOverloaded},
		{name: "rate limited", typ: "rate_limit_error", message: "too many requests", want: agentkit.ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// R-FR35-46U7
			raw := []byte(`{"type":"error","error":{"type":"` + tt.typ + `","message":"` + tt.message + `"}}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("event: error\n"))
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(raw)
				_, _ = w.Write([]byte("\n\n"))
			}))
			defer server.Close()

			conv := &agentkit.Conversation{
				Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
				Model:    "claude-sonnet-4-6",
				Retry:    agentkit.RetryPolicy{MaxAttempts: 1},
			}
			stream := conv.Send(context.Background(), "hello")
			drain(stream)

			err := stream.Err()
			if !errors.Is(err, tt.want) {
				t.Fatalf("Err() = %v, want errors.Is(..., %v)", err, tt.want)
			}
			var akErr *agentkit.Error
			if !errors.As(err, &akErr) {
				t.Fatalf("Err() does not satisfy errors.As(*agentkit.Error): %v", err)
			}
			if akErr.Provider != "anthropic" {
				t.Fatalf("Provider = %q, want anthropic", akErr.Provider)
			}
			if akErr.StatusCode != 0 {
				t.Fatalf("StatusCode = %d, want 0", akErr.StatusCode)
			}
			if akErr.Type != tt.typ {
				t.Fatalf("Type = %q, want %q", akErr.Type, tt.typ)
			}
			if !bytes.Equal(akErr.Raw, raw) {
				t.Fatalf("Raw = %s, want %s", akErr.Raw, raw)
			}
		})
	}
}

type goldenSnapshot struct {
	Blocks []goldenBlock  `json:"blocks"`
	Finish string         `json:"finish"`
	Usage  agentkit.Usage `json:"usage"`
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
	message, finish, usage, err := parseStream(raw)
	if err != nil {
		t.Fatalf("parseStream() err = %v", err)
	}
	return goldenSnapshot{Blocks: goldenBlocks(message.Blocks), Finish: finishString(finish), Usage: usage}
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

func assertSchemaKeysAbsent(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for _, key := range forbidden {
			if _, ok := value[key]; ok {
				t.Fatalf("schema contains forbidden key %q:\n%s", key, mustJSON(t, value))
			}
		}
		for _, child := range value {
			assertSchemaKeysAbsent(t, child, forbidden...)
		}
	case []any:
		for _, child := range value {
			assertSchemaKeysAbsent(t, child, forbidden...)
		}
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

func findThinkingBlock(body map[string]any) map[string]any {
	messages, _ := body["messages"].([]any)
	for _, msg := range messages {
		content, _ := msg.(map[string]any)["content"].([]any)
		for _, item := range content {
			block, _ := item.(map[string]any)
			if block["type"] == "thinking" {
				return block
			}
		}
	}
	return nil
}

func nonThinkingBlockHasThinkingField(body map[string]any) bool {
	messages, _ := body["messages"].([]any)
	for _, msg := range messages {
		content, _ := msg.(map[string]any)["content"].([]any)
		for _, item := range content {
			block, _ := item.(map[string]any)
			if block["type"] != "thinking" {
				if _, ok := block["thinking"]; ok {
					return true
				}
			}
		}
	}
	return false
}
