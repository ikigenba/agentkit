package agentkit_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ikigenba/agentkit"
)

const (
	testModel       = "test-model"
	secondModel     = "second-model"
	testToolUseID   = "toolu_123"
	secondToolUseID = "toolu_456"
)

var testPricing = agentkit.Pricing{Tiers: []agentkit.RateTier{{
	MinInputTokens: 0,
	InputUncached:  10,
	Output:         20,
}}}

type fakeProvider struct {
	name        string
	models      map[string]agentkit.Pricing
	roundTrips  []*agentkit.RoundTrip
	roundTripFn func(context.Context, *agentkit.Request) *agentkit.RoundTrip
	calls       []agentkit.Request
}

func newFakeProvider(roundTrips ...*agentkit.RoundTrip) *fakeProvider {
	return &fakeProvider{
		name:       "fake",
		models:     map[string]agentkit.Pricing{testModel: testPricing},
		roundTrips: roundTrips,
	}
}

func (p *fakeProvider) RoundTrip(ctx context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	p.calls = append(p.calls, cloneRequest(req))
	if p.roundTripFn != nil {
		return p.roundTripFn(ctx, req)
	}
	if len(p.roundTrips) == 0 {
		return textRoundTrip("ok")
	}
	rt := p.roundTrips[0]
	p.roundTrips = p.roundTrips[1:]
	return rt
}

func (p *fakeProvider) Name() string {
	return p.name
}

func (p *fakeProvider) Pricing(model string) (agentkit.Pricing, bool) {
	pricing, ok := p.models[model]
	return pricing, ok
}

func TestSendBoundaryValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("missing config", func(t *testing.T) {
		// R-ZWV0-CY54
		stream := (&agentkit.Conversation{Model: testModel}).Send(ctx, "hello")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrInvalidConfig) {
			t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
		}

		provider := newFakeProvider()
		stream = (&agentkit.Conversation{Provider: provider}).Send(ctx, "hello")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrInvalidConfig) {
			t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
		}
		if len(provider.calls) != 0 {
			t.Fatalf("provider calls = %d, want 0", len(provider.calls))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		// R-ZELD-OQNG
		provider := newFakeProvider()
		history := []agentkit.Message{{Role: agentkit.RoleAssistant, Blocks: []agentkit.Block{agentkit.TextBlock{Text: "prior"}}}}
		conv := &agentkit.Conversation{Provider: provider, Model: testModel, History: history}

		stream := conv.Send(ctx, "")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrInvalidInput) {
			t.Fatalf("Err() = %v, want ErrInvalidInput", stream.Err())
		}
		if !reflect.DeepEqual(conv.History, history) {
			t.Fatalf("History changed on invalid input: %#v", conv.History)
		}
		if len(provider.calls) != 0 {
			t.Fatalf("provider calls = %d, want 0", len(provider.calls))
		}
	})
}

