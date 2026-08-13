package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/openaicompat"
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

type staticTokenSource struct {
	bearer  string
	account string
	err     error
}

func (s staticTokenSource) Token(context.Context) (string, string, error) {
	return s.bearer, s.account, s.err
}

func TestBuildRequestToolsSortedByName(t *testing.T) {
	// R-XY0O-DBX8
	alpha := testRawTool("alpha", "first", json.RawMessage(`{"type":"object"}`), nil)
	zulu := testRawTool("zulu", "last", json.RawMessage(`{"type":"object"}`), nil)
	provider := &Provider{}

	buildTools := func(tools []agentkit.Tool) ([]byte, []toolDef) {
		t.Helper()
		request, _, err := provider.buildRequest(&agentkit.Request{Model: "gpt-test", Tools: tools})
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
	if string(forward) != string(reversed) {
		t.Fatalf("serialized tools differ by input order:\nforward: %s\nreverse: %s", forward, reversed)
	}
	if got := []string{tools[0].Name, tools[1].Name}; !reflect.DeepEqual(got, []string{"alpha", "zulu"}) {
		t.Fatalf("tool order = %#v, want [alpha zulu]", got)
	}
}

func TestProviderOmitsStrictAndPreservesCanonicalOptionality(t *testing.T) {
	schema := json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$defs":{"unused":{"type":"string"}},
		"type":"object",
		"properties":{
			"query":{"type":"string"},
			"limit":{"type":"integer"},
			"filter":{
				"type":"object",
				"properties":{"field":{"type":"string"},"exact":{"type":"boolean"}},
				"required":["field"]
			}
		},
		"required":["query","filter"]
	}`)
	var recorded map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&recorded); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	provider.RoundTrip(context.Background(), &agentkit.Request{
		Model: "gpt-test",
		Tools: []agentkit.Tool{testRawTool(
			"search",
			"search records",
			schema,
			nil,
		)},
	})

	tools, ok := recorded["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("recorded tools = %#v, want one tool", recorded["tools"])
	}
	tool := tools[0].(map[string]any)
	if _, exists := tool["strict"]; exists {
		t.Fatalf("tool unexpectedly contains strict: %#v", tool)
	}
	parameters := tool["parameters"].(map[string]any)
	assertClosedObjectSchemas(t, parameters)
	assertNoSchemaMetadata(t, parameters)
	if got, want := parameters["required"], []any{"query", "filter"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required = %#v, want %#v", got, want)
	}

	properties := parameters["properties"].(map[string]any)
	if got := properties["limit"].(map[string]any)["type"]; got != "integer" {
		t.Fatalf("optional limit type = %#v, want integer", got)
	}
	filter := properties["filter"].(map[string]any)
	if got, want := filter["required"], []any{"field"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("nested required = %#v, want %#v", got, want)
	}
	exact := filter["properties"].(map[string]any)["exact"].(map[string]any)
	if got := exact["type"]; got != "boolean" {
		t.Fatalf("nested optional exact type = %#v, want boolean", got)
	}
}

func TestOpenAIRendererPreservesConstructCompleteSchemaAndMatchesCompat(t *testing.T) {
	// R-836K-ZH1P
	// R-84EH-D8SE
	schema := constructCompleteOpenAISchema()
	request, _, err := (&Provider{}).buildRequest(&agentkit.Request{
		Model: "gpt-test",
		Tools: []agentkit.Tool{testRawTool(
			"inspect",
			"inspect a record",
			schema,
			nil,
		)},
	})
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(request.Tools) != 1 {
		t.Fatalf("rendered tools = %#v, want one tool", request.Tools)
	}
	parameters := request.Tools[0].Parameters
	assertClosedObjectSchemas(t, parameters)
	assertNoSchemaMetadata(t, parameters)

	properties := parameters["properties"].(map[string]any)
	name := properties["name"].(map[string]any)
	for key, want := range map[string]any{
		"pattern":     "^[a-z]+$",
		"minLength":   float64(2),
		"maxLength":   float64(20),
		"format":      "email",
		"default":     "agent",
		"title":       "Name",
		"description": "display name",
	} {
		if got := name[key]; !reflect.DeepEqual(got, want) {
			t.Errorf("name.%s = %#v, want %#v", key, got, want)
		}
	}
	if got := properties["state"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []any{"new", "done"}) {
		t.Errorf("state enum = %#v", got)
	}
	if got := properties["kind"].(map[string]any)["const"]; got != "record" {
		t.Errorf("kind const = %#v, want record", got)
	}
	tags := properties["tags"].(map[string]any)
	if tags["minItems"] != float64(1) || !reflect.DeepEqual(tags["items"].(map[string]any)["enum"], []any{"red", "blue"}) {
		t.Errorf("tags schema = %#v", tags)
	}
	choice := properties["choice"].(map[string]any)
	if _, exists := choice["oneOf"]; exists {
		t.Error("choice retained oneOf")
	}
	if got, ok := choice["anyOf"].([]any); !ok || len(got) != 2 {
		t.Errorf("choice anyOf = %#v, want two branches", choice["anyOf"])
	}
	if got, ok := properties["either"].(map[string]any)["anyOf"].([]any); !ok || len(got) != 2 {
		t.Errorf("either anyOf = %#v, want two branches", properties["either"])
	}
	address := properties["address"].(map[string]any)
	if _, exists := address["$ref"]; exists {
		t.Error("address retained $ref")
	}
	if got := address["properties"].(map[string]any)["zip"].(map[string]any)["pattern"]; got != "^[0-9]{5}$" {
		t.Errorf("inlined address zip pattern = %#v", got)
	}
	if got := properties["optional"].(map[string]any)["type"]; got != "integer" {
		t.Errorf("optional type = %#v, want integer", got)
	}
	if required := stringSliceSet(parameters["required"]); required["optional"] {
		t.Errorf("optional property appears in required: %#v", parameters["required"])
	}
	if got := properties["nullable"].(map[string]any)["type"]; !reflect.DeepEqual(got, []any{"string", "null"}) {
		t.Errorf("nullable type = %#v, want [string null]", got)
	}

	var compatBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode compat request: %v", err)
		}
		compatBodies = append(compatBodies, body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	for _, identity := range []agentkit.Identity{
		{Provider: agentkit.ProviderZAI, Auth: agentkit.AuthAPIKey},
		{Provider: agentkit.ProviderOpenRouter, Auth: agentkit.AuthAPIKey},
	} {
		compat := openaicompat.New(openaicompat.Config{
			Identity:   identity,
			BaseURL:    server.URL,
			APIKey:     "test-key",
			HTTPClient: server.Client(),
		})
		compat.RoundTrip(context.Background(), &agentkit.Request{
			Model: "compat-test",
			Tools: []agentkit.Tool{testRawTool(
				"inspect",
				"inspect a record",
				schema,
				nil,
			)},
		})
	}

	openAIToolRaw, err := json.Marshal(request.Tools[0])
	if err != nil {
		t.Fatalf("marshal OpenAI tool: %v", err)
	}
	var openAITool map[string]any
	if err := json.Unmarshal(openAIToolRaw, &openAITool); err != nil {
		t.Fatalf("decode OpenAI tool: %v", err)
	}
	if _, exists := openAITool["strict"]; exists {
		t.Fatalf("OpenAI tool unexpectedly contains strict: %#v", openAITool)
	}
	delete(openAITool, "type")
	if len(compatBodies) != 2 {
		t.Fatalf("compat request count = %d, want 2", len(compatBodies))
	}
	for i, compatBody := range compatBodies {
		compatTools := compatBody["tools"].([]any)
		compatFunction := compatTools[0].(map[string]any)["function"].(map[string]any)
		if _, exists := compatFunction["strict"]; exists {
			t.Fatalf("compat function %d unexpectedly contains strict: %#v", i, compatFunction)
		}
		if !reflect.DeepEqual(openAITool, compatFunction) {
			t.Fatalf("OpenAI tool payload differs from compat request %d:\nOpenAI: %#v\ncompat: %#v", i, openAITool, compatFunction)
		}
	}
}

func constructCompleteOpenAISchema() json.RawMessage {
	return json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$defs":{
			"address":{
				"type":"object",
				"title":"Address",
				"properties":{"zip":{"type":"string","pattern":"^[0-9]{5}$"}},
				"required":["zip"]
			}
		},
		"type":"object",
		"description":"construct-complete",
		"properties":{
			"name":{"type":"string","pattern":"^[a-z]+$","minLength":2,"maxLength":20,"format":"email","default":"agent","title":"Name","description":"display name"},
			"state":{"type":"string","enum":["new","done"]},
			"kind":{"type":"string","const":"record"},
			"tags":{"type":"array","items":{"type":"string","enum":["red","blue"]},"minItems":1},
			"choice":{"oneOf":[{"type":"string","const":"left"},{"type":"integer","const":2}]},
			"either":{"anyOf":[{"type":"string"},{"type":"number"}]},
			"address":{"$ref":"#/$defs/address"},
			"optional":{"type":"integer"},
			"nullable":{"type":["string","null"]},
			"nested":{"type":"object","properties":{"required":{"type":"string"},"optional":{"type":"boolean"}},"required":["required"]}
		},
		"required":["name","state","kind","tags","choice","either","address","nullable","nested"]
	}`)
}

