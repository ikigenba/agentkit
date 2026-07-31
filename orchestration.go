package agentkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ikigenba/agentkit/internal/mcp"
	internalretry "github.com/ikigenba/agentkit/internal/retry"
)

const defaultMaxToolIterations = 1000

var (
	// ErrInvalidConfig reports an unusable Conversation or Tool setup.
	ErrInvalidConfig = errors.New("agentkit: invalid configuration")
	// ErrMissingCredential reports that an operation needed a credential that
	// was never supplied.
	ErrMissingCredential = errors.New("agentkit: missing credential")
	// ErrInvalidInput reports a bad Send argument.
	ErrInvalidInput = errors.New("agentkit: invalid input")
	// ErrToolLoopLimit reports a runaway automatic tool loop.
	ErrToolLoopLimit = errors.New("agentkit: tool-loop iteration limit exceeded")
	// ErrStreamPending reports a Send while the prior Stream is still live.
	ErrStreamPending = errors.New("agentkit: prior stream not yet drained")
	// ErrClosed reports a Send after the conversation lifecycle is closed.
	ErrClosed = errors.New("agentkit: conversation closed")
)

// Provider is implemented by provider sub-packages. Consumers obtain a value
// from a provider package and assign it to Conversation.Provider.
type Provider interface {
	RoundTrip(ctx context.Context, req *Request) *RoundTrip
	Identity() Identity
}

// Request is one provider round-trip's input, built by the orchestrator.
type Request struct {
	Model           string
	System          string
	Messages        []Message
	Tools           []Tool
	Gen             GenSettings
	ProviderOptions json.RawMessage
}

// MCPServer is a remote MCP Streamable-HTTP tool server attached to a
// Conversation. Servers attach and detach by mutating Conversation.MCPServers
// between turns.
type MCPServer struct {
	Name    string
	URL     string
	Headers map[string]string
}

// FinishReason is the normalized reason a round-trip ended.
type FinishReason int

const (
	FinishStop FinishReason = iota
	FinishToolUse
	FinishMaxTokens
	FinishContentFilter
	FinishOther
)

// RoundTrip is one low-level provider call result.
type RoundTrip struct {
	message        Message
	finish         FinishReason
	usage          Usage
	warnings       []Warning
	err            error
	reportedCost   Cost
	reportedCostOK bool
}

// NewRoundTrip builds a provider SPI result. It normalizes the assembled
// message so adjacent TextBlocks are joined verbatim before storage.
func NewRoundTrip(message Message, finish FinishReason, usage Usage, warnings []Warning, err error, reportedCost Cost, reportedCostOK bool) *RoundTrip {
	message = cloneMessage(message)
	message.Blocks = mergeAdjacentText(message.Blocks)
	return &RoundTrip{
		message:        message,
		finish:         finish,
		usage:          usage,
		warnings:       append([]Warning(nil), warnings...),
		err:            err,
		reportedCost:   reportedCost,
		reportedCostOK: reportedCostOK,
	}
}

// mergeAdjacentText replaces each maximal run of TextBlocks with one block
// containing their verbatim concatenation. Empty runs are omitted.
func mergeAdjacentText(blocks []Block) []Block {
	merged := make([]Block, 0, len(blocks))
	var text strings.Builder
	flushText := func() {
		if text.Len() > 0 {
			merged = append(merged, TextBlock{Text: text.String()})
		}
		text.Reset()
	}

	for _, block := range blocks {
		if block, ok := block.(TextBlock); ok {
			text.WriteString(block.Text)
			continue
		}
		flushText()
		merged = append(merged, block)
	}
	flushText()
	return merged
}

// Message returns the assembled assistant message.
func (r *RoundTrip) Message() Message {
	if r == nil {
		return Message{}
	}
	return cloneMessage(r.message)
}

// Finish returns the normalized reason the provider round-trip ended.
func (r *RoundTrip) Finish() FinishReason {
	if r == nil {
		return FinishOther
	}
	return r.finish
}

