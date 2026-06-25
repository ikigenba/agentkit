package agentkit

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
	"testing"
	"time"
)

type mcpTestProvider struct {
	name       string
	rounds     []*RoundTrip
	calls      []Request
	onRequest  func(*Request)
	callNumber int
}

func (p *mcpTestProvider) RoundTrip(_ context.Context, req *Request) *RoundTrip {
	p.calls = append(p.calls, cloneMCPTestRequest(req))
	if p.onRequest != nil {
		p.onRequest(req)
	}
	p.callNumber++
	if len(p.rounds) == 0 {
		return mcpTextRoundTrip("ok")
	}
	rt := p.rounds[0]
	p.rounds = p.rounds[1:]
	return rt
}

func (p *mcpTestProvider) Name() string {
	if p.name == "" {
		return "fake"
	}
	return p.name
}

func (p *mcpTestProvider) Pricing(string) (Pricing, bool) {
	return Pricing{Tiers: []RateTier{{InputUncached: 1, Output: 1}}}, true
}

type mcpSchemaLimiterProvider struct {
	mcpTestProvider
	schemas []json.RawMessage
}

func (p *mcpSchemaLimiterProvider) UnsupportedSchemaKeywords(schema json.RawMessage) []string {
	p.schemas = append(p.schemas, append(json.RawMessage(nil), schema...))
	var keywords []string
	text := string(schema)
	if strings.Contains(text, "oneOf") {
		keywords = append(keywords, "oneOf")
	}
	if strings.Contains(text, "$ref") {
		keywords = append(keywords, "$ref")
	}
	if strings.Contains(text, "additionalProperties") {
		keywords = append(keywords, "additionalProperties")
	}
	return keywords
}