func assertClosedObjectSchemas(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		if _, ok := value["properties"].(map[string]any); ok {
			if value["additionalProperties"] != false {
				t.Errorf("object additionalProperties = %#v, want false", value["additionalProperties"])
			}
		}
		for _, child := range value {
			assertClosedObjectSchemas(t, child)
		}
	case []any:
		for _, child := range value {
			assertClosedObjectSchemas(t, child)
		}
	}
}

func stringSliceSet(value any) map[string]bool {
	set := make(map[string]bool)
	for _, item := range value.([]any) {
		set[item.(string)] = true
	}
	return set
}

func assertNoSchemaMetadata(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for _, key := range []string{"$schema", "$defs", "$ref", "oneOf"} {
			if _, exists := value[key]; exists {
				t.Errorf("rendered schema retained %s in %#v", key, value)
			}
		}
		for _, child := range value {
			assertNoSchemaMetadata(t, child)
		}
	case []any:
		for _, child := range value {
			assertNoSchemaMetadata(t, child)
		}
	}
}

func TestEmbeddingProviderReportsSameOpenAIAPIKeyIdentityAsChat(t *testing.T) {
	// R-LL8D-N2OD
	chatIdentity := New(APIKey("key")).Identity()
	embeddingIdentity := NewEmbedder(APIKey("key")).Identity()
	want := agentkit.Identity{Provider: agentkit.ProviderOpenAI, Auth: agentkit.AuthAPIKey}
	if chatIdentity != want || embeddingIdentity != want {
		t.Fatalf("chat identity = %#v, embedding identity = %#v, want %#v", chatIdentity, embeddingIdentity, want)
	}
	if embeddingIdentity.Auth == "" {
		t.Fatal("embedding Identity().Auth is empty")
	}
}