// Usage returns this provider round-trip's token usage.
func (r *RoundTrip) Usage() Usage {
	if r == nil {
		return Usage{}
	}
	return r.usage
}

// Warnings returns generation-setting degradations from this provider call.
func (r *RoundTrip) Warnings() []Warning {
	if r == nil {
		return nil
	}
	return append([]Warning(nil), r.warnings...)
}

// Err returns this provider round-trip's terminal error.
func (r *RoundTrip) Err() error {
	if r == nil {
		return ErrInvalidConfig
	}
	return r.err
}

// ReportedCost returns the provider-stated charge for this call, when present.
func (r *RoundTrip) ReportedCost() (Cost, bool) {
	if r == nil {
		return 0, false
	}
	return r.reportedCost, r.reportedCostOK
}

// Event is one observable item in a Conversation stream.
type Event interface {
	isEvent()
}

// ToolUse reports a complete tool call requested by the model.
type ToolUse struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult reports a tool result AgentKit fed back into the loop.
type ToolResult struct {
	ID      string
	Name    string
	Output  string
	IsError bool
}

// MessageDone marks one completed assistant message.
type MessageDone struct {
	Message Message
}

func (ToolUse) isEvent()     {}
func (ToolResult) isEvent()  {}
func (MessageDone) isEvent() {}

// Conversation is one multi-turn, tool-using text conversation with an LLM.
//
// It is not safe for concurrent use.
type Conversation struct {
	Provider          Provider
	Model             string
	Pricing           *Pricing
	System            string
	Log               io.Writer
	Gen               GenSettings
	Retry             RetryPolicy
	Tools             []Tool
	DeferredTools     []DeferredToolGroup
	MCPServers        []MCPServer
	History           []Message
	MaxToolIterations int

	streamLive          bool
	closed              bool
	turns               int
	totalUsage          Usage
	totalCost           Cost
	retryClock          retryClock
	mcpCacheKey         string
	mcpClients          map[string]*mcp.Client
	mcpToolCache        []Tool
	loadedDeferredNames []string
}

// Send starts one turn and returns its stream.
func (c *Conversation) Send(ctx context.Context, userText string) *Stream {
	if ctx == nil {
		ctx = context.Background()
	}
	if c != nil && c.closed {
		return errorStream(ErrClosed)
	}
	if c == nil || c.Provider == nil || c.Model == "" {
		return errorStream(ErrInvalidConfig)
	}
	if userText == "" {
		return errorStream(ErrInvalidInput)
	}
	if c.streamLive {
		return errorStream(ErrStreamPending)
	}

	tools, warnings, err := c.resolveTools(ctx)
	if err != nil {
		return errorStream(err)
	}

	history := append(cloneMessages(c.History), Message{
		Role:   RoleUser,
		Blocks: []Block{TextBlock{Text: userText}},
	})
	c.streamLive = true

	s := &Stream{warnings: warnings}
	s.run = func(yield func(Event) bool) (bool, error) {
		identity := c.Provider.Identity()
		s.log(c, LogRecord{Type: "turn_start", Provider: identity.Provider, Auth: identity.Auth, Model: c.Model})
		success, err := c.runTurn(ctx, &history, tools, s, yield)
		if success {
			usage := s.usage
			cost := s.cost
			s.log(c, LogRecord{Type: "usage", Usage: &usage, Cost: &cost})
			s.log(c, LogRecord{Type: "turn_end", Status: "ok"})
		} else if err != nil {
			s.logError(c, "error", err)
			s.log(c, LogRecord{Type: "turn_end", Status: "error"})
		} else {
			s.log(c, LogRecord{Type: "turn_end", Status: "abandoned"})
		}
		if success {
			c.History = history
		}
		return success, err
	}
	s.onDone = func(success bool) {
		if success {
			c.turns++
			c.totalUsage = addUsage(c.totalUsage, s.usage)
			c.totalCost += s.cost
		}
		c.streamLive = false
	}
	return s
}

