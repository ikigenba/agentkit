package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

type unknownBlock struct {
	agentkit.TextBlock
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type rawTestTool struct {
	agentkit.Tool
	name        string
	description string
	schema      json.RawMessage
	fn          func(context.Context, json.RawMessage) (string, error)
}

func testRawTool(name, description string, schema json.RawMessage, fn func(context.Context, json.RawMessage) (string, error)) agentkit.Tool {
	return rawTestTool{name: name, description: description, schema: append(json.RawMessage(nil), schema...), fn: fn}
}

func (t rawTestTool) Name() string                { return t.name }
func (t rawTestTool) Description() string         { return t.description }
func (t rawTestTool) JSONSchema() json.RawMessage { return append(json.RawMessage(nil), t.schema...) }
func (t rawTestTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.fn(ctx, append(json.RawMessage(nil), input...))
}

func TestRequestBodyToolsSortedByName(t *testing.T) {
	// R-XY0O-DBX8
	alpha := testRawTool("alpha", "first", json.RawMessage(`{"type":"object"}`), nil)
	zulu := testRawTool("zulu", "last", json.RawMessage(`{"type":"object"}`), nil)
	provider := &Provider{}

	buildTools := func(tools []agentkit.Tool) ([]byte, []map[string]any) {
		t.Helper()
		body, _, err := provider.requestBody(&agentkit.Request{Model: "gemini-test", Tools: tools})
		if err != nil {
			t.Fatalf("requestBody() error = %v", err)
		}
		wireTools := body["tools"].([]map[string]any)
		raw, err := json.Marshal(wireTools)
		if err != nil {
			t.Fatalf("marshal tools: %v", err)
		}
		return raw, wireTools
	}

	forward, tools := buildTools([]agentkit.Tool{alpha, zulu})
	reversed, _ := buildTools([]agentkit.Tool{zulu, alpha})
	if string(forward) != string(reversed) {
		t.Fatalf("serialized tools differ by input order:\nforward: %s\nreverse: %s", forward, reversed)
	}
	declarations := tools[0]["functionDeclarations"].([]map[string]any)
	if got := []string{declarations[0]["name"].(string), declarations[1]["name"].(string)}; !reflect.DeepEqual(got, []string{"alpha", "zulu"}) {
		t.Fatalf("tool order = %#v, want [alpha zulu]", got)
	}
}

func TestChatAndEmbeddingProvidersReportAPIKeyIdentity(t *testing.T) {
	// R-LK0H-9AXO
	// R-LL8D-N2OD
	want := agentkit.Identity{Provider: agentkit.ProviderGoogle, Auth: agentkit.AuthAPIKey}
	providers := []interface{ Identity() agentkit.Identity }{
		New(APIKey("key")),
		NewEmbedder(APIKey("key")),
	}
	for _, provider := range providers {
		if got := provider.Identity(); got != want {
			t.Errorf("Identity() = %#v, want %#v", got, want)
		}
		if provider.Identity().Auth == "" {
			t.Error("Identity().Auth is empty")
		}
	}
}

func TestGoogleChatAndEmbedderReportMissingAPIKeyWithoutTransport(t *testing.T) {
	// R-UMIR-LIQG
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer server.Close()

	t.Run("chat", func(t *testing.T) {
		provider := New(APIKey(""), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
		conv := &agentkit.Conversation{Provider: provider, Model: "gemini-test"}
		stream := conv.Send(context.Background(), "hello")
		for range stream.Events() {
		}

		err := stream.Err()
		if !errors.Is(err, agentkit.ErrMissingCredential) {
			t.Fatalf("Send() error = %v, want ErrMissingCredential", err)
		}
		if !strings.Contains(err.Error(), "API key") {
			t.Fatalf("Send() error = %q, want Google API key named", err)
		}
	})

	t.Run("embedding", func(t *testing.T) {
		provider := NewEmbedder(APIKey(""), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
		rt := provider.Embed(context.Background(), &agentkit.EmbedRequest{
			Model:  EmbedModelGemini001,
			Inputs: []string{"hello"},
		})

		err := rt.Err()
		if !errors.Is(err, agentkit.ErrMissingCredential) {
			t.Fatalf("Embed() error = %v, want ErrMissingCredential", err)
		}
		if !strings.Contains(err.Error(), "API key") {
			t.Fatalf("Embed() error = %q, want Google API key named", err)
		}
	})

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("HTTP requests = %d, want 0", got)
	}
}

func TestGoogleSendBuildsRequestParsesToolTurnAndUsage(t *testing.T) {
	var calls int32
	var sawAuth bool
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
			// R-CQO3-7EE9
			if r.Header.Get("X-Goog-Api-Key") != "test-key" {
				t.Fatalf("missing Gemini API key header: %q", r.Header.Get("X-Goog-Api-Key"))
			}
			sawAuth = true
			if r.URL.Path != "/v1beta/models/gemini-never-cataloged-2099:streamGenerateContent" || r.URL.Query().Get("alt") != "sse" {
				t.Fatalf("unexpected endpoint: path=%s rawQuery=%s", r.URL.Path, r.URL.RawQuery)
			}
			// R-CT3V-YXVN

			gen := field[map[string]any](t, body, "generationConfig")
			// R-P5U3-5CFZ
			if gen["temperature"] != 0.2 || gen["topP"] != 0.8 || gen["maxOutputTokens"] != float64(256) {
				t.Fatalf("generation settings not mapped: %#v", gen)
			}
			thinking := field[map[string]any](t, gen, "thinkingConfig")
			// R-T40A-VZQ7
			// R-ELUQ-VJIQ
			if thinking["thinkingBudget"] != float64(8192) || thinking["thinkingLevel"] != nil || thinking["includeThoughts"] != true {
				t.Fatalf("native reasoning budget not mapped for Gemini 2.5: %#v", thinking)
			}
			sawThinking = true

			tools := field[[]any](t, body, "tools")
			decls := field[[]any](t, tools[0].(map[string]any), "functionDeclarations")
			params := field[map[string]any](t, decls[0].(map[string]any), "parameters")
			if params["type"] != "OBJECT" {
				t.Fatalf("schema type was not converted to Gemini/OpenAPI form: %#v", params)
			}

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
	tool := testRawTool("lookup", "look up weather", json.RawMessage(`{
		"type":"object",
		"properties":{
			"city":{"type":"string"},
			"legacy":{"$ref":"#/$defs/legacy"},
			"choice":{"oneOf":[{"type":"string"},{"type":"number"}]}
		},
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
		Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "gemini-never-cataloged-2099",
		Gen: agentkit.GenSettings{
			Temperature: &temp,
			TopP:        &topP,
			MaxTokens:   256,
			Reasoning:   agentkit.Budget(8192),
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
	if !sawAuth || !sawThinking || !sawReplay || !sawToolResult {
		t.Fatalf("server assertions did not all run")
	}
}

func TestGoogleToolSchemaAllowlistRewritesCanonicalDialect(t *testing.T) {
	// R-XVKV-LSFU
	var recorded map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeRequest(t, r, &body)
		tools := field[[]any](t, body, "tools")
		declarations := field[[]any](t, tools[0].(map[string]any), "functionDeclarations")
		recorded = field[map[string]any](t, declarations[0].(map[string]any), "parameters")
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	canonical := json.RawMessage(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs": {
			"record": {
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"name": {"type": "string", "$schema": "discard-me"}
				},
				"required": ["name"]
			}
		},
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"record": {"$ref": "#/$defs/record"},
			"choice": {"oneOf": [{"type": "string"}, {"type": "number"}]},
			"fixed": {"const": "v1"},
			"nullable": {"type": ["string", "null"]}
		}
	}`)
	tool := testRawTool("complete", "exercise canonical rewrites", canonical, func(context.Context, json.RawMessage) (string, error) {
		return "ok", nil
	})
	rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{
		Model: "gemini-test",
		Tools: []agentkit.Tool{tool},
	})
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	for _, forbidden := range []string{"additionalProperties", "$schema", "$defs", "$ref", "oneOf", "const"} {
		if containsKey(recorded, forbidden) {
			t.Errorf("recorded parameters contain forbidden key %q: %#v", forbidden, recorded)
		}
	}
	properties := field[map[string]any](t, recorded, "properties")
	inlined := field[map[string]any](t, properties, "record")
	if inlined["type"] != "OBJECT" || field[map[string]any](t, inlined, "properties")["name"] == nil {
		t.Fatalf("$ref target was not inlined: %#v", inlined)
	}
	choice := field[[]any](t, field[map[string]any](t, properties, "choice"), "anyOf")
	if got := []any{choice[0].(map[string]any)["type"], choice[1].(map[string]any)["type"]}; !reflect.DeepEqual(got, []any{"STRING", "NUMBER"}) {
		t.Fatalf("oneOf was not rendered as anyOf: %#v", choice)
	}
	if got := field[[]any](t, field[map[string]any](t, properties, "fixed"), "enum"); !reflect.DeepEqual(got, []any{"v1"}) {
		t.Fatalf("const was not rendered as a one-element enum: %#v", got)
	}
	nullable := field[map[string]any](t, properties, "nullable")
	if nullable["type"] != "STRING" || nullable["nullable"] != true {
		t.Fatalf("nullable union was not rendered in Gemini form: %#v", nullable)
	}
}

func TestGoogleToolSchemaRendererPreservesEveryCanonicalConstruct(t *testing.T) {
	// R-2XB1-IV26
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
			"tags": {"type": "array", "items": {"type": "string"}, "minItems": 1},
			"status": {"type": "string", "enum": ["new", "done"]},
			"fixed": {"const": "v1"},
			"choice": {"anyOf": [{"type": "string"}, {"type": "integer"}]},
			"alternative": {"oneOf": [{"type": "boolean"}, {"type": "number"}]},
			"nullable": {"type": ["string", "null"]}
		},
		"required": ["address", "tags", "status", "fixed", "choice", "alternative", "nullable"]
	}`)

	got := convertSchema(canonical)
	properties := field[map[string]any](t, got, "properties")
	address := field[map[string]any](t, properties, "address")
	postalCode := field[map[string]any](t, field[map[string]any](t, address, "properties"), "postal_code")
	for key, want := range map[string]any{
		"pattern":   "^[0-9]{3,5}$",
		"minLength": float64(3),
		"maxLength": float64(5),
		"title":     "Address",
		"default":   "000",
		"format":    "date-time",
	} {
		target := postalCode
		if key == "title" {
			target = address
		}
		if !reflect.DeepEqual(target[key], want) {
			t.Errorf("%s = %#v, want %#v in schema %#v", key, target[key], want, got)
		}
	}
	tags := field[map[string]any](t, properties, "tags")
	if tags["minItems"] != float64(1) || field[map[string]any](t, tags, "items")["type"] != "STRING" {
		t.Errorf("array constructs were not preserved: %#v", tags)
	}
	if enum := field[[]any](t, field[map[string]any](t, properties, "status"), "enum"); !reflect.DeepEqual(enum, []any{"new", "done"}) {
		t.Errorf("enum = %#v, want [new done]", enum)
	}
	if enum := field[[]any](t, field[map[string]any](t, properties, "fixed"), "enum"); !reflect.DeepEqual(enum, []any{"v1"}) {
		t.Errorf("const enum = %#v, want [v1]", enum)
	}
	for _, name := range []string{"choice", "alternative"} {
		if branches := field[[]any](t, field[map[string]any](t, properties, name), "anyOf"); len(branches) != 2 {
			t.Errorf("%s anyOf branches = %#v, want two", name, branches)
		}
	}
	nullable := field[map[string]any](t, properties, "nullable")
	if nullable["type"] != "STRING" || nullable["nullable"] != true {
		t.Errorf("nullable = %#v, want STRING plus nullable true", nullable)
	}
	if got["title"] != "Lookup input" || got["description"] != "Every canonical construct" || address["description"] != "A postal address" {
		t.Errorf("schema metadata was not preserved: %#v", got)
	}
}

func TestGoogleRequestBodyPanicsOnUnknownOutboundBlockType(t *testing.T) {
	// R-4YSE-6YBS
	provider := New(APIKey("test-key"))
	req := &agentkit.Request{
		Model: "gemini-test",
		Messages: []agentkit.Message{{
			Role:   agentkit.RoleUser,
			Blocks: []agentkit.Block{unknownBlock{}},
		}},
	}

	assertUnknownBlockPanic(t, func() {
		_, _, _ = provider.requestBody(req)
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

func TestGoogleReasoningOffIsSentWithoutSubstitutionOrWarning(t *testing.T) {
	var sawDisabled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeRequest(t, r, &body)
		gen := field[map[string]any](t, body, "generationConfig")
		thinking := field[map[string]any](t, gen, "thinkingConfig")
		// R-CUBS-CPMC
		// R-CVJO-QHD1
		if thinking["thinkingBudget"] != float64(0) || thinking["thinkingLevel"] != nil || thinking["includeThoughts"] != false {
			t.Fatalf("DisableReasoning was not sent in Gemini's native off form: %#v", thinking)
		}
		sawDisabled = true
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
	}))
	defer server.Close()

	conv := &agentkit.Conversation{
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "gemini-arbitrary-pro",
		Pricing:  &agentkit.Pricing{},
		Gen:      agentkit.GenSettings{Reasoning: agentkit.DisableReasoning()},
	}
	stream := conv.Send(context.Background(), "hello")
	drainStream(t, stream)
	warnings := stream.Warnings()
	if !sawDisabled || len(warnings) != 0 {
		t.Fatalf("DisableReasoning was substituted or warned: %#v", warnings)
	}
}

func TestGoogleReasoningShapesMapDirectly(t *testing.T) {
	tests := []struct {
		name      string
		reasoning agentkit.ReasoningValue
		want      map[string]any
		wantOmit  bool
	}{
		{name: "unset", wantOmit: true},
		{name: "level", reasoning: agentkit.Level("vendor-native-ultra"), want: map[string]any{"thinkingLevel": "vendor-native-ultra", "includeThoughts": true}},
		{name: "budget", reasoning: agentkit.Budget(7123), want: map[string]any{"thinkingBudget": 7123, "includeThoughts": true}},
		{name: "disabled", reasoning: agentkit.DisableReasoning(), want: map[string]any{"thinkingBudget": 0, "includeThoughts": false}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, warnings, err := New(APIKey("key")).requestBody(&agentkit.Request{
				Model: "same-arbitrary-model-for-every-shape",
				Gen:   agentkit.GenSettings{Reasoning: tc.reasoning},
			})
			if err != nil {
				t.Fatalf("requestBody() error = %v", err)
			}
			// R-CUBS-CPMC
			if len(warnings) != 0 {
				t.Fatalf("reasoning shape emitted warnings: %#v", warnings)
			}
			gen, hasGen := body["generationConfig"].(map[string]any)
			thinking, hasThinking := gen["thinkingConfig"].(map[string]any)
			// R-CVJO-QHD1
			if tc.wantOmit {
				if hasGen || hasThinking {
					t.Fatalf("unset reasoning emitted generation config: %#v", body)
				}
				return
			}
			if !hasThinking || !reflect.DeepEqual(thinking, tc.want) {
				t.Fatalf("thinkingConfig = %#v, want %#v", thinking, tc.want)
			}
			if _, budget := thinking["thinkingBudget"]; budget {
				if _, level := thinking["thinkingLevel"]; level {
					t.Fatalf("Gemini request carried both thinking shapes: %#v", thinking)
				}
			}
		})
	}
}

func TestGoogleRejectsEnableReasoningBeforeTransport(t *testing.T) {
	// R-DDWV-MNZJ
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP transport invoked for unsupported enabled reasoning")
		return nil, nil
	})}
	rt := New(APIKey("key"), WithHTTPClient(client)).RoundTrip(context.Background(), &agentkit.Request{
		Model: "gemini-future",
		Gen:   agentkit.GenSettings{Reasoning: agentkit.EnableReasoning()},
	})
	if !errors.Is(rt.Err(), agentkit.ErrInvalidConfig) {
		t.Fatalf("RoundTrip() error = %v, want ErrInvalidConfig", rt.Err())
	}
}

func TestGoogleVendorRejectsNativeReasoningWithoutSubstitution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeRequest(t, r, &body)
		gen := field[map[string]any](t, body, "generationConfig")
		thinking := field[map[string]any](t, gen, "thinkingConfig")
		if thinking["thinkingLevel"] != "future-level" {
			t.Fatalf("native reasoning value was substituted: %#v", thinking)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":400,"message":"unsupported thinking level","status":"INVALID_ARGUMENT"}}`)
	}))
	defer server.Close()

	rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{
		Model: "never-cataloged-model",
		Gen:   agentkit.GenSettings{Reasoning: agentkit.Level("future-level")},
	})
	// R-CUBS-CPMC
	var providerErr *agentkit.Error
	if !errors.As(rt.Err(), &providerErr) || providerErr.Provider != "google" || !errors.Is(rt.Err(), agentkit.ErrInvalidRequest) {
		t.Fatalf("vendor rejection = %#v, want typed google invalid-request error", rt.Err())
	}
	if len(rt.Warnings()) != 0 {
		t.Fatalf("vendor-rejected reasoning emitted warnings: %#v", rt.Warnings())
	}
}

