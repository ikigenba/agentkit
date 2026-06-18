package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/ikigenba/agentkit/internal/httpx"
	"github.com/ikigenba/agentkit/internal/sse"
)

const ProtocolVersion = "2025-11-25"

// Config contains the transport settings for one Streamable-HTTP MCP server.
type Config struct {
	URL           string
	Headers       map[string]string
	HTTPClient    *http.Client
	ClientName    string
	ClientVersion string
}

// Client is a raw Streamable-HTTP JSON-RPC MCP client.
type Client struct {
	url           string
	headers       map[string]string
	httpClient    *http.Client
	clientName    string
	clientVersion string

	nextID          int64
	sessionID       string
	protocolVersion string
	ready           bool
}

// New constructs a Client. The server is contacted lazily by Initialize,
// NotifyInitialized, ListTools, or CallTool.
func New(cfg Config) *Client {
	headers := make(map[string]string, len(cfg.Headers))
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	name := cfg.ClientName
	if name == "" {
		name = "agentkit"
	}
	version := cfg.ClientVersion
	if version == "" {
		version = "0"
	}
	return &Client{
		url:             cfg.URL,
		headers:         headers,
		httpClient:      httpx.Client(cfg.HTTPClient),
		clientName:      name,
		clientVersion:   version,
		protocolVersion: ProtocolVersion,
	}
}

type InitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities,omitempty"`
	ServerInfo      ServerInfo      `json:"serverInfo,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

func (t *Tool) UnmarshalJSON(data []byte) error {
	var wire struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	t.Name = wire.Name
	t.Description = wire.Description
	t.InputSchema = append(t.InputSchema[:0], wire.InputSchema...)
	return nil
}

type CallResult struct {
	Content []Content `json:"content,omitempty"`
	IsError bool      `json:"isError,omitempty"`
	Raw     json.RawMessage
}

func (r *CallResult) UnmarshalJSON(data []byte) error {
	type callResult CallResult
	var wire callResult
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*r = CallResult(wire)
	r.Raw = append(r.Raw[:0], data...)
	return nil
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Raw  json.RawMessage
}

func (c *Content) UnmarshalJSON(data []byte) error {
	var wire struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	c.Type = wire.Type
	c.Text = wire.Text
	c.Raw = append(c.Raw[:0], data...)
	return nil
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
	Raw     json.RawMessage `json:"-"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" {
		return fmt.Sprintf("mcp json-rpc error: %d", e.Code)
	}
	return fmt.Sprintf("mcp json-rpc error %d: %s", e.Code, e.Message)
}

type HTTPError struct {
	StatusCode      int
	Message         string
	Raw             json.RawMessage
	WWWAuthenticate string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return fmt.Sprintf("mcp http error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("mcp http error %d", e.StatusCode)
}

// Initialize sends the MCP initialize request and records the negotiated
// protocol version and session id, if the server provides them.
func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	params := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    c.clientName,
			"version": c.clientVersion,
		},
	}

	raw, err := c.post(ctx, "initialize", params, true)
	if err != nil {
		return InitializeResult{}, err
	}
	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return InitializeResult{}, err
	}
	if result.ProtocolVersion == "" {
		result.ProtocolVersion = ProtocolVersion
	}
	c.protocolVersion = result.ProtocolVersion
	c.ready = false
	return result, nil
}

// NotifyInitialized sends notifications/initialized for the current session.
func (c *Client) NotifyInitialized(ctx context.Context) error {
	if _, err := c.post(ctx, "notifications/initialized", nil, false); err != nil {
		return err
	}
	c.ready = true
	return nil
}

// ListTools returns every tool from tools/list, following nextCursor until the
// server has no more pages.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	return c.listTools(ctx, true)
}

// CallTool invokes tools/call. A result with isError:true is returned as a
// successful CallResult; only JSON-RPC or transport errors are Go errors.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	if err := c.ensureReady(ctx); err != nil {
		return CallResult{}, err
	}
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	raw, err := c.post(ctx, "tools/call", params, true)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			_ = c.reinitialize(ctx)
		}
		return CallResult{}, err
	}
	var result CallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CallResult{}, err
	}
	return result, nil
}

