package agentkit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ikigenba/agentkit/internal/mcp"
)

type mcpTool struct {
	name         string
	description  string
	schema       json.RawMessage
	server       string
	originalName string
	client       *mcp.Client
}

func (t *mcpTool) Name() string {
	return t.name
}

func (t *mcpTool) Description() string {
	return t.description
}

func (t *mcpTool) JSONSchema() json.RawMessage {
	return append(json.RawMessage(nil), t.schema...)
}

func (t *mcpTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	result, err := t.client.CallTool(ctx, t.originalName, input)
	if err != nil {
		return "", terminalToolError{err: mcpError(t.server, err)}
	}
	text := mcpResultText(result)
	if result.IsError {
		return "", errors.New(text)
	}
	return text, nil
}

func (t *mcpTool) isTool() {}

type terminalToolError struct {
	err error
}

func (e terminalToolError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e terminalToolError) Unwrap() error {
	return e.err
}

func (c *Conversation) resolveTools(ctx context.Context) ([]Tool, []Warning, error) {
	mcpTools, warnings, err := c.resolveMCPTools(ctx)
	if err != nil {
		return nil, nil, err
	}
	all := make([]Tool, 0, len(c.Tools)+len(mcpTools))
	all = append(all, c.Tools...)
	all = append(all, mcpTools...)
	tools, err := validateAndSortTools(all)
	if err != nil {
		return nil, nil, err
	}
	return tools, warnings, nil
}