// Close marks the conversation closed and emits a cumulative summary record.
func (c *Conversation) Close() error {
	if c == nil {
		return ErrInvalidConfig
	}
	if c.closed {
		return nil
	}
	c.closed = true
	c.closeMCP(context.Background())
	if c.Log != nil {
		usage := c.totalUsage
		cost := c.totalCost
		record := LogRecord{
			Type:  "summary",
			Time:  c.logNow(),
			Seq:   0,
			Usage: &usage,
			Turns: c.turns,
			Cost:  &cost,
		}
		_ = json.NewEncoder(c.Log).Encode(record)
	}
	return nil
}

// TotalUsage returns the cumulative usage of successfully completed turns.
func (c *Conversation) TotalUsage() Usage {
	if c == nil {
		return Usage{}
	}
	return c.totalUsage
}

// TotalCost returns the cumulative cost of successfully completed turns.
func (c *Conversation) TotalCost() Cost {
	if c == nil {
		return 0
	}
	return c.totalCost
}

func (c *Conversation) runTurn(ctx context.Context, history *[]Message, tools []Tool, s *Stream, yield func(Event) bool) (bool, error) {
	toolByName := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		toolByName[tool.Name()] = tool
	}
	deferredByName, err := c.deferredToolCatalog(nil)
	if err != nil {
		return false, err
	}

	maxIterations := c.MaxToolIterations
	if maxIterations == 0 {
		maxIterations = defaultMaxToolIterations
	}

	var toolIterations int
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		rt, stopped, err := c.roundTripWithRetry(ctx, &Request{
			Model:    c.Model,
			System:   c.System,
			Messages: cloneMessages(*history),
			Tools:    append([]Tool(nil), tools...),
			Gen:      c.Gen,
		}, s, yield)
		if stopped {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if rt.Finish() == FinishContentFilter {
			return false, ErrContentFilter
		}

		message := rt.Message()
		*history = append(*history, message)
		usage := rt.Usage()
		s.usage = addUsage(s.usage, usage)
		warnings := rt.Warnings()
		if reported, ok := rt.ReportedCost(); ok {
			s.cost += reported
		} else if c.Pricing != nil {
			s.cost += c.Pricing.Cost(usage)
		} else {
			warnings = append(warnings, Warning{
				Setting: "cost",
				Code:    WarnCostUnknown,
				Detail:  "no reported or consumer-supplied cost; applied 0",
			})
		}
		s.warnings = append(s.warnings, warnings...)
		for _, warning := range warnings {
			warning := warning
			s.log(c, LogRecord{Type: "warning", Warning: &warning})
		}

		messageForLog := cloneMessage(message)
		s.log(c, LogRecord{Type: "message", Message: &messageForLog})
		if !yield(MessageDone{Message: cloneMessage(message)}) {
			return false, nil
		}

		uses := toolUses(message)
		if len(uses) == 0 {
			return true, nil
		}
		if toolIterations >= maxIterations {
			return false, ErrToolLoopLimit
		}
		toolIterations++

		resultBlocks := make([]Block, 0, len(uses))
		for _, use := range uses {
			toolUse := ToolUse{ID: use.ID, Name: use.Name, Input: cloneRaw(use.Input)}
			s.log(c, LogRecord{Type: "tool_use", ToolUse: &toolUse})
			if !yield(ToolUse{ID: use.ID, Name: use.Name, Input: cloneRaw(use.Input)}) {
				return false, nil
			}

			var result ToolResultBlock
			var newlyLoaded []Tool
			if use.Name == loadToolsName && len(c.DeferredTools) > 0 {
				result, newlyLoaded, err = c.runLoadTools(ctx, deferredByName, use)
			} else if toolByName[use.Name] == nil {
				result, newlyLoaded, err = c.runDeferredToolMiss(ctx, deferredByName, use)
			} else {
				result, err = runTool(ctx, toolByName[use.Name], use)
			}
			if err != nil {
				return false, err
			}
			for _, tool := range newlyLoaded {
				if toolByName[tool.Name()] != nil {
					continue
				}
				tools = append(tools, tool)
				toolByName[tool.Name()] = tool
			}
			resultBlocks = append(resultBlocks, result)
			toolResult := ToolResult{ID: result.ToolUseID, Name: result.Name, Output: result.Content, IsError: result.IsError}
			s.log(c, LogRecord{Type: "tool_result", Result: &toolResult})
			if !yield(ToolResult{ID: result.ToolUseID, Name: result.Name, Output: result.Content, IsError: result.IsError}) {
				return false, nil
			}
		}

		*history = append(*history, Message{Role: RoleUser, Blocks: resultBlocks})
	}
}