func TestSubscriptionAndAPIKeyCredentialsSelectTransport(t *testing.T) {
	// R-DE1J-AWJ6
	// R-DF9F-OO9V
	tests := []struct {
		name         string
		credential   Credential
		wantPath     string
		wantBearer   string
		subscription bool
	}{
		{name: "subscription", credential: Subscription(staticTokenSource{bearer: "living-token", account: "acct-123"}), wantPath: "/backend-api/codex/responses", wantBearer: "living-token", subscription: true},
		{name: "api key", credential: APIKey("platform-key"), wantPath: "/v1/responses", wantBearer: "platform-key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, tt.wantPath)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer "+tt.wantBearer {
					t.Errorf("Authorization = %q", got)
				}
				wantHeaders := map[string]string{
					"chatgpt-account-id": "acct-123",
					"originator":         "codex_cli_rs",
					"OpenAI-Beta":        "responses=experimental",
				}
				for name, want := range wantHeaders {
					got := r.Header.Get(name)
					if tt.subscription && got != want {
						t.Errorf("%s = %q, want %q", name, got, want)
					}
					if !tt.subscription && got != "" {
						t.Errorf("API key request unexpectedly carried %s = %q", name, got)
					}
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if body["store"] != false || body["instructions"] != "follow system" || fmt.Sprint(body["include"]) != "[reasoning.encrypted_content]" {
					t.Errorf("fixed Responses body fields = %#v", body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, textOnlySSE("ok", 1, 0, 1, 0))
			}))
			defer server.Close()

			conversation := &agentkit.Conversation{
				Provider: New(tt.credential, WithBaseURL(server.URL), WithHTTPClient(server.Client())),
				Model:    "gpt-test",
				System:   "follow system",
				Pricing:  &agentkit.Pricing{},
			}
			// R-DG9Z-8KYU
			stream := conversation.Send(context.Background(), "hello")
			for range stream.Events() {
			}
			if err := stream.Err(); err != nil {
				t.Fatalf("Send: %v", err)
			}
		})
	}
}