func TestSendRejectsInvalidModelAndToolSetup(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown model", func(t *testing.T) {
		// R-7GGH-BPYN
		provider := newFakeProvider()
		conv := &agentkit.Conversation{Provider: provider, Model: "unknown-model"}
		stream := conv.Send(ctx, "hello")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrInvalidConfig) {
			t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
		}
		if len(provider.calls) != 0 {
			t.Fatalf("provider calls = %d, want 0", len(provider.calls))
		}
	})

	t.Run("invalid raw schema", func(t *testing.T) {
		// R-SX1B-XRK2
		provider := newFakeProvider()
		conv := &agentkit.Conversation{
			Provider: provider,
			Model:    testModel,
			Tools: []agentkit.Tool{
				agentkit.RawTool("bad", "bad schema", json.RawMessage(`{`), func(context.Context, json.RawMessage) (string, error) {
					return "", nil
				}),
			},
		}
		stream := conv.Send(ctx, "hello")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrInvalidConfig) {
			t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
		}
		if len(provider.calls) != 0 {
			t.Fatalf("provider calls = %d, want 0", len(provider.calls))
		}

		conv.Tools = []agentkit.Tool{agentkit.RawTool("good", "valid schema", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
			return "ok", nil
		})}
		stream = conv.Send(ctx, "hello")
		drain(stream)
		if err := stream.Err(); err != nil {
			t.Fatalf("valid RawTool Send Err() = %v, want nil", err)
		}
	})

	t.Run("duplicate tool names", func(t *testing.T) {
		// R-SZH4-PB1G
		provider := newFakeProvider()
		schema := json.RawMessage(`{"type":"object"}`)
		conv := &agentkit.Conversation{
			Provider: provider,
			Model:    testModel,
			Tools: []agentkit.Tool{
				agentkit.RawTool("same", "first", schema, func(context.Context, json.RawMessage) (string, error) { return "", nil }),
				agentkit.RawTool("same", "second", schema, func(context.Context, json.RawMessage) (string, error) { return "", nil }),
			},
		}
		stream := conv.Send(ctx, "hello")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrInvalidConfig) {
			t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
		}
		if len(conv.History) != 0 {
			t.Fatalf("History len = %d, want 0", len(conv.History))
		}
		if len(provider.calls) != 0 {
			t.Fatalf("provider calls = %d, want 0", len(provider.calls))
		}
	})
}