func TestMCPDiscoveryMergesToolsAndRoutesCalls(t *testing.T) {
	// R-6GBE-J3SV
	// R-6HJA-WVJK
	ctx := context.Background()
	var listCalls, callCalls int
	var calledName string
	server := newMCPTestServer(t, func(w http.ResponseWriter, r *http.Request, req mcpTestRequest) {
		switch req.Method {
		case "initialize":
			writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			listCalls++
			if listCalls == 1 {
				writeMCPResult(w, req.ID, `{"tools":[{"name":"weather.get/current","description":"Weather","inputSchema":{"type":"object","properties":{"city":{"type":"string"}}}}],"nextCursor":"next"}`)
				return
			}
			writeMCPResult(w, req.ID, `{"tools":[]}`)
		case "tools/call":
			callCalls++
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatalf("decode tools/call params: %v", err)
			}
			calledName = params.Name
			if string(params.Arguments) != `{"city":"Tokyo"}` {
				t.Fatalf("arguments = %s, want city payload", params.Arguments)
			}
			writeMCPResult(w, req.ID, `{"content":[{"type":"text","text":"sunny"}]}`)
		default:
			t.Fatalf("unexpected MCP method %q", req.Method)
		}
	})
	defer server.Close()

	exposed := "srv_1_weather_get_current"
	provider := &mcpTestProvider{rounds: []*RoundTrip{
		mcpRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{ToolUseBlock{ID: "toolu_mcp", Name: exposed, Input: json.RawMessage(`{"city":"Tokyo"}`)}}}, FinishToolUse, nil),
		mcpTextRoundTrip("done"),
	}}
	conv := &Conversation{
		Provider:   provider,
		Model:      "mcp-model",
		MCPServers: []MCPServer{{Name: "srv-1", URL: server.URL}},
	}

	stream := conv.Send(ctx, "weather")
	events := drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Send Err() = %v, want nil", err)
	}
	if listCalls != 2 {
		t.Fatalf("tools/list calls = %d, want paginated exhaustion", listCalls)
	}
	if callCalls != 1 || calledName != "weather.get/current" {
		t.Fatalf("tools/call count/name = %d/%q, want 1/original MCP name", callCalls, calledName)
	}
	if got := provider.calls[0].Tools[0].Name(); got != exposed {
		t.Fatalf("exposed tool name = %q, want sanitized %q", got, exposed)
	}
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`).MatchString(provider.calls[0].Tools[0].Name()) {
		t.Fatalf("tool name %q does not match provider-safe charset", provider.calls[0].Tools[0].Name())
	}
	result := firstMCPEvent[ToolResult](t, events)
	if result.Output != "sunny" || result.IsError {
		t.Fatalf("ToolResult = %#v, want successful MCP result", result)
	}
}

func TestMCPDiscoveryFailuresAndCollisionsStopBeforeProvider(t *testing.T) {
	t.Run("unreachable server fails before provider", func(t *testing.T) {
		// R-6L70-26RN
		server := httptest.NewServer(http.NotFoundHandler())
		server.Close()
		provider := &mcpTestProvider{}
		clock := &fakeMCPClock{}
		conv := &Conversation{
			Provider:   provider,
			Model:      "mcp-model",
			Retry:      RetryPolicy{MaxAttempts: 1},
			MCPServers: []MCPServer{{Name: "down", URL: server.URL}},
			retryClock: clock,
		}

		stream := conv.Send(context.Background(), "hello")
		drainMCP(stream)

		if !errors.Is(stream.Err(), ErrNetwork) {
			t.Fatalf("Err() = %v, want ErrNetwork", stream.Err())
		}
		var akErr *Error
		if !errors.As(stream.Err(), &akErr) {
			t.Fatalf("Err() = %T %[1]v, want *Error", stream.Err())
		}
		if akErr.MCPServer != "down" || akErr.Provider != "" || akErr.Message == "" {
			t.Fatalf("MCP error = %+v, want attributed network error without provider", akErr)
		}
		if len(conv.History) != 0 || len(provider.calls) != 0 {
			t.Fatalf("history/provider calls = %d/%d, want unchanged/no provider call", len(conv.History), len(provider.calls))
		}
		if len(clock.sleeps) != 0 {
			t.Fatalf("sleeps = %v, want no backoff for single-attempt discovery", clock.sleeps)
		}
	})

	t.Run("collision", func(t *testing.T) {
		// R-6IR7-ANA9
		server := newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
			switch req.Method {
			case "initialize":
				writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/list":
				writeMCPResult(w, req.ID, `{"tools":[{"name":"tool","inputSchema":{"type":"object"}}]}`)
			}
		})
		defer server.Close()
		provider := &mcpTestProvider{}
		conv := &Conversation{
			Provider: provider,
			Model:    "mcp-model",
			Tools: []Tool{RawTool("srv_tool", "custom", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
				return "custom", nil
			})},
			MCPServers: []MCPServer{{Name: "srv", URL: server.URL}},
		}
		stream := conv.Send(context.Background(), "hello")
		drainMCP(stream)
		if !errors.Is(stream.Err(), ErrInvalidConfig) {
			t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
		}
		if len(conv.History) != 0 || len(provider.calls) != 0 {
			t.Fatalf("history/provider calls = %d/%d, want unchanged/no provider call", len(conv.History), len(provider.calls))
		}
	})

	t.Run("discovery rpc error attribution", func(t *testing.T) {
		// R-6L70-26RN
		// R-6TQA-QKYI
		server := newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
			switch req.Method {
			case "initialize":
				writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/list":
				writeMCPError(w, req.ID, -32601, "missing method")
			}
		})
		defer server.Close()
		provider := &mcpTestProvider{}
		conv := &Conversation{Provider: provider, Model: "mcp-model", MCPServers: []MCPServer{{Name: "bad", URL: server.URL}}}
		stream := conv.Send(context.Background(), "hello")
		drainMCP(stream)
		if !errors.Is(stream.Err(), ErrInvalidRequest) {
			t.Fatalf("Err() = %v, want ErrInvalidRequest", stream.Err())
		}
		var akErr *Error
		if !errors.As(stream.Err(), &akErr) {
			t.Fatalf("Err() = %T %[1]v, want *Error", stream.Err())
		}
		if akErr.MCPServer != "bad" || akErr.Provider != "" || akErr.Type != "-32601" || !strings.Contains(string(akErr.Raw), "missing method") {
			t.Fatalf("MCP error = %+v raw=%s, want attributed raw JSON-RPC error", akErr, akErr.Raw)
		}
		if len(conv.History) != 0 || len(provider.calls) != 0 {
			t.Fatalf("history/provider calls = %d/%d, want unchanged/no provider call", len(conv.History), len(provider.calls))
		}
	})

	t.Run("bad server does not corrupt detached healthy server", func(t *testing.T) {
		// R-6L70-26RN
		var healthyCalls int
		healthy := newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
			switch req.Method {
			case "initialize":
				writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/list":
				writeMCPResult(w, req.ID, `{"tools":[{"name":"echo","inputSchema":{"type":"object"}}]}`)
			case "tools/call":
				healthyCalls++
				writeMCPResult(w, req.ID, `{"content":[{"type":"text","text":"healthy ok"}]}`)
			default:
				t.Fatalf("unexpected healthy MCP method %q", req.Method)
			}
		})
		defer healthy.Close()
		bad := newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
			switch req.Method {
			case "initialize":
				writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/list":
				writeMCPError(w, req.ID, -32601, "bad tools")
			default:
				t.Fatalf("unexpected bad MCP method %q", req.Method)
			}
		})
		defer bad.Close()

		provider := &mcpTestProvider{}
		clock := &fakeMCPClock{}
		conv := &Conversation{
			Provider: provider,
			Model:    "mcp-model",
			Retry:    RetryPolicy{MaxAttempts: 1},
			MCPServers: []MCPServer{
				{Name: "good", URL: healthy.URL},
				{Name: "bad", URL: bad.URL},
			},
			retryClock: clock,
		}

		stream := conv.Send(context.Background(), "hello")
		drainMCP(stream)

		if !errors.Is(stream.Err(), ErrInvalidRequest) {
			t.Fatalf("Err() = %v, want ErrInvalidRequest", stream.Err())
		}
		var akErr *Error
		if !errors.As(stream.Err(), &akErr) {
			t.Fatalf("Err() = %T %[1]v, want *Error", stream.Err())
		}
		if akErr.MCPServer != "bad" || akErr.Provider != "" || akErr.Type != "-32601" {
			t.Fatalf("MCP error = %+v, want attribution only to bad server", akErr)
		}
		if len(conv.History) != 0 || len(provider.calls) != 0 {
			t.Fatalf("history/provider calls = %d/%d, want unchanged/no provider call", len(conv.History), len(provider.calls))
		}

		provider.rounds = []*RoundTrip{
			mcpRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{ToolUseBlock{ID: "toolu_good", Name: "good_echo", Input: json.RawMessage(`{}`)}}}, FinishToolUse, nil),
			mcpTextRoundTrip("done"),
		}
		conv.MCPServers = []MCPServer{{Name: "good", URL: healthy.URL}}
		stream = conv.Send(context.Background(), "hello again")
		events := drainMCP(stream)

		if err := stream.Err(); err != nil {
			t.Fatalf("detached healthy Err() = %v, want nil", err)
		}
		if healthyCalls != 1 {
			t.Fatalf("healthy tool calls = %d, want one successful call", healthyCalls)
		}
		result := firstMCPEvent[ToolResult](t, events)
		if result.Name != "good_echo" || result.Output != "healthy ok" || result.IsError {
			t.Fatalf("ToolResult = %#v, want successful healthy server result", result)
		}
		if got := toolNames(provider.calls[0].Tools); !reflect.DeepEqual(got, []string{"good_echo"}) {
			t.Fatalf("healthy tools = %v, want only good server tool", got)
		}
	})
}

func TestMCPToolResultErrorVsTerminalFailure(t *testing.T) {
	t.Run("isError is in-band", func(t *testing.T) {
		// R-6NMS-TQ91
		server := newMCPToolCallServer(t, func(w http.ResponseWriter, req mcpTestRequest) {
			if req.Method == "tools/call" {
				writeMCPResult(w, req.ID, `{"content":[{"type":"text","text":"business failure"}],"isError":true}`)
			}
		})
		defer server.Close()
		provider := &mcpTestProvider{rounds: []*RoundTrip{
			mcpRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{ToolUseBlock{ID: "toolu_mcp", Name: "srv_fail", Input: json.RawMessage(`{}`)}}}, FinishToolUse, nil),
			mcpTextRoundTrip("recovered"),
		}}
		conv := &Conversation{Provider: provider, Model: "mcp-model", MCPServers: []MCPServer{{Name: "srv", URL: server.URL}}}
		events := drainMCP(conv.Send(context.Background(), "call"))
		result := firstMCPEvent[ToolResult](t, events)
		if !result.IsError || result.Output != "business failure" {
			t.Fatalf("ToolResult = %#v, want in-band isError result", result)
		}
		if len(provider.calls) != 2 {
			t.Fatalf("provider calls = %d, want loop continuation", len(provider.calls))
		}
	})

	t.Run("json-rpc error is terminal", func(t *testing.T) {
		// R-6NMS-TQ91
		server := newMCPToolCallServer(t, func(w http.ResponseWriter, req mcpTestRequest) {
			if req.Method == "tools/call" {
				writeMCPError(w, req.ID, -32603, "boom")
			}
		})
		defer server.Close()
		provider := &mcpTestProvider{rounds: []*RoundTrip{
			mcpRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{ToolUseBlock{ID: "toolu_mcp", Name: "srv_fail", Input: json.RawMessage(`{}`)}}}, FinishToolUse, nil),
			mcpTextRoundTrip("unreached"),
		}}
		conv := &Conversation{Provider: provider, Model: "mcp-model", MCPServers: []MCPServer{{Name: "srv", URL: server.URL}}}
		stream := conv.Send(context.Background(), "call")
		events := drainMCP(stream)
		if eventIndexMCP[ToolResult](events) >= 0 {
			t.Fatalf("events included ToolResult for terminal MCP error: %#v", events)
		}
		if !errors.Is(stream.Err(), ErrServerError) {
			t.Fatalf("Err() = %v, want ErrServerError", stream.Err())
		}
		if len(provider.calls) != 1 {
			t.Fatalf("provider calls = %d, want no continuation after terminal tool error", len(provider.calls))
		}
	})
}

func TestMCPAuthHeadersAndPermissionMapping(t *testing.T) {
	// R-6Q2L-L9QF
	// R-6UY7-4CP7
	var sawAuth bool
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer token" {
			sawAuth = true
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
		http.Error(w, "token required", http.StatusUnauthorized)
	}))
	defer authServer.Close()

	conv := &Conversation{Provider: &mcpTestProvider{}, Model: "mcp-model", MCPServers: []MCPServer{{Name: "auth", URL: authServer.URL, Headers: map[string]string{"Authorization": "Bearer token"}}}}
	stream := conv.Send(context.Background(), "hello")
	drainMCP(stream)
	if !sawAuth {
		t.Fatal("consumer header was not sent on MCP request")
	}
	if !errors.Is(stream.Err(), ErrAuthentication) {
		t.Fatalf("Err() = %v, want ErrAuthentication", stream.Err())
	}
	var akErr *Error
	if !errors.As(stream.Err(), &akErr) || akErr.Provider != "" || akErr.MCPServer != "auth" || !strings.Contains(akErr.Message, `Bearer realm="mcp"`) || !strings.Contains(string(akErr.Raw), `Bearer realm=\"mcp\"`) {
		t.Fatalf("auth Error = %+v raw=%s, want MCP attribution and WWW-Authenticate", akErr, akErr.Raw)
	}

	permissionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer permissionServer.Close()
	stream = (&Conversation{Provider: &mcpTestProvider{}, Model: "mcp-model", MCPServers: []MCPServer{{Name: "deny", URL: permissionServer.URL}}}).Send(context.Background(), "hello")
	drainMCP(stream)
	if !errors.Is(stream.Err(), ErrPermission) {
		t.Fatalf("403 Err() = %v, want ErrPermission", stream.Err())
	}
}