func TestCredentialModesSelectIdentityAndProviderErrorAttribution(t *testing.T) {
	tests := []struct {
		name       string
		credential Credential
		wantAuth   agentkit.AuthMode
	}{
		{name: "api key", credential: APIKey("key"), wantAuth: agentkit.AuthAPIKey},
		{name: "subscription", credential: Subscription(staticTokenSource{bearer: "token", account: "account"}), wantAuth: agentkit.AuthSubscription},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"message":"bad request","type":"invalid_request_error"}}`)
			}))
			defer server.Close()
			provider := New(tt.credential, WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			// R-DL5K-RNXM
			// R-LK0H-9AXO
			wantIdentity := agentkit.Identity{Provider: agentkit.ProviderOpenAI, Auth: tt.wantAuth}
			if got := provider.Identity(); got != wantIdentity {
				t.Fatalf("Identity() = %#v, want %#v", got, wantIdentity)
			}
			rt := provider.RoundTrip(context.Background(), &agentkit.Request{Model: "gpt-test"})
			var providerErr *agentkit.Error
			if !errors.As(rt.Err(), &providerErr) || providerErr.Provider != agentkit.ProviderOpenAI || providerErr.Auth != tt.wantAuth {
				t.Fatalf("provider error = %#v, want identity %#v", rt.Err(), wantIdentity)
			}
		})
	}
}

func TestMissingChatCredentialsFailAtSendWithoutTransport(t *testing.T) {
	// R-UIV2-G7ID
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	tests := []struct {
		name       string
		credential Credential
		want       string
	}{
		{name: "API key", credential: APIKey(""), want: "openai: API key is absent"},
		{name: "subscription", credential: Subscription(nil), want: "openai: ChatGPT subscription token source is absent"},
	}
	var messages []string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := New(tt.credential, WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			if provider == nil {
				t.Fatal("New() returned nil")
			}
			conversation := &agentkit.Conversation{
				Provider: provider,
				Model:    "gpt-test",
				Pricing:  &agentkit.Pricing{},
			}
			stream := conversation.Send(context.Background(), "hello")
			for range stream.Events() {
			}
			err := stream.Err()
			if !errors.Is(err, agentkit.ErrMissingCredential) {
				t.Fatalf("Send() error = %v, want ErrMissingCredential", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Send() error = %q, want message containing %q", err, tt.want)
			}
			messages = append(messages, err.Error())
		})
	}
	if messages[0] == messages[1] {
		t.Fatalf("credential errors are identical: %q", messages[0])
	}
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want 0", requests)
	}

	var nilProvider *Provider
	if err := nilProvider.RoundTrip(context.Background(), &agentkit.Request{}).Err(); !errors.Is(err, agentkit.ErrInvalidConfig) {
		t.Fatalf("nil provider error = %v, want ErrInvalidConfig", err)
	}
	if err := New(APIKey("key")).RoundTrip(context.Background(), nil).Err(); !errors.Is(err, agentkit.ErrInvalidConfig) {
		t.Fatalf("nil request error = %v, want ErrInvalidConfig", err)
	}
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
	tool := testRawTool("weather", "get weather", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		called = true
		if string(input) != `{"city":"Paris"}` {
			t.Fatalf("tool input = %s", input)
		}
		return "sunny", nil
	})
	c := &agentkit.Conversation{
		Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "gpt-5.5",
		System:   "Be terse.",
		Gen: agentkit.GenSettings{
			Temperature: &temperature,
			TopP:        &topP,
			MaxTokens:   128,
			Reasoning:   agentkit.Level("low"),
		},
		Tools: []agentkit.Tool{tool},
	}

	// R-XR4M-U1ZT, R-XW08-D4YL, R-C8UE-VJ67,
	// R-P5U3-5CFZ, R-T40A-VZQ7
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
	provider := New(APIKey("test-key"))
	req := &agentkit.Request{
		Model: "gpt-5.5",
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
		Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "gpt-5.5",
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

func TestNewAPIKeyAuthenticatesResponsesRequest(t *testing.T) {
	var provider agentkit.Provider = New(APIKey("secret"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want bearer credential", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, textOnlySSE("ok", 1, 0, 1, 0))
	}))
	defer server.Close()
	provider = New(APIKey("secret"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))

	// R-CQO3-7EE9
	stream := (&agentkit.Conversation{Provider: provider, Model: "any-model", Pricing: &agentkit.Pricing{}}).Send(context.Background(), "hello")
	for range stream.Events() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestUncatalogedModelFlowsToWireAndVendorErrorIsTyped(t *testing.T) {
	const model = "future-model-never-cataloged"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != model {
			t.Errorf("model = %#v, want %q", body["model"], model)
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"unknown model","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	p := New(APIKey("secret"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	// R-CT3V-YXVN
	rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: model})
	var providerErr *agentkit.Error
	if !errors.As(rt.Err(), &providerErr) || providerErr.Provider != agentkit.ProviderOpenAI || providerErr.Auth != agentkit.AuthAPIKey || providerErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("error = %#v, want typed OpenAI 400", rt.Err())
	}
}

func TestReasoningLowersByShapeWithoutModelKnowledge(t *testing.T) {
	tests := []struct {
		name       string
		value      agentkit.ReasoningValue
		wantEffort string
	}{
		{name: "level", value: agentkit.Level("brand-new-effort"), wantEffort: "brand-new-effort"},
		{name: "disabled", value: agentkit.DisableReasoning(), wantEffort: "none"},
		{name: "unset"},
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

			p := New(APIKey("secret"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: "unknown-" + tt.name, Gen: agentkit.GenSettings{Reasoning: tt.value}})
			if err := rt.Err(); err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			if warnings := rt.Warnings(); len(warnings) != 0 {
				t.Fatalf("warnings = %#v, want none", warnings)
			}
			reasoning, present := body["reasoning"].(map[string]any)
			if tt.wantEffort == "" && present {
				t.Fatalf("reasoning = %#v, want omitted", reasoning)
			}
			if tt.wantEffort != "" && (!present || reasoning["effort"] != tt.wantEffort) {
				t.Fatalf("reasoning = %#v, want effort %q", body["reasoning"], tt.wantEffort)
			}
		})
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	p := New(APIKey("secret"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	// R-CVJO-QHD1
	rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: "any-model", Gen: agentkit.GenSettings{Reasoning: agentkit.Budget(8000)}})
	if !errors.Is(rt.Err(), agentkit.ErrInvalidConfig) || requests != 0 {
		t.Fatalf("budget error = %v, requests = %d; want ErrInvalidConfig without request", rt.Err(), requests)
	}
}

func TestOpenAIRejectsEnableReasoningBeforeTransport(t *testing.T) {
	// R-DDWV-MNZJ
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP transport invoked for unsupported enabled reasoning")
		return nil, nil
	})}
	rt := New(APIKey("secret"), WithHTTPClient(client)).RoundTrip(context.Background(), &agentkit.Request{
		Model: "gpt-future",
		Gen:   agentkit.GenSettings{Reasoning: agentkit.EnableReasoning()},
	})
	if !errors.Is(rt.Err(), agentkit.ErrInvalidConfig) {
		t.Fatalf("RoundTrip() error = %v, want ErrInvalidConfig", rt.Err())
	}
}

func TestVendorRejectedReasoningValueReturnsTypedErrorUnchanged(t *testing.T) {
	const effort = "vendor-will-reject-this"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		reasoning, _ := body["reasoning"].(map[string]any)
		if reasoning["effort"] != effort {
			t.Errorf("reasoning = %#v, want unchanged effort", body["reasoning"])
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"unsupported effort","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	p := New(APIKey("secret"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	// R-CUBS-CPMC
	rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: "uncataloged-model", Gen: agentkit.GenSettings{Reasoning: agentkit.Level(effort)}})
	if len(rt.Warnings()) != 0 {
		t.Fatalf("warnings = %#v, want none", rt.Warnings())
	}
	var providerErr *agentkit.Error
	if !errors.As(rt.Err(), &providerErr) || providerErr.Provider != agentkit.ProviderOpenAI || providerErr.Auth != agentkit.AuthAPIKey {
		t.Fatalf("error = %#v, want typed OpenAI error", rt.Err())
	}
}

func TestProviderOptionsShallowMergeIntoResponsesBody(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, textOnlySSE("ok", 1, 0, 1, 0))
	}))
	defer server.Close()

	p := New(APIKey("secret"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	requests := []*agentkit.Request{
		{Model: "custom", ProviderOptions: json.RawMessage(`{"metadata":{"trace":"abc"},"service_tier":"priority"}`)},
		{Model: "plain"},
	}
	// R-CXZH-I0UF
	for _, req := range requests {
		rt := p.RoundTrip(context.Background(), req)
		if err := rt.Err(); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("bodies = %d, want 2", len(bodies))
	}
	if bodies[0]["service_tier"] != "priority" || !reflect.DeepEqual(bodies[0]["metadata"], map[string]any{"trace": "abc"}) {
		t.Fatalf("merged body = %#v", bodies[0])
	}
	if _, ok := bodies[1]["service_tier"]; ok {
		t.Fatalf("empty options leaked merged key: %#v", bodies[1])
	}
	if _, ok := bodies[1]["metadata"]; ok {
		t.Fatalf("empty options leaked metadata: %#v", bodies[1])
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
	pathTool := testRawTool("read_path", "read path", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		calls = append(calls, "read_path")
		if string(input) != `{"path":"PING"}` {
			t.Fatalf("path tool input = %s", input)
		}
		return "pong", nil
	})
	echoTool := testRawTool("echo_text", "echo text", json.RawMessage(`{"type":"object"}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		calls = append(calls, "echo_text")
		if string(input) != `{"text":"hello"}` {
			t.Fatalf("echo tool input = %s", input)
		}
		return "hello", nil
	})
	c := &agentkit.Conversation{
		Provider: New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    "gpt-5.5",
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

	p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model: "gpt-5.4-mini",
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

	p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	rt := p.RoundTrip(context.Background(), &agentkit.Request{
		Model:    "gpt-5.4",
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

			p := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: "gpt-5.4-nano"})
			err := rt.Err()
			// R-BUR1-XAK8, R-BX6U-OU1M, R-BYER-2LSB
			if !errors.Is(err, tt.category) {
				t.Fatalf("errors.Is(%v) = false for %v", tt.category, err)
			}
			var providerErr *agentkit.Error
			if !errors.As(err, &providerErr) {
				t.Fatalf("errors.As(*agentkit.Error) failed for %v", err)
			}
			if providerErr.Provider != agentkit.ProviderOpenAI || providerErr.Auth != agentkit.AuthAPIKey || providerErr.StatusCode != tt.status || providerErr.RequestID != "req_123" {
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

func TestOpenAIEmbedderBatchesUsageOrderAndNormalizes(t *testing.T) {
	var provider agentkit.EmbeddingProvider
	var mu sync.Mutex
	var requests []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %s, want /v1/embeddings", r.URL.Path)
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
		mu.Unlock()

		inputs, _ := body["input"].([]any)
		data := make([]map[string]any, len(inputs))
		for i, rawInput := range inputs {
			n := embeddingInputNumber(fmt.Sprint(rawInput))
			data[i] = map[string]any{
				"index":     i,
				"embedding": []float32{float32(n + 1), 1},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data,
			"usage": map[string]int64{
				"prompt_tokens": int64(len(inputs)),
			},
		})
	}))
	defer server.Close()

	provider = NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	inputs := make([]string, 2050)
	for i := range inputs {
		inputs[i] = fmt.Sprintf("input-%04d", i)
	}
	embedder := &agentkit.Embedder{Provider: provider, Model: EmbedModel3Small, Dimensions: 2}

	result, err := embedder.Embed(context.Background(), inputs, agentkit.InputQuery)
	// R-D7QO-K6RZ, R-YJ6S-3S9G, R-YPAA-0MYX, R-Y5RV-WB3T, R-YHYV-Q0IR
	if err != nil {
		t.Fatalf("Embed() error = %v, want nil", err)
	}
	if len(result.Vectors) != len(inputs) {
		t.Fatalf("vectors = %d, want %d", len(result.Vectors), len(inputs))
	}
	if got, want := result.Usage(), (agentkit.EmbeddingUsage{InputTokens: 2050, Total: 2050}); got != want {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}
	for _, index := range []int{0, 2048, 2049} {
		wantFirst := float64(index+1) / math.Sqrt(float64((index+1)*(index+1)+1))
		if got := float64(result.Vectors[index][0]); math.Abs(got-wantFirst) > 1e-6 {
			t.Fatalf("vector[%d][0] = %v, want %v", index, got, wantFirst)
		}
		if norm := l2(result.Vectors[index]); math.Abs(norm-1) > 1e-6 {
			t.Fatalf("vector[%d] norm = %v, want 1", index, norm)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	firstInputs, _ := requests[0]["input"].([]any)
	secondInputs, _ := requests[1]["input"].([]any)
	if len(firstInputs) != 2048 || len(secondInputs) != 2 {
		t.Fatalf("chunk sizes = %d/%d, want 2048/2", len(firstInputs), len(secondInputs))
	}
}

func TestOpenAIEmbedderClassifiesMissingAndUnusableCredentialsWithoutTransport(t *testing.T) {
	// R-ULAV-7QZR
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	tests := []struct {
		name            string
		credential      Credential
		wantSentinel    error
		wantMessage     string
		notWantSentinel error
	}{
		{
			name:         "missing API key",
			credential:   APIKey(""),
			wantSentinel: agentkit.ErrMissingCredential,
			wantMessage:  "openai: API key is absent",
		},
		{
			name:            "subscription cannot serve embeddings",
			credential:      Subscription(staticTokenSource{bearer: "token", account: "account"}),
			wantSentinel:    agentkit.ErrInvalidConfig,
			wantMessage:     "a ChatGPT subscription credential cannot serve embeddings; an API key is required",
			notWantSentinel: agentkit.ErrMissingCredential,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewEmbedder(tt.credential, WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			if provider == nil {
				t.Fatal("NewEmbedder() returned nil")
			}
			rt := provider.Embed(context.Background(), &agentkit.EmbedRequest{
				Model:  EmbedModel3Small,
				Inputs: []string{"hello"},
			})
			err := rt.Err()
			if !errors.Is(err, tt.wantSentinel) {
				t.Fatalf("Embed() error = %v, want %v", err, tt.wantSentinel)
			}
			if tt.notWantSentinel != nil && errors.Is(err, tt.notWantSentinel) {
				t.Fatalf("Embed() error = %v, do not want %v", err, tt.notWantSentinel)
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("Embed() error = %q, want message containing %q", err, tt.wantMessage)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want 0", requests)
	}
}

func TestOpenAIEmbeddingsIgnoreInputTypeOnWire(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()
		writeEmbeddingResponse(t, w, [][]float32{{3, 4}}, 1)
	}))
	defer server.Close()

	embedder := &agentkit.Embedder{
		Provider:   NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:      EmbedModel3Small,
		Dimensions: 2,
	}
	for _, role := range []agentkit.InputType{agentkit.InputUnspecified, agentkit.InputQuery, agentkit.InputDocument} {
		if _, err := embedder.Embed(context.Background(), []string{"hello"}, role); err != nil {
			t.Fatalf("Embed(%v) error = %v", role, err)
		}
	}

	// R-YLMK-VBQU, R-YANH-FE2L
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requests))
	}
	for _, body := range requests {
		for _, key := range []string{"role", "task", "input_type"} {
			if _, ok := body[key]; ok {
				t.Fatalf("request carried %q: %#v", key, body)
			}
		}
	}
}