func TestTextOnlyTurnStreamsAndCommitsHistory(t *testing.T) {
	usage := agentkit.Usage{InputUncached: 3, Output: 2, Total: 5}
	provider := newFakeProvider(newRoundTrip(
		assistant(agentkit.TextBlock{Text: "hello"}),
		agentkit.FinishStop,
		usage,
		nil,
	))
	conv := &agentkit.Conversation{Provider: provider, Model: testModel}

	stream := conv.Send(context.Background(), "hi")
	events := drain(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	// R-HUZX-7N2W, R-C7MI-HRFI
	if len(events) != 1 {
		t.Fatalf("events = %#v, want exactly one MessageDone", events)
	}
	done := onlyMessageDone(t, events)
	if got := messageText(done.Message); got != "hello" {
		t.Fatalf("MessageDone text = %q, want assembled final text", got)
	}
	if stream.Usage() != usage {
		t.Fatalf("Usage() = %#v, want %#v", stream.Usage(), usage)
	}

	// R-ZZAT-4HMI
	if len(conv.History) != 2 {
		t.Fatalf("History len = %d, want user+assistant", len(conv.History))
	}
	if conv.History[0].Role != agentkit.RoleUser || conv.History[1].Role != agentkit.RoleAssistant {
		t.Fatalf("History roles = %v, %v; want user, assistant", conv.History[0].Role, conv.History[1].Role)
	}

	// R-CBA7-N2NL
	if !reflect.DeepEqual(done.Message, conv.History[1]) {
		t.Fatalf("MessageDone message = %#v, want History assistant %#v", done.Message, conv.History[1])
	}

	// R-VV9Y-GMKH
	if countMessageDone(events) != 1 {
		t.Fatalf("MessageDone count = %d, want 1", countMessageDone(events))
	}
	if len(provider.calls) != 1 {
		t.Fatalf("round-trip calls = %d, want 1", len(provider.calls))
	}
}

func TestProviderSwitchPreservesHistoryAndUsesNewBackend(t *testing.T) {
	first := newFakeProvider(textRoundTrip("first"))
	second := newFakeProvider(textRoundTrip("second"))
	first.models = map[string]agentkit.Pricing{testModel: testPricing}
	second.models = map[string]agentkit.Pricing{secondModel: testPricing}

	conv := &agentkit.Conversation{Provider: first, Model: testModel}
	drain(conv.Send(context.Background(), "one"))
	conv.Provider = second
	conv.Model = secondModel
	drain(conv.Send(context.Background(), "two"))

	// R-00IP-I9D7
	if len(first.calls) != 1 || first.calls[0].Model != testModel {
		t.Fatalf("first provider calls/model = %d/%q, want 1/%q", len(first.calls), first.calls[0].Model, testModel)
	}
	if len(second.calls) != 1 || second.calls[0].Model != secondModel {
		t.Fatalf("second provider calls/model = %d/%q, want 1/%q", len(second.calls), second.calls[0].Model, secondModel)
	}
	if len(second.calls[0].Messages) != 3 {
		t.Fatalf("second request history len = %d, want prior turn plus new user", len(second.calls[0].Messages))
	}
	if len(conv.History) != 4 {
		t.Fatalf("conversation history len = %d, want two complete turns", len(conv.History))
	}
}

func TestToolLoopRunsToolsAndContinuesToFinalMessage(t *testing.T) {
	tool := agentkit.NewTool("lookup", "look up a city", func(_ context.Context, in struct {
		City string `json:"city"`
	}) (string, error) {
		if in.City != "Tokyo" {
			t.Fatalf("decoded City = %q, want Tokyo", in.City)
		}
		return "sunny", nil
	})
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: testToolUseID, Name: "lookup", Input: json.RawMessage(`{"city":"Tokyo"}`)}), agentkit.FinishToolUse, agentkit.Usage{InputUncached: 1, Total: 1}, nil),
		textRoundTrip("done"),
	)
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{tool}}
	events := drain(conv.Send(context.Background(), "weather"))

	// R-C8UE-VJ67
	useIndex, resultIndex := eventIndexes[agentkit.ToolUse](events), eventIndexes[agentkit.ToolResult](events)
	if useIndex < 0 || resultIndex < 0 || useIndex > resultIndex {
		t.Fatalf("ToolUse/ToolResult indexes = %d/%d, want ToolUse before ToolResult", useIndex, resultIndex)
	}
	use := events[useIndex].(agentkit.ToolUse)
	if string(use.Input) != `{"city":"Tokyo"}` {
		t.Fatalf("ToolUse.Input = %s, want complete JSON object", use.Input)
	}

	// R-VWHU-UEB6
	result := events[resultIndex].(agentkit.ToolResult)
	if result.ID != testToolUseID || result.Name != "lookup" || result.Output != "sunny" || result.IsError {
		t.Fatalf("ToolResult = %#v, want successful lookup result", result)
	}
	if len(conv.History) != 4 {
		t.Fatalf("History len = %d, want user, assistant(tool_use), user(tool_result), assistant(final)", len(conv.History))
	}
	resultBlock := conv.History[2].Blocks[0].(agentkit.ToolResultBlock)
	if resultBlock.ToolUseID != testToolUseID || resultBlock.Content != "sunny" || resultBlock.IsError {
		t.Fatalf("History tool result = %#v, want successful result", resultBlock)
	}

	// R-02PH-VYKB
	if len(provider.calls) != 2 {
		t.Fatalf("round-trip calls = %d, want continuation after tool use and stop after final message", len(provider.calls))
	}
}