// DeleteSession best-effort terminates the current Streamable-HTTP session.
// A 405 response is benign because some MCP servers do not implement DELETE.
func (c *Client) DeleteSession(ctx context.Context) error {
	if c.sessionID == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("MCP-Protocol-Version", c.protocolVersion)
	req.Header.Set("Mcp-Session-Id", c.sessionID)
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		c.sessionID = ""
		c.ready = false
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return &HTTPError{
			StatusCode:      resp.StatusCode,
			Message:         strings.TrimSpace(string(raw)),
			Raw:             append(json.RawMessage(nil), raw...),
			WWWAuthenticate: resp.Header.Get("WWW-Authenticate"),
		}
	}
	c.sessionID = ""
	c.ready = false
	return nil
}

func (c *Client) listTools(ctx context.Context, retryExpiredSession bool) ([]Tool, error) {
	if err := c.ensureReady(ctx); err != nil {
		return nil, err
	}

	var tools []Tool
	var cursor string
	for {
		page, err := c.listToolsPage(ctx, cursor)
		if err != nil {
			var httpErr *HTTPError
			if retryExpiredSession && errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
				if initErr := c.reinitialize(ctx); initErr != nil {
					return nil, initErr
				}
				return c.listTools(ctx, false)
			}
			return nil, err
		}
		tools = append(tools, page.Tools...)
		if page.NextCursor == "" {
			return tools, nil
		}
		cursor = page.NextCursor
	}
}

type listToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

func (c *Client) listToolsPage(ctx context.Context, cursor string) (listToolsResult, error) {
	params := map[string]any{}
	if cursor != "" {
		params["cursor"] = cursor
	}
	raw, err := c.post(ctx, "tools/list", params, true)
	if err != nil {
		return listToolsResult{}, err
	}
	var result listToolsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return listToolsResult{}, err
	}
	return result, nil
}

func (c *Client) ensureReady(ctx context.Context) error {
	if c.ready {
		return nil
	}
	return c.reinitialize(ctx)
}

func (c *Client) reinitialize(ctx context.Context) error {
	if _, err := c.Initialize(ctx); err != nil {
		return err
	}
	return c.NotifyInitialized(ctx)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func (c *Client) post(ctx context.Context, method string, params any, needsResponse bool) (json.RawMessage, error) {
	body := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	if needsResponse {
		body.ID = atomic.AddInt64(&c.nextID, 1)
	}

	req, err := httpx.JSONRequest(ctx, http.MethodPost, c.url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.protocolVersion)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if sessionID := resp.Header.Get("Mcp-Session-Id"); sessionID != "" {
		c.sessionID = sessionID
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, &HTTPError{
			StatusCode:      resp.StatusCode,
			Message:         strings.TrimSpace(string(raw)),
			Raw:             append(json.RawMessage(nil), raw...),
			WWWAuthenticate: resp.Header.Get("WWW-Authenticate"),
		}
	}

	if !needsResponse && (resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent) {
		return nil, nil
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		return readSSEResponse(resp.Body, needsResponse)
	}
	return readJSONResponse(resp.Body, needsResponse)
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

func readJSONResponse(r io.Reader, needsResponse bool) (json.RawMessage, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		if needsResponse {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, nil
	}
	return parseEnvelope(raw)
}

func readSSEResponse(r io.Reader, needsResponse bool) (json.RawMessage, error) {
	events, err := sse.ReadAll(r)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		data := bytes.TrimSpace(event.Data)
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		result, err := parseEnvelope(data)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	if needsResponse {
		return nil, io.ErrUnexpectedEOF
	}
	return nil, nil
}

func parseEnvelope(raw json.RawMessage) (json.RawMessage, error) {
	var env rpcEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if len(env.Error) > 0 && !bytes.Equal(bytes.TrimSpace(env.Error), []byte("null")) {
		var rpcErr RPCError
		if err := json.Unmarshal(env.Error, &rpcErr); err != nil {
			return nil, err
		}
		rpcErr.Raw = append(json.RawMessage(nil), env.Error...)
		return nil, &rpcErr
	}
	if env.Result == nil {
		return json.RawMessage(`null`), nil
	}
	return append(json.RawMessage(nil), env.Result...), nil
}