func (c *Conversation) roundTripWithRetry(ctx context.Context, req *Request, s *Stream, yield func(Event) bool) (*RoundTrip, bool, error) {
	clock := c.retryClock
	if clock == nil {
		clock = realRetryClock{}
	}
	rt, err := internalretry.Do(ctx, retryPolicy(c.Retry), clock, func() (*RoundTrip, error) {
		rt := c.Provider.RoundTrip(ctx, req)
		if rt == nil {
			return nil, ErrInvalidConfig
		}
		return rt, rt.Err()
	}, retryDecision, func(err error, _ time.Duration) {
		s.logError(c, "retry", err)
	})
	if err != nil {
		return nil, false, err
	}
	return rt, false, nil
}

func validateAndSortTools(tools []Tool) ([]Tool, error) {
	seen := make(map[string]struct{}, len(tools))
	sorted := append([]Tool(nil), tools...)
	for _, tool := range sorted {
		if tool == nil || tool.Name() == "" {
			return nil, ErrInvalidConfig
		}
		if _, ok := seen[tool.Name()]; ok {
			return nil, ErrInvalidConfig
		}
		seen[tool.Name()] = struct{}{}
		if err := validateToolSchema(tool.JSONSchema()); err != nil {
			return nil, fmt.Errorf("%w: tool %s schema %v", ErrInvalidConfig, toolValidationName(tool), err)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name() < sorted[j].Name()
	})
	return sorted, nil
}

func toolValidationName(tool Tool) string {
	if tool, ok := tool.(*mcpTool); ok {
		return tool.server + "." + tool.originalName
	}
	return tool.Name()
}

var toolSchemaFormats = map[string]struct{}{
	"date-time": {},
	"date":      {},
	"time":      {},
	"duration":  {},
	"email":     {},
	"hostname":  {},
	"ipv4":      {},
	"ipv6":      {},
	"uuid":      {},
}

// validateToolSchema is the sole authority on membership in AgentKit's
// provider-independent, canonical tool-schema subset.
func validateToolSchema(schema json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(schema))
	decoder.UseNumber()
	var v any
	if err := decoder.Decode(&v); err != nil {
		return fmt.Errorf("at #: invalid JSON: %v", err)
	}
	if err := decoder.Decode(new(any)); err == nil {
		return errors.New("at #: invalid JSON: multiple values")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("at #: invalid JSON: %v", err)
	}

	root, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("at #: root shape must be an object schema with type %q", "object")
	}
	if typ, ok := root["type"].(string); !ok || typ != "object" {
		return fmt.Errorf("at #/type: root shape must have type %q", "object")
	}
	if err := validateSchemaObject(root, "#"); err != nil {
		return err
	}
	return validateSchemaRefs(root, root, "#", nil)
}