func TestUnknownToolAndToolErrorAreFedBackInBand(t *testing.T) {
	t.Run("unknown tool", func(t *testing.T) {
		provider := newFakeProvider(
			newRoundTrip(assistant(agentkit.ToolUseBlock{ID: testToolUseID, Name: "missing", Input: json.RawMessage(`{}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
			textRoundTrip("recovered"),
		)
		events := drain((&agentkit.Conversation{Provider: provider, Model: testModel}).Send(context.Background(), "call it"))

		// R-VYXN-LXSK
		result := firstEvent[agentkit.ToolResult](t, events)
		if !result.IsError || result.Name != "missing" {
			t.Fatalf("ToolResult = %#v, want in-band unknown-tool error", result)
		}
		if len(provider.calls) != 2 {
			t.Fatalf("round-trip calls = %d, want turn continuation", len(provider.calls))
		}
	})

	t.Run("tool function error", func(t *testing.T) {
		tool := agentkit.NewTool("fail", "fail", func(context.Context, struct{}) (string, error) {
			return "", errors.New("tool failed")
		})
		provider := newFakeProvider(
			newRoundTrip(assistant(agentkit.ToolUseBlock{ID: secondToolUseID, Name: "fail", Input: json.RawMessage(`{}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
			textRoundTrip("recovered"),
		)
		events := drain((&agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{tool}}).Send(context.Background(), "call it"))

		// R-X1FI-EMCP
		result := firstEvent[agentkit.ToolResult](t, events)
		if !result.IsError || result.Output != "tool failed" {
			t.Fatalf("ToolResult = %#v, want in-band tool error", result)
		}
		if len(provider.calls) != 2 {
			t.Fatalf("round-trip calls = %d, want turn continuation", len(provider.calls))
		}
	})
}

func TestToolsAreSortedDeterministicallyAcrossTurns(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	a := agentkit.RawTool("a_tool", "a", schema, func(context.Context, json.RawMessage) (string, error) { return "a", nil })
	b := agentkit.RawTool("b_tool", "b", schema, func(context.Context, json.RawMessage) (string, error) { return "b", nil })
	provider := newFakeProvider(textRoundTrip("one"), textRoundTrip("two"))
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{b, a}}
	drain(conv.Send(context.Background(), "one"))
	drain(conv.Send(context.Background(), "two"))

	// R-VXPR-861V
	if len(provider.calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.calls))
	}
	for i, call := range provider.calls {
		names := []string{call.Tools[0].Name(), call.Tools[1].Name()}
		if !reflect.DeepEqual(names, []string{"a_tool", "b_tool"}) {
			t.Fatalf("call %d tool order = %v, want name-sorted", i, names)
		}
		if string(call.Tools[0].JSONSchema()) != string(schema) || string(call.Tools[1].JSONSchema()) != string(schema) {
			t.Fatalf("call %d schemas are not byte-stable deterministic JSON", i)
		}
	}
}

func TestReasoningBlockIsReplayedOnToolLoopRequest(t *testing.T) {
	tool := agentkit.RawTool("ok", "ok", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
		return "ok", nil
	})
	reasoning := agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"signature":"opaque"}`), Summary: "summary"}
	provider := newFakeProvider(
		newRoundTrip(assistant(reasoning, agentkit.ToolUseBlock{ID: testToolUseID, Name: "ok", Input: json.RawMessage(`{}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("done"),
	)
	drain((&agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{tool}}).Send(context.Background(), "loop"))

	// R-W1DG-DH9Y
	if len(provider.calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.calls))
	}
	if !messagesContainReasoning(provider.calls[1].Messages, reasoning) {
		t.Fatalf("second request history did not replay reasoning block %#v", reasoning)
	}
}

func TestContentFilterFinishIsMappedToSentinel(t *testing.T) {
	provider := newFakeProvider(newRoundTrip(assistant(), agentkit.FinishContentFilter, agentkit.Usage{}, nil))
	conv := &agentkit.Conversation{Provider: provider, Model: testModel}
	stream := conv.Send(context.Background(), "blocked")
	drain(stream)

	// R-03XE-9QB0
	if !errors.Is(stream.Err(), agentkit.ErrContentFilter) {
		t.Fatalf("Err() = %v, want ErrContentFilter", stream.Err())
	}
}

func TestFailedTurnsSurfaceErrAndRollback(t *testing.T) {
	boom := errors.New("boom")
	provider := newFakeProvider(newRoundTrip(assistant(agentkit.TextBlock{Text: "partial"}), agentkit.FinishOther, agentkit.Usage{}, boom))
	conv := &agentkit.Conversation{Provider: provider, Model: testModel}
	stream := conv.Send(context.Background(), "fail")
	events := drain(stream)

	// R-CDQ0-EM4Z
	if len(events) != 0 {
		t.Fatalf("events before failed Err() = %#v, want none", events)
	}
	if !errors.Is(stream.Err(), boom) {
		t.Fatalf("Err() = %v, want provider error", stream.Err())
	}

	// R-Y4JJ-1J5G
	if len(conv.History) != 0 {
		t.Fatalf("History len after failed turn = %d, want rollback to pre-Send state", len(conv.History))
	}
}