func (c *Conversation) resolveMCPTools(ctx context.Context) ([]Tool, []Warning, error) {
	if len(c.MCPServers) == 0 {
		if len(c.mcpClients) > 0 {
			c.closeMCP(ctx)
		}
		c.mcpCacheKey = ""
		c.mcpToolCache = nil
		return nil, nil, nil
	}

	key, err := mcpServerSetKey(c.MCPServers)
	if err != nil {
		return nil, nil, err
	}
	if key == c.mcpCacheKey {
		return append([]Tool(nil), c.mcpToolCache...), c.mcpSchemaWarnings(c.mcpToolCache), nil
	}

	c.closeMCP(ctx)
	c.mcpClients = make(map[string]*mcp.Client, len(c.MCPServers))

	seenServers := make(map[string]struct{}, len(c.MCPServers))
	var tools []Tool
	for _, server := range c.MCPServers {
		if server.Name == "" || server.URL == "" {
			return nil, nil, ErrInvalidConfig
		}
		if _, ok := seenServers[server.Name]; ok {
			return nil, nil, ErrInvalidConfig
		}
		seenServers[server.Name] = struct{}{}

		client := mcp.New(mcp.Config{URL: server.URL, Headers: server.Headers})
		list, err := c.discoverMCPTools(ctx, server.Name, client)
		if err != nil {
			c.closeMCP(ctx)
			return nil, nil, err
		}
		c.mcpClients[server.Name] = client
		for _, discovered := range list {
			name := sanitizeMCPToolName(server.Name + "_" + discovered.Name)
			schema := discovered.InputSchema
			if len(bytes.TrimSpace(schema)) == 0 {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			tools = append(tools, &mcpTool{
				name:         name,
				description:  discovered.Description,
				schema:       append(json.RawMessage(nil), schema...),
				server:       server.Name,
				originalName: discovered.Name,
				client:       client,
			})
		}
	}

	if _, err := validateAndSortTools(append(append([]Tool(nil), c.Tools...), tools...)); err != nil {
		c.closeMCP(ctx)
		return nil, nil, err
	}
	c.mcpCacheKey = key
	c.mcpToolCache = append([]Tool(nil), tools...)
	return append([]Tool(nil), tools...), c.mcpSchemaWarnings(tools), nil
}

func (c *Conversation) discoverMCPTools(ctx context.Context, serverName string, client *mcp.Client) ([]mcp.Tool, error) {
	policy := c.Retry.withDefaults()
	clock := c.retryClock
	if clock == nil {
		clock = realRetryClock{}
	}
	start := clock.Now()

	for attempt := 1; ; attempt++ {
		tools, err := client.ListTools(ctx)
		if err == nil {
			return tools, nil
		}
		mapped := mcpError(serverName, err)
		if !isRetryable(mapped) || attempt >= policy.MaxAttempts {
			return nil, mapped
		}
		delay := retryDelay(policy, clock, start, attempt, mapped)
		if delay < 0 {
			return nil, mapped
		}
		if err := clock.Sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func (c *Conversation) mcpSchemaWarnings(tools []Tool) []Warning {
	if c.Provider == nil || c.Provider.Name() != "google" {
		return nil
	}
	var warnings []Warning
	for _, tool := range tools {
		mt, ok := tool.(*mcpTool)
		if !ok {
			continue
		}
		keywords := unsupportedGeminiSchemaKeywords(mt.schema)
		if len(keywords) == 0 {
			continue
		}
		warnings = append(warnings, Warning{
			Setting: "mcp_schema",
			Detail:  fmt.Sprintf("%s.%s drops unsupported schema keywords for Gemini: %s", mt.server, mt.originalName, strings.Join(keywords, ", ")),
		})
	}
	return warnings
}

func (c *Conversation) closeMCP(ctx context.Context) {
	for _, client := range c.mcpClients {
		_ = client.DeleteSession(ctx)
	}
	c.mcpClients = nil
	c.mcpToolCache = nil
	c.mcpCacheKey = ""
}

func mcpServerSetKey(servers []MCPServer) (string, error) {
	parts := make([]string, 0, len(servers))
	for _, server := range servers {
		if server.Name == "" || server.URL == "" {
			return "", ErrInvalidConfig
		}
		headers := make([]string, 0, len(server.Headers))
		for k, v := range server.Headers {
			headers = append(headers, k+"\x00"+v)
		}
		sort.Strings(headers)
		parts = append(parts, server.Name+"\x00"+server.URL+"\x00"+strings.Join(headers, "\x00"))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x01"), nil
}

func sanitizeMCPToolName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if isToolNameRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || !isToolNameStart(rune(out[0])) {
		out = "_" + out
	}
	if len(out) <= 64 {
		return out
	}
	hash := sha256.Sum256([]byte(out))
	suffix := "_" + hex.EncodeToString(hash[:4])
	return out[:64-len(suffix)] + suffix
}

func isToolNameRune(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isToolNameStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func mcpResultText(result mcp.CallResult) string {
	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if content.Text != "" {
			parts = append(parts, content.Text)
			continue
		}
		if len(content.Raw) > 0 {
			parts = append(parts, string(content.Raw))
		}
	}
	if len(parts) == 0 && len(result.Raw) > 0 {
		return string(result.Raw)
	}
	return strings.Join(parts, "\n")
}

func mcpError(server string, err error) error {
	if err == nil {
		return nil
	}
	var rpcErr *mcp.RPCError
	if errors.As(err, &rpcErr) {
		return &Error{
			Category:  mcpRPCCategory(rpcErr.Code),
			MCPServer: server,
			Type:      strconv.Itoa(rpcErr.Code),
			Message:   rpcErr.Message,
			Raw:       append(json.RawMessage(nil), rpcErr.Raw...),
			Err:       err,
		}
	}
	var httpErr *mcp.HTTPError
	if errors.As(err, &httpErr) {
		raw := append(json.RawMessage(nil), httpErr.Raw...)
		message := httpErr.Message
		if httpErr.StatusCode == http.StatusUnauthorized && httpErr.WWWAuthenticate != "" {
			message = httpErr.WWWAuthenticate
			raw, _ = json.Marshal(httpErr.WWWAuthenticate)
		}
		return &Error{
			Category:   mcpHTTPCategory(httpErr.StatusCode),
			MCPServer:  server,
			StatusCode: httpErr.StatusCode,
			Type:       strconv.Itoa(httpErr.StatusCode),
			Message:    message,
			Raw:        raw,
			Err:        err,
		}
	}
	category := ErrNetwork
	if errors.Is(err, context.DeadlineExceeded) {
		category = ErrTimeout
	} else if errors.Is(err, context.Canceled) {
		category = ErrNetwork
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			category = ErrTimeout
		} else if errors.Is(err, io.ErrUnexpectedEOF) {
			category = ErrServerError
		}
	}
	return &Error{
		Category:  category,
		MCPServer: server,
		Message:   err.Error(),
		Err:       err,
	}
}

func mcpRPCCategory(code int) error {
	switch {
	case code == -32600 || code == -32601 || code == -32602:
		return ErrInvalidRequest
	case code == -32603 || code == -32700 || (code >= -32099 && code <= -32000):
		return ErrServerError
	default:
		return ErrUnknown
	}
}

func mcpHTTPCategory(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return ErrAuthentication
	case http.StatusForbidden:
		return ErrPermission
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return ErrTimeout
	case http.StatusTooManyRequests:
		return ErrRateLimited
	}
	if status >= 500 {
		return ErrServerError
	}
	if status >= 400 {
		return ErrInvalidRequest
	}
	return ErrUnknown
}

func unsupportedGeminiSchemaKeywords(raw json.RawMessage) []string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	collectUnsupportedSchemaKeywords(v, seen)
	keywords := make([]string, 0, len(seen))
	for keyword := range seen {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	return keywords
}

func collectUnsupportedSchemaKeywords(v any, seen map[string]struct{}) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			switch k {
			case "$ref", "additionalProperties", "oneOf":
				seen[k] = struct{}{}
			}
			collectUnsupportedSchemaKeywords(child, seen)
		}
	case []any:
		for _, child := range v {
			collectUnsupportedSchemaKeywords(child, seen)
		}
	}
}

var _ Tool = (*mcpTool)(nil)
var _ error = terminalToolError{}