func validateSchemaObject(schema map[string]any, pointer string) error {
	keywords := make([]string, 0, len(schema))
	for keyword := range schema {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	for _, keyword := range keywords {
		value := schema[keyword]
		location := pointer + "/" + escapeJSONPointer(keyword)
		switch keyword {
		case "type":
			typ, ok := value.(string)
			if !ok || !validSchemaType(typ) {
				return fmt.Errorf("at %s: type must be one supported single string value", location)
			}
		case "description", "title":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("at %s: %s must be a string", location, keyword)
			}
		case "properties", "$defs":
			children, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("at %s: %s must be an object", location, keyword)
			}
			for name, child := range children {
				childSchema, ok := child.(map[string]any)
				if !ok {
					return fmt.Errorf("at %s/%s: schema must be an object", location, escapeJSONPointer(name))
				}
				if err := validateSchemaObject(childSchema, location+"/"+escapeJSONPointer(name)); err != nil {
					return err
				}
			}
		case "required":
			if !stringArray(value) {
				return fmt.Errorf("at %s: required must contain only strings", location)
			}
		case "items":
			child, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("at %s: items must be a schema object", location)
			}
			if err := validateSchemaObject(child, location); err != nil {
				return err
			}
		case "enum":
			if !stringArray(value) {
				return fmt.Errorf("at %s: enum must contain only string values", location)
			}
		case "const":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("at %s: const must be a string value", location)
			}
		case "anyOf", "oneOf":
			children, ok := value.([]any)
			if !ok || len(children) == 0 {
				return fmt.Errorf("at %s: %s must be a non-empty schema array", location, keyword)
			}
			for i, child := range children {
				childSchema, ok := child.(map[string]any)
				childLocation := location + "/" + strconv.Itoa(i)
				if !ok {
					return fmt.Errorf("at %s: schema must be an object", childLocation)
				}
				if err := validateSchemaObject(childSchema, childLocation); err != nil {
					return err
				}
			}
		case "$ref":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("at %s: $ref must be a string", location)
			}
		case "minLength", "maxLength":
			if !nonnegativeInteger(value) {
				return fmt.Errorf("at %s: %s must be a non-negative integer", location, keyword)
			}
		case "pattern":
			pattern, ok := value.(string)
			if !ok {
				return fmt.Errorf("at %s: pattern must be a string", location)
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("at %s: pattern is not RE2-safe: %v", location, err)
			}
		case "minItems":
			number, ok := value.(json.Number)
			if !ok || (number.String() != "0" && number.String() != "1") {
				return fmt.Errorf("at %s: minItems must be 0 or 1", location)
			}
		case "format":
			format, ok := value.(string)
			if _, allowed := toolSchemaFormats[format]; !ok || !allowed {
				return fmt.Errorf("at %s: format %q is outside the canonical allowlist", location, format)
			}
		case "default":
			// Defaults are annotations and may carry any JSON value.
		default:
			return fmt.Errorf("at %s: construct %q is outside the canonical subset", location, keyword)
		}
	}
	return nil
}