func TestMaxToolIterationsStopsRunawayLoopAndRollsBack(t *testing.T) {
	provider := newFakeProvider()
	provider.roundTripFn = func(context.Context, *agentkit.Request) *agentkit.RoundTrip {
		return newRoundTrip(assistant(agentkit.ToolUseBlock{ID: testToolUseID, Name: "missing", Input: json.RawMessage(`{}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil)
	}
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, MaxToolIterations: 1}
	stream := conv.Send(context.Background(), "loop")
	drain(stream)

	// R-W05J-ZPJ9
	if !errors.Is(stream.Err(), agentkit.ErrToolLoopLimit) {
		t.Fatalf("Err() = %v, want ErrToolLoopLimit", stream.Err())
	}
	if len(provider.calls) != 2 {
		t.Fatalf("round-trip calls = %d, want configured one tool iteration then failure", len(provider.calls))
	}

	// R-Y4JJ-1J5G
	if len(conv.History) != 0 {
		t.Fatalf("History len after loop limit = %d, want rollback", len(conv.History))
	}
}

func TestStreamPendingAndEarlyBreakCleanup(t *testing.T) {
	t.Run("pending stream blocks reentrant send", func(t *testing.T) {
		provider := newFakeProvider(textRoundTrip("eventual"))
		conv := &agentkit.Conversation{Provider: provider, Model: testModel}
		_ = conv.Send(context.Background(), "first")
		second := conv.Send(context.Background(), "second")
		drain(second)

		// R-XZNX-IG6O
		if !errors.Is(second.Err(), agentkit.ErrStreamPending) {
			t.Fatalf("second Err() = %v, want ErrStreamPending", second.Err())
		}
		if len(provider.calls) != 0 {
			t.Fatalf("provider calls before first drain = %d, want 0", len(provider.calls))
		}
		if len(conv.History) != 0 {
			t.Fatalf("History len = %d, want unchanged", len(conv.History))
		}
	})

	t.Run("early break releases resources and rolls back", func(t *testing.T) {
		rt := newRoundTrip(assistant(agentkit.TextBlock{Text: "firstsecond"}), agentkit.FinishToolUse, agentkit.Usage{}, nil)
		provider := newFakeProvider(rt, textRoundTrip("next"))
		conv := &agentkit.Conversation{Provider: provider, Model: testModel}
		stream := conv.Send(context.Background(), "first")
		for range stream.Events() {
			break
		}

		// R-CCI4-0UEA
		next := conv.Send(context.Background(), "next")
		drain(next)
		if err := next.Err(); err != nil {
			t.Fatalf("next Err() = %v, want nil after early-break cleanup", err)
		}

		// R-Y4JJ-1J5G
		if len(conv.History) != 2 {
			t.Fatalf("History len = %d, want only the successful next turn committed", len(conv.History))
		}
	})
}

func drain(stream *agentkit.Stream) []agentkit.Event {
	var events []agentkit.Event
	for ev := range stream.Events() {
		events = append(events, ev)
	}
	return events
}

func newRoundTrip(message agentkit.Message, finish agentkit.FinishReason, usage agentkit.Usage, err error) *agentkit.RoundTrip {
	return agentkit.NewRoundTrip(message, finish, usage, nil, err)
}

func textRoundTrip(text string) *agentkit.RoundTrip {
	return newRoundTrip(
		assistant(agentkit.TextBlock{Text: text}),
		agentkit.FinishStop,
		agentkit.Usage{InputUncached: 1, Output: 1, Total: 2},
		nil,
	)
}

func assistant(blocks ...agentkit.Block) agentkit.Message {
	return agentkit.Message{Role: agentkit.RoleAssistant, Blocks: blocks}
}

func messageText(message agentkit.Message) string {
	var text string
	for _, block := range message.Blocks {
		if block, ok := block.(agentkit.TextBlock); ok {
			text += block.Text
		}
	}
	return text
}

func onlyMessageDone(t *testing.T, events []agentkit.Event) agentkit.MessageDone {
	t.Helper()
	var dones []agentkit.MessageDone
	for _, ev := range events {
		if done, ok := ev.(agentkit.MessageDone); ok {
			dones = append(dones, done)
		}
	}
	if len(dones) != 1 {
		t.Fatalf("MessageDone count = %d, want 1", len(dones))
	}
	return dones[0]
}

func countMessageDone(events []agentkit.Event) int {
	var count int
	for _, ev := range events {
		if _, ok := ev.(agentkit.MessageDone); ok {
			count++
		}
	}
	return count
}

func eventIndexes[T agentkit.Event](events []agentkit.Event) int {
	for i, ev := range events {
		if _, ok := ev.(T); ok {
			return i
		}
	}
	return -1
}

func firstEvent[T agentkit.Event](t *testing.T, events []agentkit.Event) T {
	t.Helper()
	for _, ev := range events {
		if typed, ok := ev.(T); ok {
			return typed
		}
	}
	var zero T
	t.Fatalf("event %T not found in %v", zero, events)
	return zero
}

func messagesContainReasoning(messages []agentkit.Message, want agentkit.ReasoningBlock) bool {
	for _, message := range messages {
		for _, block := range message.Blocks {
			reasoning, ok := block.(agentkit.ReasoningBlock)
			if ok && string(reasoning.Opaque) == string(want.Opaque) && reasoning.Summary == want.Summary {
				return true
			}
		}
	}
	return false
}

func cloneRequest(req *agentkit.Request) agentkit.Request {
	if req == nil {
		return agentkit.Request{}
	}
	cloned := *req
	cloned.Messages = cloneMessages(req.Messages)
	cloned.Tools = append([]agentkit.Tool(nil), req.Tools...)
	return cloned
}

func cloneMessages(messages []agentkit.Message) []agentkit.Message {
	cloned := make([]agentkit.Message, len(messages))
	for i, message := range messages {
		cloned[i] = agentkit.Message{
			Role:   message.Role,
			Blocks: append([]agentkit.Block(nil), message.Blocks...),
		}
	}
	return cloned
}

func TestNewRoundTripAccessorsDefensivelyCopy(t *testing.T) {
	warnings := []agentkit.Warning{{Setting: "reasoning", Detail: "degraded"}}
	raw := json.RawMessage(`{"q":"x"}`)
	rt := agentkit.NewRoundTrip(assistant(agentkit.ToolUseBlock{ID: testToolUseID, Name: "lookup", Input: raw}), agentkit.FinishStop, agentkit.Usage{Total: 1}, warnings, nil)
	warnings[0].Detail = "mutated"
	raw[0] = ' '

	if got := rt.Warnings()[0].Detail; got != "degraded" {
		t.Fatalf("Warnings()[0].Detail = %q, want defensive copy", got)
	}
	msg := rt.Message()
	use := msg.Blocks[0].(agentkit.ToolUseBlock)
	if string(use.Input) != `{"q":"x"}` {
		t.Fatalf("Message ToolUse input = %s, want defensive copy", use.Input)
	}
	if rt.Finish() != agentkit.FinishStop || rt.Usage().Total != 1 || rt.Err() != nil {
		t.Fatalf("RoundTrip accessors returned inconsistent values")
	}
}

func ExampleConversation_Send() {
	provider := newFakeProvider(textRoundTrip("hello"))
	conv := &agentkit.Conversation{Provider: provider, Model: testModel}
	stream := conv.Send(context.Background(), "hi")
	for ev := range stream.Events() {
		if done, ok := ev.(agentkit.MessageDone); ok {
			fmt.Print(messageText(done.Message))
		}
	}
	// Output: hello
}