func TestGoogleProviderOptionsAreShallowMergedOnWire(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		decodeRequest(t, r, &body)
		// R-CXZH-I0UF
		if calls == 1 {
			if body["vendorPreview"] != "verbatim" || !reflect.DeepEqual(body["routing"], map[string]any{"tier": "fast"}) {
				t.Fatalf("provider options were not shallow-merged verbatim: %#v", body)
			}
		} else if _, ok := body["vendorPreview"]; ok {
			t.Fatalf("empty provider options merged an extra key: %#v", body)
		}
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	p := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	for _, options := range []json.RawMessage{json.RawMessage(`{"vendorPreview":"verbatim","routing":{"tier":"fast"}}`), nil} {
		rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: "gemini-test", ProviderOptions: options})
		if err := rt.Err(); err != nil {
			t.Fatalf("RoundTrip() error = %v", err)
		}
	}
	if calls != 2 {
		t.Fatalf("requests = %d, want 2", calls)
	}
}

func TestGoogleUnknownModelVendorErrorIsTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/not-in-any-agentkit-catalog:streamGenerateContent" {
			t.Fatalf("arbitrary model did not reach endpoint verbatim: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":404,"message":"model missing","status":"NOT_FOUND"}}`)
	}))
	defer server.Close()

	var provider agentkit.Provider = New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := provider.RoundTrip(context.Background(), &agentkit.Request{Model: "not-in-any-agentkit-catalog"})
	// R-CT3V-YXVN
	var providerErr *agentkit.Error
	if !errors.As(rt.Err(), &providerErr) || providerErr.Provider != "google" || !errors.Is(rt.Err(), agentkit.ErrNotFound) {
		t.Fatalf("unknown-model rejection = %#v, want typed google not-found error", rt.Err())
	}
}

func TestGoogleSignatureOnFunctionCallPartPreservesToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"city":"Austin"}},"thoughtSignature":"sig-on-call"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{Model: "gemini-test"})
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	// R-DNS8-QC6Z
	if rt.Finish() != agentkit.FinishToolUse {
		t.Fatalf("Finish() = %v, want FinishToolUse", rt.Finish())
	}

	var reasoning agentkit.ReasoningBlock
	var use agentkit.ToolUseBlock
	for _, block := range rt.Message().Blocks {
		switch block := block.(type) {
		case agentkit.ReasoningBlock:
			reasoning = block
		case agentkit.ToolUseBlock:
			use = block
		}
	}
	// R-DNS8-QC6Z
	if use.ID == "" || use.Name != "lookup" || string(use.Input) != `{"city":"Austin"}` {
		t.Fatalf("signature-bearing functionCall did not assemble tool use: %#v", rt.Message().Blocks)
	}
	// R-DTVQ-N6WG
	if reasoning.BoundToID != use.ID || decodeThoughtSignature(reasoning.Opaque) != "sig-on-call" {
		t.Fatalf("signature was not captured and bound to tool use: reasoning=%#v use=%#v", reasoning, use)
	}
}

func TestGoogleParallelReasoningBindsPositionallyToToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"choose weather source","thought":true,"thoughtSignature":"sig-a"},{"functionCall":{"name":"lookup_weather","args":{"city":"Austin"}}},{"text":"choose calendar source","thought":true,"thoughtSignature":"sig-b"},{"functionCall":{"name":"lookup_calendar","args":{"date":"2026-06-25"}}}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{Model: "gemini-test"})
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}

	var reasonings []agentkit.ReasoningBlock
	var uses []agentkit.ToolUseBlock
	for _, block := range rt.Message().Blocks {
		switch block := block.(type) {
		case agentkit.ReasoningBlock:
			reasonings = append(reasonings, block)
		case agentkit.ToolUseBlock:
			uses = append(uses, block)
		}
	}

	// R-IPGC-I69W
	if len(reasonings) != 2 || len(uses) != 2 {
		t.Fatalf("blocks = %#v, want two reasoning blocks and two tool-use blocks", rt.Message().Blocks)
	}
	if uses[0].ID == "" || uses[1].ID == "" || uses[0].ID == uses[1].ID {
		t.Fatalf("tool-use IDs were not distinct non-empty values: %#v", uses)
	}
	if decodeThoughtSignature(reasonings[0].Opaque) != "sig-a" || reasonings[0].BoundToID != uses[0].ID {
		t.Fatalf("reasoning A was not bound to call A: reasoning=%#v useA=%#v", reasonings[0], uses[0])
	}
	if decodeThoughtSignature(reasonings[1].Opaque) != "sig-b" || reasonings[1].BoundToID != uses[1].ID {
		t.Fatalf("reasoning B was not bound to call B: reasoning=%#v useB=%#v", reasonings[1], uses[1])
	}
	if reasonings[0].BoundToID == uses[1].ID {
		t.Fatalf("reasoning A incorrectly bound to call B: reasoning=%#v useB=%#v", reasonings[0], uses[1])
	}
}

func TestGoogleSignatureOnTextPartPreservesVisibleText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"visible answer","thoughtSignature":"sig-on-text"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{Model: "gemini-test"})
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}

	var sawText bool
	for _, block := range rt.Message().Blocks {
		switch block := block.(type) {
		case agentkit.TextBlock:
			if block.Text == "visible answer" {
				sawText = true
			}
		case agentkit.ReasoningBlock:
			if block.Summary != "" {
				t.Fatalf("visible text was captured as reasoning summary: %#v", block)
			}
		}
	}
	// R-DRFX-VNF2
	if !sawText {
		t.Fatalf("message did not persist visible text block: %#v", rt.Message().Blocks)
	}
}

func TestGoogleSSETextAssemblyIsIndependentOfFrameBoundaries(t *testing.T) {
	// R-QUWY-MLCD
	multiFrame := parseGoogleSSEMessage(t,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"I ha"}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"ve access"}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":" to tools."}]},"finishReason":"STOP"}]}`,
	)
	singleFrame := parseGoogleSSEMessage(t,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"I ha"},{"text":"ve access"},{"text":" to tools."}]},"finishReason":"STOP"}]}`,
	)

	want := agentkit.Message{
		Role:   agentkit.RoleAssistant,
		Blocks: []agentkit.Block{agentkit.TextBlock{Text: "I have access to tools."}},
	}
	if !reflect.DeepEqual(multiFrame, want) {
		t.Fatalf("multi-frame message = %#v, want %#v", multiFrame, want)
	}
	if !reflect.DeepEqual(multiFrame, singleFrame) {
		t.Fatalf("transport-equivalent messages differ:\nmulti:  %#v\nsingle: %#v", multiFrame, singleFrame)
	}
}

func TestGoogleSSEAdjacentTextPartsMergeWithinFrame(t *testing.T) {
	// R-QW4V-0D32
	message := parseGoogleSSEMessage(t,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"joined"},{"text":" verbatim"}]},"finishReason":"STOP"}]}`,
	)

	want := []agentkit.Block{agentkit.TextBlock{Text: "joined verbatim"}}
	if !reflect.DeepEqual(message.Blocks, want) {
		t.Fatalf("blocks = %#v, want %#v", message.Blocks, want)
	}
}