func validateSchemaRefs(schema, root map[string]any, pointer string, active map[string]bool) error {
	if ref, ok := schema["$ref"].(string); ok {
		location := pointer + "/$ref"
		target, found := resolveToolSchemaRef(root, ref)
		if !found {
			return fmt.Errorf("at %s: $ref %q cannot be resolved", location, ref)
		}
		if active[ref] {
			return fmt.Errorf("at %s: recursive $ref %q is outside the canonical subset", location, ref)
		}
		next := make(map[string]bool, len(active)+1)
		for key, value := range active {
			next[key] = value
		}
		next[ref] = true
		if err := validateSchemaRefs(target, root, location, next); err != nil {
			return err
		}
	}

	for _, keyword := range []string{"properties", "$defs"} {
		children, _ := schema[keyword].(map[string]any)
		for name, child := range children {
			if err := validateSchemaRefs(child.(map[string]any), root, pointer+"/"+escapeJSONPointer(keyword)+"/"+escapeJSONPointer(name), active); err != nil {
				return err
			}
		}
	}
	if child, ok := schema["items"].(map[string]any); ok {
		if err := validateSchemaRefs(child, root, pointer+"/items", active); err != nil {
			return err
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		children, _ := schema[keyword].([]any)
		for i, child := range children {
			if err := validateSchemaRefs(child.(map[string]any), root, pointer+"/"+keyword+"/"+strconv.Itoa(i), active); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveToolSchemaRef(root map[string]any, ref string) (map[string]any, bool) {
	if ref == "#" {
		return root, true
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	var current any = root
	for _, token := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[token]
		if !ok {
			return nil, false
		}
	}
	resolved, ok := current.(map[string]any)
	return resolved, ok
}

func validSchemaType(typ string) bool {
	switch typ {
	case "object", "array", "string", "number", "integer", "boolean", "null":
		return true
	default:
		return false
	}
}

func stringArray(value any) bool {
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func nonnegativeInteger(value any) bool {
	number, ok := value.(json.Number)
	if !ok || strings.ContainsAny(number.String(), ".eE-") {
		return false
	}
	_, err := strconv.ParseUint(number.String(), 10, 64)
	return err == nil
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func toolUses(message Message) []ToolUseBlock {
	uses := make([]ToolUseBlock, 0)
	for _, block := range message.Blocks {
		if use, ok := block.(ToolUseBlock); ok {
			uses = append(uses, ToolUseBlock{
				ID:    use.ID,
				Name:  use.Name,
				Input: cloneRaw(use.Input),
			})
		}
	}
	return uses
}

func runTool(ctx context.Context, tool Tool, use ToolUseBlock) (ToolResultBlock, error) {
	if tool == nil {
		return ToolResultBlock{
			ToolUseID: use.ID,
			Name:      use.Name,
			Content:   fmt.Sprintf("unknown tool: %s", use.Name),
			IsError:   true,
		}, nil
	}

	output, err := tool.Call(ctx, use.Input)
	if err != nil {
		var terminal terminalToolError
		if errors.As(err, &terminal) {
			return ToolResultBlock{}, terminal.err
		}
		return ToolResultBlock{
			ToolUseID: use.ID,
			Name:      use.Name,
			Content:   err.Error(),
			IsError:   true,
		}, nil
	}
	return ToolResultBlock{
		ToolUseID: use.ID,
		Name:      use.Name,
		Content:   output,
	}, nil
}

func cloneMessages(messages []Message) []Message {
	cloned := make([]Message, len(messages))
	for i, message := range messages {
		cloned[i] = cloneMessage(message)
	}
	return cloned
}

func cloneMessage(message Message) Message {
	return Message{
		Role:   message.Role,
		Blocks: cloneBlocks(message.Blocks),
	}
}

func cloneBlocks(blocks []Block) []Block {
	cloned := make([]Block, len(blocks))
	for i, block := range blocks {
		switch block := block.(type) {
		case TextBlock:
			cloned[i] = block
		case ToolUseBlock:
			block.Input = cloneRaw(block.Input)
			cloned[i] = block
		case ToolResultBlock:
			cloned[i] = block
		case ReasoningBlock:
			block.Opaque = cloneRaw(block.Opaque)
			cloned[i] = block
		default:
			cloned[i] = block
		}
	}
	return cloned
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func addUsage(a, b Usage) Usage {
	return Usage{
		InputUncached:   a.InputUncached + b.InputUncached,
		CacheReadInput:  a.CacheReadInput + b.CacheReadInput,
		CacheWriteInput: a.CacheWriteInput + b.CacheWriteInput,
		CacheWrite5m:    a.CacheWrite5m + b.CacheWrite5m,
		CacheWrite1h:    a.CacheWrite1h + b.CacheWrite1h,
		Output:          a.Output + b.Output,
		ReasoningOutput: a.ReasoningOutput + b.ReasoningOutput,
		Total:           a.Total + b.Total,
	}
}
