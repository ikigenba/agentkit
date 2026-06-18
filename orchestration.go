package agentkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"sort"
	"time"
)

const defaultMaxToolIterations = 1000

var (
	// ErrInvalidConfig reports an unusable Conversation or Tool setup.
	ErrInvalidConfig = errors.New("agentkit: invalid configuration")
	// ErrInvalidInput reports a bad Send argument.
	ErrInvalidInput = errors.New("agentkit: invalid input")
	// ErrToolLoopLimit reports a runaway automatic tool loop.
	ErrToolLoopLimit = errors.New("agentkit: tool-loop iteration limit exceeded")
	// ErrStreamPending reports a Send while the prior Stream is still live.
	ErrStreamPending = errors.New("agentkit: prior stream not yet drained")
)

// Provider is implemented by provider sub-packages. Consumers obtain a value
// from a provider package and assign it to Conversation.Provider.
type Provider interface {
	RoundTrip(ctx context.Context, req *Request) *RoundTrip
	Name() string
	Pricing(model string) (Pricing, bool)
}

// Request is one provider round-trip's input, built by the orchestrator.
type Request struct {
	Model    string
	System   string
	Messages []Message
	Tools    []Tool
	Gen      GenSettings
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
	events   iter.Seq[Event]
	message  Message
	finish   FinishReason
	usage    Usage
	warnings []Warning
	err      error
}

// NewRoundTrip builds a provider SPI result.
func NewRoundTrip(events iter.Seq[Event], message Message, finish FinishReason, usage Usage, warnings []Warning, err error) *RoundTrip {
	return &RoundTrip{
		events:   events,
		message:  cloneMessage(message),
		finish:   finish,
		usage:    usage,
		warnings: append([]Warning(nil), warnings...),
		err:      err,
	}
}

// Events yields TextDelta and ReasoningDelta events from this provider call.
func (r *RoundTrip) Events() iter.Seq[Event] {
	return func(yield func(Event) bool) {
		if r == nil || r.events == nil {
			return
		}
		for ev := range r.events {
			if !yield(ev) {
				return
			}
		}
	}
}

// Message returns the assembled assistant message after Events is drained.
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

// Event is one observable item in a Conversation stream.
type Event interface {
	isEvent()
}

// TextDelta is a visible answer fragment.
type TextDelta struct {
	Text string
}

// ReasoningDelta is a human-readable reasoning-summary fragment.
type ReasoningDelta struct {
	Text string
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

func (TextDelta) isEvent()      {}
func (ReasoningDelta) isEvent() {}
func (ToolUse) isEvent()        {}
func (ToolResult) isEvent()     {}
func (MessageDone) isEvent()    {}

// Conversation is one multi-turn, tool-using text conversation with an LLM.
//
// It is not safe for concurrent use.
type Conversation struct {
	Provider          Provider
	Model             string
	System            string
	Gen               GenSettings
	Retry             RetryPolicy
	Tools             []Tool
	History           []Message
	MaxToolIterations int

	streamLive bool
	totalCost  Cost
	retryClock retryClock
}

// Send starts one turn and returns its stream.
func (c *Conversation) Send(ctx context.Context, userText string) *Stream {
	if ctx == nil {
		ctx = context.Background()
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

	pricing, ok := c.Provider.Pricing(c.Model)
	if !ok {
		return errorStream(ErrInvalidConfig)
	}
	tools, err := validateAndSortTools(c.Tools)
	if err != nil {
		return errorStream(err)
	}

	history := append(cloneMessages(c.History), Message{
		Role:   RoleUser,
		Blocks: []Block{TextBlock{Text: userText}},
	})
	c.streamLive = true

	s := &Stream{}
	s.run = func(yield func(Event) bool) (bool, error) {
		success, err := c.runTurn(ctx, &history, tools, pricing, s, yield)
		if success {
			c.History = history
		}
		return success, err
	}
	s.onDone = func(success bool) {
		if success {
			c.totalCost += s.cost
		}
		c.streamLive = false
	}
	return s
}

// TotalCost returns the cumulative cost of successfully completed turns.
func (c *Conversation) TotalCost() Cost {
	if c == nil {
		return 0
	}
	return c.totalCost
}

func (c *Conversation) runTurn(ctx context.Context, history *[]Message, tools []Tool, pricing Pricing, s *Stream, yield func(Event) bool) (bool, error) {
	toolByName := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		toolByName[tool.Name()] = tool
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
		}, yield)
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
		s.usage = addUsage(s.usage, rt.Usage())
		s.warnings = append(s.warnings, rt.Warnings()...)
		s.cost = pricing.Cost(s.usage)

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
			if !yield(ToolUse{ID: use.ID, Name: use.Name, Input: cloneRaw(use.Input)}) {
				return false, nil
			}

			result := runTool(ctx, toolByName[use.Name], use)
			resultBlocks = append(resultBlocks, result)
			if !yield(ToolResult{ID: result.ToolUseID, Name: result.Name, Output: result.Content, IsError: result.IsError}) {
				return false, nil
			}
		}

		*history = append(*history, Message{Role: RoleUser, Blocks: resultBlocks})
	}
}