func TestGoogleSSEBindsThoughtSignatureAcrossFrames(t *testing.T) {
	// R-QXCR-E4TR
	message := parseGoogleSSEMessage(t,
		`{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"thoughtSignature":"cross-frame-signature"}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"city":"Austin"}}}]},"finishReason":"STOP"}]}`,
	)

	if len(message.Blocks) != 2 {
		t.Fatalf("blocks = %#v, want reasoning followed by tool use", message.Blocks)
	}
	reasoning, reasoningOK := message.Blocks[0].(agentkit.ReasoningBlock)
	use, useOK := message.Blocks[1].(agentkit.ToolUseBlock)
	if !reasoningOK || !useOK {
		t.Fatalf("block types = %T, %T, want ReasoningBlock, ToolUseBlock", message.Blocks[0], message.Blocks[1])
	}
	if use.ID == "" || reasoning.BoundToID != use.ID {
		t.Fatalf("cross-frame reasoning was not bound to tool use: reasoning=%#v use=%#v", reasoning, use)
	}
	if got := decodeThoughtSignature(reasoning.Opaque); got != "cross-frame-signature" {
		t.Fatalf("thought signature = %q, want cross-frame-signature", got)
	}
}

func TestGoogleSSETextRunsRemainSeparatedByToolUse(t *testing.T) {
	// R-QYKN-RWKG
	message := parseGoogleSSEMessage(t,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"before"}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}}}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"after"}]},"finishReason":"STOP"}]}`,
	)

	if len(message.Blocks) != 3 {
		t.Fatalf("blocks = %#v, want exactly text, tool use, text", message.Blocks)
	}
	before, beforeOK := message.Blocks[0].(agentkit.TextBlock)
	use, useOK := message.Blocks[1].(agentkit.ToolUseBlock)
	after, afterOK := message.Blocks[2].(agentkit.TextBlock)
	if !beforeOK || !useOK || !afterOK {
		t.Fatalf("block types = %T, %T, %T, want TextBlock, ToolUseBlock, TextBlock", message.Blocks[0], message.Blocks[1], message.Blocks[2])
	}
	if before.Text != "before" || use.Name != "lookup" || after.Text != "after" {
		t.Fatalf("blocks lost order or content: %#v", message.Blocks)
	}
}

func TestGoogleReplayedToolUsePlacesThoughtSignatureOnPart(t *testing.T) {
	var calls int32
	var sawReplay bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		var body map[string]any
		decodeRequest(t, r, &body)

		switch call {
		case 1:
			writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"city":"Austin"}},"thoughtSignature":"sig-on-call"}]},"finishReason":"STOP"}]}`)
		case 2:
			contents := field[[]any](t, body, "contents")
			part := findFunctionCallPart(t, contents, "lookup")
			// R-GSIG-PT07
			if part["thoughtSignature"] != "sig-on-call" {
				t.Fatalf("thoughtSignature was not serialized at part level: %#v", part)
			}
			call := field[map[string]any](t, part, "functionCall")
			// R-GSIG-PT07
			if _, ok := call["thoughtSignature"]; ok {
				t.Fatalf("thoughtSignature was nested inside functionCall: %#v", call)
			}
			for key := range call {
				if key != "name" && key != "args" && key != "id" {
					t.Fatalf("functionCall carried unexpected key %q in %#v", key, call)
				}
			}
			sawReplay = true
			writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`)
		default:
			t.Fatalf("unexpected request %d", call)
		}
	}))
	defer server.Close()

	tool := testRawTool("lookup", "look up weather", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		return "sunny", nil
	})
	conv := &agentkit.Conversation{
		Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "gemini-test",
		Tools:    []agentkit.Tool{tool},
	}
	drainStream(t, conv.Send(context.Background(), "weather"))
	if !sawReplay {
		t.Fatalf("replay request was not inspected")
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

			rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{Model: "gemini-test"})
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

func TestGoogleEmbedderBatchesUsageOrderAndRequestShape(t *testing.T) {
	var provider agentkit.EmbeddingProvider
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-embedding-001:batchEmbedContents" {
			t.Errorf("path = %s, want :batchEmbedContents endpoint", r.URL.Path)
		}
		if got := r.Header.Get("X-Goog-Api-Key"); got != "test-key" {
			t.Errorf("X-Goog-Api-Key = %q", got)
		}
		var body map[string]any
		decodeRequest(t, r, &body)
		requests = append(requests, body)

		items := field[[]any](t, body, "requests")
		embeddings := make([]map[string]any, len(items))
		for i, raw := range items {
			item := raw.(map[string]any)
			if item["model"] != "models/gemini-embedding-001" || item["autoTruncate"] != false {
				t.Fatalf("request item = %#v, want model and autoTruncate:false", item)
			}
			if item["taskType"] != "RETRIEVAL_QUERY" || item["outputDimensionality"] != float64(128) {
				t.Fatalf("request item task/dimensions = %#v", item)
			}
			content := field[map[string]any](t, item, "content")
			parts := field[[]any](t, content, "parts")
			input := field[string](t, parts[0].(map[string]any), "text")
			n := googleEmbeddingInputNumber(input)
			vector := make([]float32, 128)
			vector[0], vector[1] = float32(n+1), 1
			embeddings[i] = map[string]any{"values": vector}
		}
		writeGoogleEmbeddingResponse(t, w, embeddings, int64(len(items)))
	}))
	defer server.Close()

	provider = NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	inputs := make([]string, 101)
	for i := range inputs {
		inputs[i] = fmt.Sprintf("input-%03d", i)
	}
	embedder := &agentkit.Embedder{Provider: provider, Model: EmbedModelGemini001, Dimensions: 128}

	result, err := embedder.Embed(context.Background(), inputs, agentkit.InputQuery)
	// R-D7QO-K6RZ, R-YGQZ-C8S2, R-YJ6S-3S9G, R-YPAA-0MYX
	if err != nil {
		t.Fatalf("Embed() error = %v, want nil", err)
	}
	if len(result.Vectors) != len(inputs) {
		t.Fatalf("vectors = %d, want %d", len(result.Vectors), len(inputs))
	}
	if got, want := result.Usage(), (agentkit.EmbeddingUsage{InputTokens: 101, Total: 101}); got != want {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}
	for _, index := range []int{0, 99, 100} {
		wantFirst := float64(index+1) / math.Sqrt(float64((index+1)*(index+1)+1))
		if got := float64(result.Vectors[index][0]); math.Abs(got-wantFirst) > 1e-6 {
			t.Fatalf("vector[%d][0] = %v, want %v", index, got, wantFirst)
		}
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	firstItems := field[[]any](t, requests[0], "requests")
	secondItems := field[[]any](t, requests[1], "requests")
	if len(firstItems) != 100 || len(secondItems) != 1 {
		t.Fatalf("chunk sizes = %d/%d, want 100/1", len(firstItems), len(secondItems))
	}
}

func TestGoogleEmbeddingInputTypes(t *testing.T) {
	var taskTypes []any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeRequest(t, r, &body)
		item := field[[]any](t, body, "requests")[0].(map[string]any)
		taskTypes = append(taskTypes, item["taskType"])
		vector := make([]float32, 128)
		vector[0], vector[1] = 3, 4
		writeGoogleEmbeddingResponse(t, w, []map[string]any{{"values": vector}}, 1)
	}))
	defer server.Close()

	embedder := &agentkit.Embedder{
		Provider:   NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:      EmbedModelGemini001,
		Dimensions: 128,
	}
	for _, role := range []agentkit.InputType{agentkit.InputUnspecified, agentkit.InputQuery, agentkit.InputDocument} {
		if _, err := embedder.Embed(context.Background(), []string{"hello"}, role); err != nil {
			t.Fatalf("Embed(%v) error = %v", role, err)
		}
	}

	// R-YLMK-VBQU
	if got, want := taskTypes, []any{nil, "RETRIEVAL_QUERY", "RETRIEVAL_DOCUMENT"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("taskTypes = %#v, want %#v", got, want)
	}
}

func TestGoogleEmbeddingContextLengthError(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		decodeRequest(t, r, &body)
		item := field[[]any](t, body, "requests")[0].(map[string]any)
		if item["autoTruncate"] != false {
			t.Fatalf("autoTruncate = %#v, want false", item["autoTruncate"])
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"code":400,"message":"token limit exceeded","status":"INVALID_ARGUMENT"}}`)
	}))
	defer server.Close()

	tooLong := &agentkit.Embedder{
		Provider: NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    EmbedModelGemini001,
	}
	result, err := tooLong.Embed(context.Background(), []string{"hello"}, agentkit.InputQuery)
	// R-YKEO-HK05
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !errors.Is(err, agentkit.ErrContextLength) {
		t.Fatalf("context error = %v, want ErrContextLength", err)
	}
	if calls != 1 {
		t.Fatalf("calls after context error = %d, want 1", calls)
	}
}