func TestMCPDiscoveryRetriesButToolCallDoesNot(t *testing.T) {
	// R-6XDZ-VW6L
	t.Run("transient discovery retries", func(t *testing.T) {
		var listCalls int
		server := newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
			switch req.Method {
			case "initialize":
				writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/list":
				listCalls++
				if listCalls == 1 {
					http.Error(w, "temporary", http.StatusInternalServerError)
					return
				}
				writeMCPResult(w, req.ID, `{"tools":[{"name":"ok","inputSchema":{"type":"object"}}]}`)
			}
		})
		defer server.Close()
		clock := &fakeMCPClock{}
		provider := &mcpTestProvider{}
		conv := &Conversation{
			Provider:   provider,
			Model:      "mcp-model",
			Retry:      RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
			MCPServers: []MCPServer{{Name: "srv", URL: server.URL}},
			retryClock: clock,
		}
		stream := conv.Send(context.Background(), "hello")
		drainMCP(stream)
		if err := stream.Err(); err != nil {
			t.Fatalf("Err() = %v, want nil after discovery retry", err)
		}
		if listCalls != 2 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Millisecond}) {
			t.Fatalf("listCalls/sleeps = %d/%v, want one retry", listCalls, clock.sleeps)
		}
	})

	t.Run("non retryable discovery HTTP status fails fast", func(t *testing.T) {
		tests := []struct {
			name     string
			status   int
			category error
		}{
			{name: "400", status: http.StatusBadRequest, category: ErrInvalidRequest},
			{name: "401", status: http.StatusUnauthorized, category: ErrAuthentication},
			{name: "403", status: http.StatusForbidden, category: ErrPermission},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var listCalls int
				server := newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
					switch req.Method {
					case "initialize":
						writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
					case "notifications/initialized":
						w.WriteHeader(http.StatusAccepted)
					case "tools/list":
						listCalls++
						http.Error(w, "not mcp", tt.status)
					}
				})
				defer server.Close()
				clock := &fakeMCPClock{}
				provider := &mcpTestProvider{}
				conv := &Conversation{
					Provider:   provider,
					Model:      "mcp-model",
					Retry:      RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
					MCPServers: []MCPServer{{Name: "srv", URL: server.URL}},
					retryClock: clock,
				}

				stream := conv.Send(context.Background(), "hello")
				drainMCP(stream)

				if !errors.Is(stream.Err(), tt.category) {
					t.Fatalf("Err() = %v, want %v", stream.Err(), tt.category)
				}
				if listCalls != 1 {
					t.Fatalf("tools/list calls = %d, want exactly one fail-fast attempt", listCalls)
				}
				if len(clock.sleeps) != 0 {
					t.Fatalf("sleeps = %v, want none for non-retryable discovery failure", clock.sleeps)
				}
				if len(conv.History) != 0 || len(provider.calls) != 0 {
					t.Fatalf("history/provider calls = %d/%d, want unchanged/no provider call", len(conv.History), len(provider.calls))
				}
			})
		}
	})

	// R-6YLW-9NXA
	var toolCalls int
	callServer := newMCPToolCallServer(t, func(w http.ResponseWriter, req mcpTestRequest) {
		if req.Method == "tools/call" {
			toolCalls++
			http.Error(w, "temporary", http.StatusInternalServerError)
		}
	})
	defer callServer.Close()
	provider := &mcpTestProvider{rounds: []*RoundTrip{
		mcpRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{ToolUseBlock{ID: "toolu_mcp", Name: "srv_fail", Input: json.RawMessage(`{}`)}}}, FinishToolUse, nil),
		mcpTextRoundTrip("unreached"),
	}}
	clock := &fakeMCPClock{}
	conv := &Conversation{
		Provider:   provider,
		Model:      "mcp-model",
		Retry:      RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		MCPServers: []MCPServer{{Name: "srv", URL: callServer.URL}},
		retryClock: clock,
	}
	stream := conv.Send(context.Background(), "call")
	events := drainMCP(stream)
	if !errors.Is(stream.Err(), ErrServerError) {
		t.Fatalf("tool call Err() = %v, want ErrServerError", stream.Err())
	}
	if toolCalls != 1 || len(clock.sleeps) != 0 {
		t.Fatalf("toolCalls/sleeps = %d/%v, want no automatic retry", toolCalls, clock.sleeps)
	}
	if eventIndexMCP[ToolResult](events) >= 0 {
		t.Fatalf("events included partial ToolResult for terminal MCP transport failure: %#v", events)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider calls = %d, want no continuation after terminal MCP transport failure", len(provider.calls))
	}
}