func TestOpenAIEmbeddingContextLengthError(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"too many tokens","type":"invalid_request_error","code":"context_length_exceeded"}}`)
	}))
	defer server.Close()

	tooLong := &agentkit.Embedder{
		Provider: NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    EmbedModel3Small,
	}
	result, err := tooLong.Embed(context.Background(), []string{"hello"}, agentkit.InputQuery)
	// R-YKEO-HK05
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !errors.Is(err, agentkit.ErrContextLength) {
		t.Fatalf("context error = %v, want ErrContextLength", err)
	}
	var providerErr *agentkit.Error
	// R-LL8D-N2OD
	if !errors.As(err, &providerErr) ||
		providerErr.Category != agentkit.ErrContextLength ||
		providerErr.Provider != agentkit.ProviderOpenAI ||
		providerErr.Auth != agentkit.AuthAPIKey {
		t.Fatalf("provider error = %#v, want context-length category", providerErr)
	}
	if calls != 1 {
		t.Fatalf("calls after context error = %d, want 1", calls)
	}
}

func TestOpenAIEmbeddingDimensionsAndModelSwitching(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()

		dimensions := 1536
		if raw, ok := body["dimensions"].(float64); ok {
			dimensions = int(raw)
		}
		vector := make([]float32, dimensions)
		vector[0] = 3
		if dimensions > 1 {
			vector[1] = 4
		}
		writeEmbeddingResponse(t, w, [][]float32{vector}, 1)
	}))
	defer server.Close()

	embedder := &agentkit.Embedder{
		Provider: NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())),
		Model:    EmbedModel3Small,
	}
	native, err := embedder.Embed(context.Background(), []string{"native"}, agentkit.InputUnspecified)
	if err != nil {
		t.Fatalf("native Embed() error = %v", err)
	}
	if len(native.Vectors[0]) != 1536 {
		t.Fatalf("native vector dimension = %d, want 1536", len(native.Vectors[0]))
	}

	embedder.Model = EmbedModel3Large
	embedder.Dimensions = 3
	produced, err := embedder.Embed(context.Background(), []string{"three"}, agentkit.InputDocument)
	// R-YBVD-T5TA, R-Y6ZS-A2UI
	if err != nil {
		t.Fatalf("dimensioned Embed() error = %v", err)
	}
	if len(produced.Vectors[0]) != 3 {
		t.Fatalf("dimensioned vector dimension = %d, want 3", len(produced.Vectors[0]))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if _, ok := requests[0]["dimensions"]; ok {
		t.Fatalf("native request carried dimensions: %#v", requests[0])
	}
	if requests[1]["model"] != EmbedModel3Large || requests[1]["dimensions"] != float64(3) {
		t.Fatalf("second request = %#v, want large/dimensions=3", requests[1])
	}
}

func TestOpenAIEmbeddingRetryPolicy(t *testing.T) {
	t.Run("retryable chunk failure retries", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"error":{"message":"try again","type":"server_error"}}`)
				return
			}
			writeEmbeddingResponse(t, w, [][]float32{{1, 0}}, 1)
		}))
		defer server.Close()

		clock := &fakeEmbeddingClock{now: time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)}
		provider := NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).(*embeddingProvider)
		provider.cfg.Clock = clock
		embedder := &agentkit.Embedder{
			Provider: provider,
			Model:    EmbedModel3Small,
			Retry:    agentkit.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		}

		_, err := embedder.Embed(context.Background(), []string{"hello"}, agentkit.InputQuery)
		// R-YO2D-MV88
		if err != nil {
			t.Fatalf("Embed() error = %v, want nil", err)
		}
		if calls != 2 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Millisecond}) {
			t.Fatalf("calls/sleeps = %d/%v, want 2/[1ms]", calls, clock.sleeps)
		}
	})

	t.Run("non-retryable failure does not retry", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"bad input","type":"invalid_request_error"}}`)
		}))
		defer server.Close()

		clock := &fakeEmbeddingClock{now: time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)}
		provider := NewEmbedder(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).(*embeddingProvider)
		provider.cfg.Clock = clock
		embedder := &agentkit.Embedder{
			Provider: provider,
			Model:    EmbedModel3Small,
			Retry:    agentkit.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		}

		_, err := embedder.Embed(context.Background(), []string{"hello"}, agentkit.InputQuery)
		// R-YO2D-MV88
		if !errors.Is(err, agentkit.ErrInvalidRequest) {
			t.Fatalf("Embed() error = %v, want ErrInvalidRequest", err)
		}
		if calls != 1 || len(clock.sleeps) != 0 {
			t.Fatalf("calls/sleeps = %d/%v, want 1/[]", calls, clock.sleeps)
		}
	})
}

func embeddingInputNumber(input string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(input, "input-"))
	if err != nil {
		return 0
	}
	return n
}

func l2(vector []float32) float64 {
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	return math.Sqrt(sum)
}

func writeEmbeddingResponse(t *testing.T, w http.ResponseWriter, vectors [][]float32, promptTokens int64) {
	t.Helper()
	data := make([]map[string]any, len(vectors))
	for i, vector := range vectors {
		data[i] = map[string]any{"index": i, "embedding": vector}
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"data": data,
		"usage": map[string]int64{
			"prompt_tokens": promptTokens,
			"total_tokens":  promptTokens,
		},
	}); err != nil {
		t.Fatalf("write embedding response: %v", err)
	}
}

type fakeEmbeddingClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *fakeEmbeddingClock) Now() time.Time {
	return c.now
}

func (c *fakeEmbeddingClock) Sleep(ctx context.Context, delay time.Duration) error {
	c.sleeps = append(c.sleeps, delay)
	if err := ctx.Err(); err != nil {
		return err
	}
	c.now = c.now.Add(delay)
	return nil
}

func (c *fakeEmbeddingClock) Jitter(cap time.Duration) time.Duration {
	return cap
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
