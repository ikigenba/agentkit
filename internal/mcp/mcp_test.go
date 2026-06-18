package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCompletesHandshakeDiscoveryCallAndErrorPathsOffline(t *testing.T) {
	// R-711P-17EO
	// R-6MEW-FYIC
	ctx := context.Background()
	var sawInitialized bool
	var listCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeRequest(t, r)
		switch req.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "session-a")
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25","serverInfo":{"name":"fake","version":"1"}}}`)
		case "notifications/initialized":
			sawInitialized = true
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			listCalls++
			w.Header().Set("Content-Type", "text/event-stream")
			if listCalls == 1 {
				fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"echo\",\"description\":\"Echo\",\"inputSchema\":{\"type\":\"object\"}}],\"nextCursor\":\"page-2\"}}\n\n")
				return
			}
			fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":3,\"result\":{\"tools\":[{\"name\":\"fail\",\"description\":\"Fail\",\"inputSchema\":{\"type\":\"object\"}}]}}\n\n")
		case "tools/call":
			var params struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatalf("decode tools/call params: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			switch params.Name {
			case "fail":
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":4,"result":{"content":[{"type":"text","text":"business failure"}],"isError":true}}`)
			case "rpc-error":
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":5,"error":{"code":-32602,"message":"bad params","data":{"field":"x"}}}`)
			default:
				t.Fatalf("unexpected tool call %q", params.Name)
			}
		default:
			t.Fatalf("unexpected method %q", req.Method)
		}
	}))
	defer server.Close()

	client := New(Config{URL: server.URL})
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if !sawInitialized {
		t.Fatal("client did not send notifications/initialized")
	}
	if listCalls != 2 {
		t.Fatalf("ListTools calls = %d, want 2 pages", listCalls)
	}
	if got, want := len(tools), 2; got != want {
		t.Fatalf("len(tools) = %d, want %d", got, want)
	}
	if got := string(tools[0].InputSchema); got != `{"type":"object"}` {
		t.Fatalf("first tool schema = %s", got)
	}

	result, err := client.CallTool(ctx, "fail", json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatalf("CallTool returned error for isError result: %v", err)
	}
	if !result.IsError || len(result.Content) != 1 || result.Content[0].Text != "business failure" {
		t.Fatalf("CallTool result = %+v, want isError text result", result)
	}

	_, err = client.CallTool(ctx, "rpc-error", nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("CallTool rpc-error err = %T %[1]v, want *RPCError", err)
	}
	if rpcErr.Code != -32602 || rpcErr.Message != "bad params" || len(rpcErr.Raw) == 0 {
		t.Fatalf("rpc error = %+v, want code/message/raw", rpcErr)
	}

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
		http.Error(w, "token required", http.StatusUnauthorized)
	}))
	defer authServer.Close()
	_, err = New(Config{URL: authServer.URL}).Initialize(ctx)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("Initialize auth err = %T %[1]v, want *HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized || httpErr.WWWAuthenticate != `Bearer realm="mcp"` {
		t.Fatalf("auth error = %+v, want 401 with WWW-Authenticate", httpErr)
	}
}

func TestClientEchoesSessionAndProtocolHeadersAfterInitialize(t *testing.T) {
	// R-6OUP-7HZQ
	ctx := context.Background()
	var postInitRequests int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := decodeMethod(t, r)
		if method != "initialize" {
			postInitRequests++
			if got := r.Header.Get("Mcp-Session-Id"); got != "session-a" {
				t.Fatalf("%s Mcp-Session-Id = %q, want session-a", method, got)
			}
			if got := r.Header.Get("MCP-Protocol-Version"); got != ProtocolVersion {
				t.Fatalf("%s MCP-Protocol-Version = %q, want %s", method, got, ProtocolVersion)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-a")
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25"}}`)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`)
		case "tools/call":
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"ok"}]}}`)
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	client := New(Config{URL: server.URL})
	if _, err := client.ListTools(ctx); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if _, err := client.CallTool(ctx, "echo", nil); err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if postInitRequests != 3 {
		t.Fatalf("post-init requests = %d, want 3", postInitRequests)
	}
}

func TestExpiredSessionDiscoveryReinitializesButToolCallDoesNotReplay(t *testing.T) {
	// R-6RAH-Z1H4
	ctx := context.Background()
	var initCalls int
	var listCalls int
	var toolCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := decodeMethod(t, r)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "initialize":
			initCalls++
			w.Header().Set("Mcp-Session-Id", fmt.Sprintf("session-%d", initCalls))
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25","serverInfo":{"name":"fake-%d"}}}`, initCalls)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			listCalls++
			if listCalls == 1 {
				http.Error(w, "expired", http.StatusNotFound)
				return
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"echo","inputSchema":{"type":"object"}}]}}`)
		case "tools/call":
			toolCalls++
			http.Error(w, "expired", http.StatusNotFound)
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	client := New(Config{URL: server.URL})
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v, want echo", tools)
	}
	if initCalls != 2 || listCalls != 2 {
		t.Fatalf("after discovery: initCalls=%d listCalls=%d, want 2 and 2", initCalls, listCalls)
	}

	_, err = client.CallTool(ctx, "echo", nil)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("CallTool err = %T %[1]v, want 404 *HTTPError", err)
	}
	if toolCalls != 1 {
		t.Fatalf("toolCalls = %d, want no replay after first 404", toolCalls)
	}
	if initCalls != 3 {
		t.Fatalf("initCalls = %d, want tools/call 404 to re-establish once", initCalls)
	}
}

func decodeMethod(t *testing.T, r *http.Request) string {
	t.Helper()
	return decodeRequest(t, r).Method
}

type decodedRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func decodeRequest(t *testing.T, r *http.Request) decodedRequest {
	t.Helper()
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	decodeJSONRPC(t, r, &req)
	return decodedRequest(req)
}

func decodeJSONRPC(t *testing.T, r *http.Request, v any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}