func TestMCPToolsJoinDeterministicOrderAndSchemaWarnings(t *testing.T) {
	// R-6W63-I4FW
	// R-6ZTS-NFNZ
	serverA := newMCPListOnlyServer(t, "zeta", `{"type":"object","oneOf":[{"type":"object"}]}`)
	defer serverA.Close()
	serverB := newMCPListOnlyServer(t, "alpha", `{"type":"object","properties":{"x":{"$ref":"#/$defs/X"}},"additionalProperties":false,"$defs":{"X":{"type":"string"}}}`)
	defer serverB.Close()
	serverM := newMCPListOnlyServer(t, "middle", `{"type":"object"}`)
	defer serverM.Close()

	custom := RawTool("custom_mid", "custom", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) { return "ok", nil })
	provider := &mcpSchemaLimiterProvider{mcpTestProvider: mcpTestProvider{name: "schema-limited"}}
	conv := &Conversation{
		Provider: provider,
		Model:    "mcp-model",
		Tools:    []Tool{custom},
		MCPServers: []MCPServer{
			{Name: "srvZ", URL: serverA.URL},
			{Name: "srvA", URL: serverB.URL},
		},
	}
	stream := conv.Send(context.Background(), "one")
	drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("first Err() = %v, want nil", err)
	}
	stream = conv.Send(context.Background(), "two")
	drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("second Err() = %v, want nil", err)
	}
	want := []string{"custom_mid", "srvA_alpha", "srvZ_zeta"}
	for i, call := range provider.calls {
		if got := toolNames(call.Tools); !reflect.DeepEqual(got, want) {
			t.Fatalf("call %d tools = %v, want %v", i, got, want)
		}
	}
	schemaWarnings := stream.Warnings()
	conv.MCPServers = append(conv.MCPServers, MCPServer{Name: "srvM", URL: serverM.URL})
	stream = conv.Send(context.Background(), "three")
	drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("attach Err() = %v, want nil", err)
	}
	want = []string{"custom_mid", "srvA_alpha", "srvM_middle", "srvZ_zeta"}
	if got := toolNames(provider.calls[2].Tools); !reflect.DeepEqual(got, want) {
		t.Fatalf("attached tools = %v, want newly sorted merged order %v", got, want)
	}
	conv.MCPServers = []MCPServer{
		{Name: "srvZ", URL: serverA.URL},
		{Name: "srvM", URL: serverM.URL},
	}
	stream = conv.Send(context.Background(), "four")
	drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("detach one Err() = %v, want nil", err)
	}
	want = []string{"custom_mid", "srvM_middle", "srvZ_zeta"}
	if got := toolNames(provider.calls[3].Tools); !reflect.DeepEqual(got, want) {
		t.Fatalf("detached-one tools = %v, want re-sorted remaining merged order %v", got, want)
	}
	if len(schemaWarnings) != 2 {
		t.Fatalf("MCP schema warnings = %#v, want two warnings", schemaWarnings)
	}
	// R-SKVI-TSZQ
	if provider.Name() == "google" {
		t.Fatal("test provider name unexpectedly matched google")
	}
	detail := schemaWarnings[0].Detail + " " + schemaWarnings[1].Detail
	for _, attributed := range []string{"srvA.alpha", "srvZ.zeta"} {
		if !strings.Contains(detail, attributed) {
			t.Fatalf("warnings %q do not attribute MCP tool %s", detail, attributed)
		}
	}
	for _, keyword := range []string{"$ref", "additionalProperties", "oneOf"} {
		if !strings.Contains(detail, keyword) {
			t.Fatalf("warnings %q do not name dropped keyword %s", detail, keyword)
		}
	}
	if len(provider.schemas) == 0 {
		t.Fatalf("schema limiter was not consulted")
	}

	nonLimiter := &mcpTestProvider{name: "google"}
	conv = &Conversation{Provider: nonLimiter, Model: "mcp-model", MCPServers: []MCPServer{{Name: "srvA", URL: serverB.URL}}}
	stream = conv.Send(context.Background(), "openai")
	drainMCP(stream)
	// R-SNBB-LCH4
	if len(stream.Warnings()) != 0 {
		t.Fatalf("non-limiter warnings = %#v, want none even when Provider.Name is google", stream.Warnings())
	}
}