func TestGoogleEmbeddingDimensionsAndRetryPolicy(t *testing.T) {
	t.Run("dimensions", func(t *testing.T) {
		var requests []map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			decodeRequest(t, r, &body)
			requests = append(requests, body)
			item := field[[]any](t, body, "requests")[0].(map[string]any)
			dimensions := 3072
			if raw, ok := item["outputDimensionality"].(float64); ok {
				dimensions = int(raw)
			}
			vector := make([]float32, dimensions)
			vector[0] = 3
			vector[1] = 4
			writeGoogleEmbeddingResponse(t, w, []map[string]any{{"values": vector}}, 1)
		}))
		defer server.Close()

		embedder := &agentkit.Embedder{
			Provider: NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
			Model:    EmbedModelGemini001,
		}
		native, err := embedder.Embed(context.Background(), []string{"native"}, agentkit.InputUnspecified)
		if err != nil {
			t.Fatalf("native Embed() error = %v", err)
		}
		if len(native.Vectors[0]) != 3072 {
			t.Fatalf("native vector dimension = %d, want 3072", len(native.Vectors[0]))
		}

		embedder.Dimensions = 128
		produced, err := embedder.Embed(context.Background(), []string{"small"}, agentkit.InputDocument)
		// R-YBVD-T5TA
		if err != nil {
			t.Fatalf("dimensioned Embed() error = %v, want nil", err)
		}
		if len(produced.Vectors[0]) != 128 {
			t.Fatalf("dimensioned vector dimension = %d, want 128", len(produced.Vectors[0]))
		}
		first := field[[]any](t, requests[0], "requests")[0].(map[string]any)
		second := field[[]any](t, requests[1], "requests")[0].(map[string]any)
		if _, ok := first["outputDimensionality"]; ok {
			t.Fatalf("native request carried dimensions: %#v", first)
		}
		if second["outputDimensionality"] != float64(128) {
			t.Fatalf("dimensioned request = %#v, want outputDimensionality=128", second)
		}
	})

	t.Run("retryable and non-retryable", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"error":{"code":500,"message":"try again","status":"INTERNAL"}}`)
				return
			}
			if calls == 3 {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"code":400,"message":"bad input","status":"INVALID_ARGUMENT"}}`)
				return
			}
			writeGoogleEmbeddingResponse(t, w, []map[string]any{{"values": []float32{1, 0}}}, 1)
		}))
		defer server.Close()

		clock := &fakeGoogleEmbeddingClock{now: time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)}
		provider := NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).(*embeddingProvider)
		provider.clock = clock
		embedder := &agentkit.Embedder{
			Provider: provider,
			Model:    EmbedModelGemini001,
			Retry:    agentkit.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		}

		_, err := embedder.Embed(context.Background(), []string{"hello"}, agentkit.InputQuery)
		// R-YO2D-MV88
		if err != nil {
			t.Fatalf("retryable Embed() error = %v, want nil", err)
		}
		if calls != 2 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Millisecond}) {
			t.Fatalf("calls/sleeps = %d/%v, want 2/[1ms]", calls, clock.sleeps)
		}

		_, err = embedder.Embed(context.Background(), []string{"bad"}, agentkit.InputQuery)
		// R-YO2D-MV88
		if !errors.Is(err, agentkit.ErrInvalidRequest) {
			t.Fatalf("non-retryable Embed() error = %v, want ErrInvalidRequest", err)
		}
		if calls != 3 || len(clock.sleeps) != 1 {
			t.Fatalf("calls/sleeps after non-retryable = %d/%v, want 3/[1ms]", calls, clock.sleeps)
		}
	})
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

	rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{
		Model: "gemini-test",
		Messages: []agentkit.Message{{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{
				agentkit.TextBlock{Text: "prior"},
				agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"encrypted_content":"foreign"}`), Summary: "drop me"},
			},
		}},
	})
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	if !sawRequest {
		t.Fatalf("server did not receive request")
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
		Tools: []agentkit.Tool{testRawTool("lookup", "lookup", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
			return "tool output", nil
		})},
	}
	drainStream(t, conv.Send(context.Background(), "first"))
	if first.calls != 2 {
		t.Fatalf("scripted first provider calls = %d, want 2", first.calls)
	}

	conv.Provider = New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	conv.Model = "gemini-test"
	drainStream(t, conv.Send(context.Background(), "second"))
	if !sawPriorToolBlocks || !sawNoForeignReasoning {
		t.Fatalf("switch assertions did not run")
	}
}

type scriptedProvider struct {
	calls int
}

func (p *scriptedProvider) Identity() agentkit.Identity {
	return agentkit.Identity{Provider: "scripted", Auth: agentkit.AuthAPIKey}
}

func (p *scriptedProvider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	p.calls++
	switch p.calls {
	case 1:
		msg := agentkit.Message{Role: agentkit.RoleAssistant, Blocks: []agentkit.Block{
			agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"encrypted_content":"foreign"}`), Summary: "foreign", BoundToID: "abc_123"},
			agentkit.ToolUseBlock{ID: "abc_123", Name: "lookup", Input: json.RawMessage(`{"q":"x"}`)},
		}}
		return agentkit.NewRoundTrip(msg, agentkit.FinishToolUse, agentkit.Usage{}, nil, nil, 0, false)
	default:
		msg := agentkit.Message{Role: agentkit.RoleAssistant, Blocks: []agentkit.Block{agentkit.TextBlock{Text: "first done"}}}
		return agentkit.NewRoundTrip(msg, agentkit.FinishStop, agentkit.Usage{}, nil, nil, 0, false)
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