func (c *Conversation) roundTripWithRetry(ctx context.Context, req *Request, yield func(Event) bool) (*RoundTrip, bool, error) {
	policy := c.Retry.withDefaults()
	clock := c.retryClock
	if clock == nil {
		clock = realRetryClock{}
	}
	start := clock.Now()

	for attempt := 1; ; attempt++ {
		rt := c.Provider.RoundTrip(ctx, req)
		if rt == nil {
			return nil, false, ErrInvalidConfig
		}

		delivered := false
		for ev := range rt.Events() {
			switch ev.(type) {
			case TextDelta, ReasoningDelta:
			default:
				return nil, false, ErrInvalidConfig
			}
			if !yield(ev) {
				return nil, true, nil
			}
			delivered = true
		}

		err := rt.Err()
		if err == nil {
			return rt, false, nil
		}
		if delivered || !isRetryable(err) || attempt >= policy.MaxAttempts {
			return nil, false, err
		}

		delay := retryDelay(policy, clock, start, attempt, err)
		if delay < 0 {
			return nil, false, err
		}
		if err := clock.Sleep(ctx, delay); err != nil {
			return nil, false, err
		}
	}
}

func isRetryable(err error) bool {
	return errors.Is(err, ErrRateLimited) ||
		errors.Is(err, ErrOverloaded) ||
		errors.Is(err, ErrServerError) ||
		errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrNetwork)
}

func retryDelay(policy RetryPolicy, clock retryClock, start time.Time, attempt int, err error) time.Duration {
	var providerErr *Error
	if !policy.IgnoreRetryAfter && errors.As(err, &providerErr) && providerErr.RetryAfter > 0 {
		return boundedRetryDelay(policy, clock, start, providerErr.RetryAfter)
	}
	return boundedRetryDelay(policy, clock, start, clock.Jitter(backoffCap(policy, attempt)))
}

func boundedRetryDelay(policy RetryPolicy, clock retryClock, start time.Time, delay time.Duration) time.Duration {
	if delay < 0 {
		delay = 0
	}
	if policy.MaxElapsed == 0 {
		return delay
	}
	remaining := policy.MaxElapsed - clock.Now().Sub(start)
	if remaining < 0 || delay > remaining {
		return -1
	}
	return delay
}

func backoffCap(policy RetryPolicy, attempt int) time.Duration {
	delay := policy.BaseDelay
	for i := 1; i < attempt && delay < policy.MaxDelay; i++ {
		if delay > policy.MaxDelay/2 {
			delay = policy.MaxDelay
			break
		}
		delay *= 2
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
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
		if !validJSONSchema(tool.JSONSchema()) {
			return nil, ErrInvalidConfig
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name() < sorted[j].Name()
	})
	return sorted, nil
}

func validJSONSchema(schema json.RawMessage) bool {
	var v any
	if err := json.Unmarshal(schema, &v); err != nil {
		return false
	}
	switch v.(type) {
	case bool, map[string]any:
		return true
	default:
		return false
	}
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

func runTool(ctx context.Context, tool Tool, use ToolUseBlock) ToolResultBlock {
	if tool == nil {
		return ToolResultBlock{
			ToolUseID: use.ID,
			Name:      use.Name,
			Content:   fmt.Sprintf("unknown tool: %s", use.Name),
			IsError:   true,
		}
	}

	output, err := tool.Call(ctx, use.Input)
	if err != nil {
		return ToolResultBlock{
			ToolUseID: use.ID,
			Name:      use.Name,
			Content:   err.Error(),
			IsError:   true,
		}
	}
	return ToolResultBlock{
		ToolUseID: use.ID,
		Name:      use.Name,
		Content:   output,
	}
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