func TestMCPDetachRemovesToolsAndCloseDeletesSessions(t *testing.T) {
	// R-6SIE-CT7T
	var deletes int
	server := newMCPListOnlyServerWithDelete(t, "echo", `{"type":"object"}`, &deletes)
	defer server.Close()
	provider := &mcpTestProvider{}
	conv := &Conversation{Provider: provider, Model: "mcp-model", MCPServers: []MCPServer{{Name: "srv", URL: server.URL}}}
	stream := conv.Send(context.Background(), "with mcp")
	drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("first Err() = %v, want nil", err)
	}
	if got := toolNames(provider.calls[0].Tools); !reflect.DeepEqual(got, []string{"srv_echo"}) {
		t.Fatalf("first tools = %v, want MCP tool", got)
	}

	conv.MCPServers = nil
	stream = conv.Send(context.Background(), "detached")
	drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("detached Err() = %v, want nil", err)
	}
	if got := toolNames(provider.calls[1].Tools); len(got) != 0 {
		t.Fatalf("detached tools = %v, want none", got)
	}
	if deletes != 1 {
		t.Fatalf("deletes after detach = %d, want one best-effort DELETE", deletes)
	}

	conv.MCPServers = []MCPServer{{Name: "srv", URL: server.URL}}
	stream = conv.Send(context.Background(), "reattach")
	drainMCP(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("reattach Err() = %v, want nil", err)
	}
	if err := conv.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if deletes != 2 {
		t.Fatalf("deletes after close = %d, want second best-effort DELETE", deletes)
	}
}