func writeSSE(t *testing.T, w http.ResponseWriter, data ...string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, frame := range data {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", frame); err != nil {
			t.Fatalf("write sse: %v", err)
		}
	}
}

func parseGoogleSSEMessage(t *testing.T, data ...string) agentkit.Message {
	t.Helper()
	response := httptest.NewRecorder()
	writeSSE(t, response, data...)
	message, _, _, err := parseResponse(response.Header().Get("Content-Type"), response.Body.Bytes())
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}
	return message
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

func googleEmbeddingInputNumber(input string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(input, "input-"))
	if err != nil {
		return 0
	}
	return n
}

func writeGoogleEmbeddingResponse(t *testing.T, w http.ResponseWriter, embeddings []map[string]any, promptTokens int64) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"embeddings": embeddings,
		"usageMetadata": map[string]int64{
			"promptTokenCount": promptTokens,
		},
	}); err != nil {
		t.Fatalf("write embedding response: %v", err)
	}
}

type fakeGoogleEmbeddingClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *fakeGoogleEmbeddingClock) Now() time.Time {
	return c.now
}

func (c *fakeGoogleEmbeddingClock) Sleep(ctx context.Context, delay time.Duration) error {
	c.sleeps = append(c.sleeps, delay)
	if err := ctx.Err(); err != nil {
		return err
	}
	c.now = c.now.Add(delay)
	return nil
}

func (c *fakeGoogleEmbeddingClock) Jitter(cap time.Duration) time.Duration {
	return cap
}

func findFunctionCallPart(t *testing.T, contents []any, name string) map[string]any {
	t.Helper()
	for _, contentValue := range contents {
		content, ok := contentValue.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for _, partValue := range parts {
			part, ok := partValue.(map[string]any)
			if !ok {
				continue
			}
			call, ok := part["functionCall"].(map[string]any)
			if ok && call["name"] == name {
				return part
			}
		}
	}
	t.Fatalf("did not find functionCall part named %q in %#v", name, contents)
	return nil
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