type mcpTestRequest struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func newMCPToolCallServer(t *testing.T, onCall func(http.ResponseWriter, mcpTestRequest)) *httptest.Server {
	t.Helper()
	return newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
		switch req.Method {
		case "initialize":
			writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeMCPResult(w, req.ID, `{"tools":[{"name":"fail","inputSchema":{"type":"object"}}]}`)
		default:
			onCall(w, req)
		}
	})
}

func newMCPListOnlyServer(t *testing.T, name, schema string) *httptest.Server {
	t.Helper()
	return newMCPListOnlyServerWithDelete(t, name, schema, nil)
}

func newMCPListOnlyServerWithDelete(t *testing.T, name, schema string, deletes *int) *httptest.Server {
	t.Helper()
	return newMCPTestServer(t, func(w http.ResponseWriter, r *http.Request, req mcpTestRequest) {
		if r.Method == http.MethodDelete {
			if deletes != nil {
				(*deletes)++
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-"+name)
			writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeMCPResult(w, req.ID, fmt.Sprintf(`{"tools":[{"name":%q,"inputSchema":%s}]}`, name, schema))
		default:
			t.Fatalf("unexpected method %q", req.Method)
		}
	})
}

func newMCPTestServer(t *testing.T, handler func(http.ResponseWriter, *http.Request, mcpTestRequest)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handler(w, r, mcpTestRequest{})
			return
		}
		var req mcpTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		handler(w, r, req)
	}))
}

func writeMCPResult(w http.ResponseWriter, id int64, result string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, id, result)
}

func writeMCPError(w http.ResponseWriter, id int64, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"error":{"code":%d,"message":%q}}`, id, code, message)
}

type fakeMCPClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *fakeMCPClock) Now() time.Time {
	return c.now
}

func (c *fakeMCPClock) Sleep(ctx context.Context, delay time.Duration) error {
	c.sleeps = append(c.sleeps, delay)
	c.now = c.now.Add(delay)
	return ctx.Err()
}

func (c *fakeMCPClock) Jitter(cap time.Duration) time.Duration {
	return cap
}

func mcpRoundTrip(message Message, finish FinishReason, err error) *RoundTrip {
	return NewRoundTrip(message, finish, Usage{InputUncached: 1, Output: 1, Total: 2}, nil, err)
}

func mcpTextRoundTrip(text string) *RoundTrip {
	return mcpRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: text}}}, FinishStop, nil)
}

func drainMCP(stream *Stream) []Event {
	var events []Event
	for ev := range stream.Events() {
		events = append(events, ev)
	}
	return events
}

func firstMCPEvent[T Event](t *testing.T, events []Event) T {
	t.Helper()
	for _, ev := range events {
		if typed, ok := ev.(T); ok {
			return typed
		}
	}
	var zero T
	t.Fatalf("events %#v did not contain %T", events, zero)
	return zero
}

func eventIndexMCP[T Event](events []Event) int {
	for i, ev := range events {
		if _, ok := ev.(T); ok {
			return i
		}
	}
	return -1
}

func toolNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	return names
}

func cloneMCPTestRequest(req *Request) Request {
	if req == nil {
		return Request{}
	}
	return Request{
		Model:    req.Model,
		System:   req.System,
		Messages: cloneMessages(req.Messages),
		Tools:    append([]Tool(nil), req.Tools...),
		Gen:      req.Gen,
	}
}
