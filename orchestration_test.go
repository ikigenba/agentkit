package agentkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/openai"
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
	roundTrips  []*agentkit.RoundTrip
	roundTripFn func(context.Context, *agentkit.Request) *agentkit.RoundTrip
	calls       []agentkit.Request
}

func newFakeProvider(roundTrips ...*agentkit.RoundTrip) *fakeProvider {
	return &fakeProvider{
		name:       "fake",
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

func TestSendAcceptsFreeFlowModelAndRejectsInvalidToolSetup(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown model", func(t *testing.T) {
		provider := newFakeProvider()
		conv := &agentkit.Conversation{Provider: provider, Model: "unknown-model"}
		stream := conv.Send(ctx, "hello")
		drain(stream)
		if stream.Err() != nil {
			t.Fatalf("Err() = %v, want nil", stream.Err())
		}
		if len(provider.calls) != 1 || provider.calls[0].Model != "unknown-model" {
			t.Fatalf("provider calls/model = %d/%q, want 1/unknown-model", len(provider.calls), provider.calls[0].Model)
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

func TestReportedCostBeatsSuppliedPricingAndAccumulates(t *testing.T) {
	// R-CZ7D-VSL4
	usage := agentkit.Usage{InputUncached: 2, Output: 3, Total: 5}
	provider := newFakeProvider(
		agentkit.NewRoundTrip(assistant(agentkit.TextBlock{Text: "first"}), agentkit.FinishStop, usage, nil, nil, 17, true),
		agentkit.NewRoundTrip(assistant(agentkit.TextBlock{Text: "second"}), agentkit.FinishStop, usage, nil, nil, 0, true),
	)
	pricing := agentkit.Pricing{Tiers: []agentkit.RateTier{{InputUncached: 100, Output: 200}}}
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Pricing: &pricing}

	first := conv.Send(context.Background(), "one")
	drain(first)
	if first.Err() != nil || first.Cost() != 17 {
		t.Fatalf("first Err()/Cost() = %v/%d, want nil/17", first.Err(), first.Cost())
	}
	second := conv.Send(context.Background(), "two")
	drain(second)
	if second.Err() != nil || second.Cost() != 0 {
		t.Fatalf("second Err()/Cost() = %v/%d, want nil/0 for present reported zero", second.Err(), second.Cost())
	}
	if got := conv.TotalCost(); got != 17 {
		t.Fatalf("TotalCost() = %d, want 17", got)
	}
	if len(first.Warnings()) != 0 || len(second.Warnings()) != 0 {
		t.Fatalf("reported costs produced warnings: first=%#v second=%#v", first.Warnings(), second.Warnings())
	}
}

func TestSuppliedPricingRatesRoundTripUsageAtSelectedTier(t *testing.T) {
	// R-D0FA-9KBT
	usage := agentkit.Usage{
		InputUncached:   61,
		CacheReadInput:  20,
		CacheWriteInput: 20,
		CacheWrite5m:    7,
		CacheWrite1h:    13,
		Output:          3,
		ReasoningOutput: 5,
		Total:           109,
	}
	pricing := agentkit.Pricing{Tiers: []agentkit.RateTier{
		{MinInputTokens: 0, InputUncached: 1, CacheReadInput: 2, CacheWrite5m: 3, CacheWrite1h: 4, Output: 5},
		{MinInputTokens: 101, InputUncached: 11, CacheReadInput: 13, CacheWrite5m: 17, CacheWrite1h: 19, Output: 23},
	}}
	provider := newFakeProvider(agentkit.NewRoundTrip(
		assistant(agentkit.TextBlock{Text: "priced"}), agentkit.FinishStop, usage, nil, nil, 0, false,
	))
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Pricing: &pricing}

	stream := conv.Send(context.Background(), "hello")
	drain(stream)
	want := agentkit.Cost(61*11 + 20*13 + 7*17 + 13*19 + (3+5)*23)
	if stream.Err() != nil || stream.Cost() != want {
		t.Fatalf("Err()/Cost() = %v/%d, want nil/%d", stream.Err(), stream.Cost(), want)
	}
	if conv.TotalCost() != want {
		t.Fatalf("TotalCost() = %d, want %d", conv.TotalCost(), want)
	}
	if len(stream.Warnings()) != 0 {
		t.Fatalf("Warnings() = %#v, want none", stream.Warnings())
	}
}

func TestUnknownCostWarnsEveryRoundTripUntilPricingSupplied(t *testing.T) {
	// R-D1N6-NC2I
	provider := newFakeProvider(textRoundTrip("one"), textRoundTrip("two"), textRoundTrip("three"))
	var logs bytes.Buffer
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Log: &logs}

	for _, prompt := range []string{"one", "two"} {
		stream := conv.Send(context.Background(), prompt)
		drain(stream)
		if stream.Err() != nil || stream.Cost() != 0 {
			t.Fatalf("%s Err()/Cost() = %v/%d, want nil/0", prompt, stream.Err(), stream.Cost())
		}
		warnings := stream.Warnings()
		if len(warnings) != 1 || warnings[0].Setting != "cost" || warnings[0].Code != agentkit.WarnCostUnknown {
			t.Fatalf("%s Warnings() = %#v, want exactly one cost/WarnCostUnknown", prompt, warnings)
		}
	}
	if got := countLoggedWarnings(t, logs.Bytes(), agentkit.WarnCostUnknown); got != 2 {
		t.Fatalf("cost warning log records = %d, want one for each affected turn", got)
	}

	pricing := testPricing
	conv.Pricing = &pricing
	third := conv.Send(context.Background(), "three")
	drain(third)
	if third.Err() != nil || third.Cost() != testPricing.Cost(third.Usage()) {
		t.Fatalf("priced Err()/Cost() = %v/%d, want nil/%d", third.Err(), third.Cost(), testPricing.Cost(third.Usage()))
	}
	if len(third.Warnings()) != 0 {
		t.Fatalf("priced Warnings() = %#v, want none", third.Warnings())
	}
	if got := countLoggedWarnings(t, logs.Bytes(), agentkit.WarnCostUnknown); got != 2 {
		t.Fatalf("cost warning log records after pricing = %d, want still 2", got)
	}
}

func countLoggedWarnings(t *testing.T, raw []byte, code agentkit.WarningCode) int {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	count := 0
	for {
		var record struct {
			Type    string            `json:"type"`
			Warning *agentkit.Warning `json:"warning"`
		}
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode log record: %v", err)
		}
		if record.Type == "warning" && record.Warning != nil && record.Warning.Code == code {
			count++
		}
	}
	return count
}

func TestProviderSwitchPreservesHistoryAndUsesNewBackend(t *testing.T) {
	first := newFakeProvider(textRoundTrip("first"))
	second := newFakeProvider(textRoundTrip("second"))
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

func TestMessageDoneMirrorsHistoryForRichToolUseMessage(t *testing.T) {
	tool := agentkit.RawTool("lookup", "look up", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
		return "sunny", nil
	})
	reasoning := agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"signature":"opaque"}`), Summary: "looked at request"}
	toolUse := agentkit.ToolUseBlock{ID: testToolUseID, Name: "lookup", Input: json.RawMessage(`{"city":"Tokyo"}`)}
	provider := newFakeProvider(
		newRoundTrip(
			assistant(
				reasoning,
				agentkit.TextBlock{Text: "I'll check."},
				toolUse,
			),
			agentkit.FinishToolUse,
			agentkit.Usage{InputUncached: 1, Output: 1, Total: 2},
			nil,
		),
		textRoundTrip("done"),
	)
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{tool}}

	events := drain(conv.Send(context.Background(), "weather"))

	// R-CBA7-N2NL
	if len(events) == 0 {
		t.Fatalf("events = nil, want first MessageDone")
	}
	done, ok := events[0].(agentkit.MessageDone)
	if !ok {
		t.Fatalf("first event = %T, want MessageDone", events[0])
	}
	if len(conv.History) != 4 {
		t.Fatalf("History len = %d, want user, assistant(tool_use), user(tool_result), assistant(final)", len(conv.History))
	}
	if !reflect.DeepEqual(done.Message, conv.History[1]) {
		t.Fatalf("MessageDone message = %#v, want History assistant %#v", done.Message, conv.History[1])
	}

	gotReasoning, gotText, gotToolUse := false, false, false
	for _, block := range conv.History[1].Blocks {
		switch block := block.(type) {
		case agentkit.ReasoningBlock:
			gotReasoning = string(block.Opaque) == string(reasoning.Opaque) && block.Summary == reasoning.Summary
		case agentkit.TextBlock:
			gotText = block.Text == "I'll check."
		case agentkit.ToolUseBlock:
			gotToolUse = block.ID == toolUse.ID && block.Name == toolUse.Name && string(block.Input) == string(toolUse.Input)
		}
	}
	if !gotReasoning || !gotText || !gotToolUse {
		t.Fatalf("History assistant blocks include reasoning/text/tool_use = %v/%v/%v, want all true", gotReasoning, gotText, gotToolUse)
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

func TestDeferredToolsAdvertiseLoadToolsAndEmptyIsNoop(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"secret_prop":{"type":"string"}}}`)
	deferred := agentkit.RawTool("later_search", "SECRET deferred search description", schema, func(context.Context, json.RawMessage) (string, error) {
		return "later", nil
	})
	provider := newFakeProvider(textRoundTrip("ok"))
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "research",
			Blurb: "Find archived documents",
			Tools: []agentkit.Tool{deferred},
		}},
	}
	drain(conv.Send(context.Background(), "hello"))

	// R-9RQ8-9G3W
	if len(provider.calls) != 1 || len(provider.calls[0].Tools) != 1 {
		t.Fatalf("request tools = %#v, want exactly synthetic load_tools", provider.calls)
	}
	loadTools := provider.calls[0].Tools[0]
	if loadTools.Name() != "load_tools" {
		t.Fatalf("tool name = %q, want load_tools", loadTools.Name())
	}
	description := loadTools.Description()
	for _, want := range []string{"research", "Find archived documents", "later_search"} {
		if !strings.Contains(description, want) {
			t.Fatalf("load_tools description %q missing %q", description, want)
		}
	}
	for _, forbidden := range []string{"SECRET deferred search description", "secret_prop"} {
		if strings.Contains(description, forbidden) {
			t.Fatalf("load_tools description leaked deferred detail %q: %q", forbidden, description)
		}
	}

	noDeferredProvider := newFakeProvider(textRoundTrip("ok"))
	emptyDeferredProvider := newFakeProvider(textRoundTrip("ok"))
	drain((&agentkit.Conversation{Provider: noDeferredProvider, Model: testModel}).Send(context.Background(), "same"))
	drain((&agentkit.Conversation{Provider: emptyDeferredProvider, Model: testModel, DeferredTools: []agentkit.DeferredToolGroup{}}).Send(context.Background(), "same"))

	// R-9SY4-N7UL
	if !reflect.DeepEqual(noDeferredProvider.calls, emptyDeferredProvider.calls) {
		t.Fatalf("empty DeferredTools request = %#v, want identical to absent field %#v", emptyDeferredProvider.calls, noDeferredProvider.calls)
	}
}

func TestDeferredLoadToolsLoadsWithinTurnAndReturnsSchemas(t *testing.T) {
	var calledWith json.RawMessage
	deferred := agentkit.RawTool("later_lookup", "Lookup deferred facts", json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`), func(_ context.Context, input json.RawMessage) (string, error) {
		calledWith = append(json.RawMessage(nil), input...)
		return "fact", nil
	})
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_load", Name: "load_tools", Input: json.RawMessage(`{"tools":["later_lookup"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_later", Name: "later_lookup", Input: json.RawMessage(`{"q":"agentkit"}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("done"),
		textRoundTrip("next"),
	)
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "facts",
			Blurb: "Fact lookup tools",
			Tools: []agentkit.Tool{deferred},
		}},
	}
	events := drain(conv.Send(context.Background(), "load then call"))

	// R-D6XP-LUMJ
	loadResult := toolResultByName(t, events, "load_tools")
	if loadResult.IsError || !strings.Contains(loadResult.Output, "Lookup deferred facts") || !strings.Contains(loadResult.Output, `"input_schema"`) || !strings.Contains(loadResult.Output, `"q"`) {
		t.Fatalf("load_tools result = %#v, want loaded description and input schema", loadResult)
	}

	// R-D5PT-82VU
	if len(provider.calls) != 3 {
		t.Fatalf("provider calls = %d, want load, native call, final", len(provider.calls))
	}
	if !reflect.DeepEqual(requestToolNames(provider.calls[1]), []string{"load_tools", "later_lookup"}) {
		t.Fatalf("second request tools = %v, want load_tools then loaded tool", requestToolNames(provider.calls[1]))
	}
	callResult := toolResultByName(t, events, "later_lookup")
	if callResult.Output != "fact" || callResult.IsError || string(calledWith) != `{"q":"agentkit"}` {
		t.Fatalf("deferred call result/input = %#v/%s, want real tool dispatch", callResult, calledWith)
	}

	drain(conv.Send(context.Background(), "still loaded"))
	// R-D9DI-DE3X
	if !reflect.DeepEqual(requestToolNames(provider.calls[3]), []string{"load_tools", "later_lookup"}) {
		t.Fatalf("later Send request tools = %v, want loaded deferred tool retained", requestToolNames(provider.calls[3]))
	}
}

func TestDeferredLoadToolsGroupNameLoadsGroupInOrder(t *testing.T) {
	var calledWith json.RawMessage
	first := agentkit.RawTool("z_group_first", "First grouped tool", json.RawMessage(`{"type":"object","properties":{"first":{"type":"string"}}}`), func(context.Context, json.RawMessage) (string, error) {
		return "first", nil
	})
	second := agentkit.RawTool("a_group_second", "Second grouped tool", json.RawMessage(`{"type":"object","properties":{"second":{"type":"string"}}}`), func(_ context.Context, input json.RawMessage) (string, error) {
		calledWith = append(json.RawMessage(nil), input...)
		return "second", nil
	})
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_load_group", Name: "load_tools", Input: json.RawMessage(`{"tools":["grouped"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_second", Name: "a_group_second", Input: json.RawMessage(`{"second":"value"}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("done"),
	)
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "grouped",
			Blurb: "Grouped tools",
			Tools: []agentkit.Tool{first, second},
		}},
	}
	events := drain(conv.Send(context.Background(), "load group"))

	// R-B5BR-U5M1
	if len(provider.calls) != 3 {
		t.Fatalf("provider calls = %d, want load, native call, final", len(provider.calls))
	}
	description := provider.calls[0].Tools[0].Description()
	if !strings.Contains(description, "group name") || !strings.Contains(description, "loads every tool in that group") {
		t.Fatalf("load_tools description = %q, want group-name whole-group guidance", description)
	}
	payload := decodeLoadToolsResult(t, toolResultByName(t, events, "load_tools").Output)
	if got := loadResultNames(payload); !reflect.DeepEqual(got, []string{"z_group_first", "a_group_second"}) {
		t.Fatalf("loaded result names = %v, want group order", got)
	}
	if payload.Loaded[0].Description != "First grouped tool" || !strings.Contains(string(payload.Loaded[0].InputSchema), `"first"`) {
		t.Fatalf("first load result = %#v, want description and input schema", payload.Loaded[0])
	}
	if payload.Loaded[1].Description != "Second grouped tool" || !strings.Contains(string(payload.Loaded[1].InputSchema), `"second"`) {
		t.Fatalf("second load result = %#v, want description and input schema", payload.Loaded[1])
	}
	if !reflect.DeepEqual(requestToolNames(provider.calls[1]), []string{"load_tools", "z_group_first", "a_group_second"}) {
		t.Fatalf("second request tools = %v, want loaded group tools in group order", requestToolNames(provider.calls[1]))
	}
	callResult := toolResultByName(t, events, "a_group_second")
	if callResult.IsError || callResult.Output != "second" || string(calledWith) != `{"second":"value"}` {
		t.Fatalf("group tool result/input = %#v/%s, want real tool dispatch", callResult, calledWith)
	}
}

func TestDeferredLoadToolsMixedNamesReportsUnknownAndContinues(t *testing.T) {
	deferred := agentkit.RawTool("valid_deferred", "Valid deferred tool", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
		return "valid", nil
	})
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_load", Name: "load_tools", Input: json.RawMessage(`{"tools":["valid_deferred","missing_one","missing_two"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("continued"),
	)
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "mixed",
			Blurb: "Mixed load group",
			Tools: []agentkit.Tool{deferred},
		}},
	}
	events := drain(conv.Send(context.Background(), "load mixed"))

	// R-D85L-ZMD8
	result := toolResultByName(t, events, "load_tools")
	if !result.IsError || !strings.Contains(result.Output, "valid_deferred") || !strings.Contains(result.Output, "missing_one") || !strings.Contains(result.Output, "missing_two") {
		t.Fatalf("mixed load_tools result = %#v, want valid load and each unknown name", result)
	}
	if len(provider.calls) != 2 || !reflect.DeepEqual(requestToolNames(provider.calls[1]), []string{"load_tools", "valid_deferred"}) {
		t.Fatalf("provider calls/tools = %d/%v, want continued turn with valid tool loaded", len(provider.calls), requestToolNames(provider.calls[1]))
	}
}

func TestDeferredLoadToolsMixedToolGroupAndUnknown(t *testing.T) {
	solo := agentkit.RawTool("solo_deferred", "Solo deferred tool", json.RawMessage(`{"type":"object","properties":{"solo":{"type":"boolean"}}}`), func(context.Context, json.RawMessage) (string, error) {
		return "solo", nil
	})
	groupFirst := agentkit.RawTool("group_first", "First mixed group tool", json.RawMessage(`{"type":"object","properties":{"one":{"type":"string"}}}`), func(context.Context, json.RawMessage) (string, error) {
		return "first", nil
	})
	groupSecond := agentkit.RawTool("group_second", "Second mixed group tool", json.RawMessage(`{"type":"object","properties":{"two":{"type":"string"}}}`), func(context.Context, json.RawMessage) (string, error) {
		return "second", nil
	})
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_mixed_group", Name: "load_tools", Input: json.RawMessage(`{"tools":["solo_deferred","mixed_group","missing_name"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("continued"),
	)
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{
			{Name: "solo_group", Blurb: "Solo group", Tools: []agentkit.Tool{solo}},
			{Name: "mixed_group", Blurb: "Mixed group", Tools: []agentkit.Tool{groupFirst, groupSecond}},
		},
	}
	events := drain(conv.Send(context.Background(), "load mixed group"))

	// R-B6JO-7XCQ
	result := toolResultByName(t, events, "load_tools")
	if !result.IsError {
		t.Fatalf("load_tools result IsError = false, want true for unknown name")
	}
	payload := decodeLoadToolsResult(t, result.Output)
	if got := loadResultNames(payload); !reflect.DeepEqual(got, []string{"solo_deferred", "group_first", "group_second"}) {
		t.Fatalf("loaded result names = %v, want named tool then group tools", got)
	}
	if !reflect.DeepEqual(payload.Unknown, []string{"missing_name"}) {
		t.Fatalf("unknown names = %v, want only missing_name", payload.Unknown)
	}
	if len(provider.calls) != 2 || !reflect.DeepEqual(requestToolNames(provider.calls[1]), []string{"load_tools", "solo_deferred", "group_first", "group_second"}) {
		t.Fatalf("provider calls/tools = %d/%v, want turn continuation with loaded tool and group", len(provider.calls), requestToolNames(provider.calls[1]))
	}
	if got := lastMessageDoneText(t, events); got != "continued" {
		t.Fatalf("final message = %q, want continued turn", got)
	}
}

func TestDeferredLoadToolsNamePrefersToolOverGroup(t *testing.T) {
	shadowTool := agentkit.RawTool("shadow_name", "Shadow tool", json.RawMessage(`{"type":"object","properties":{"shadow":{"type":"string"}}}`), func(context.Context, json.RawMessage) (string, error) {
		return "shadow", nil
	})
	groupOnly := agentkit.RawTool("group_only", "Group-only tool", json.RawMessage(`{"type":"object","properties":{"group":{"type":"string"}}}`), func(context.Context, json.RawMessage) (string, error) {
		return "group", nil
	})
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_shadow", Name: "load_tools", Input: json.RawMessage(`{"tools":["shadow_name"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("continued"),
	)
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{
			{Name: "source_group", Blurb: "Source group", Tools: []agentkit.Tool{shadowTool}},
			{Name: "shadow_name", Blurb: "Name colliding group", Tools: []agentkit.Tool{groupOnly}},
		},
	}
	events := drain(conv.Send(context.Background(), "load shadow"))

	// R-B7RK-LP3F
	result := toolResultByName(t, events, "load_tools")
	if result.IsError {
		t.Fatalf("load_tools result = %#v, want successful tool-name load", result)
	}
	payload := decodeLoadToolsResult(t, result.Output)
	if got := loadResultNames(payload); !reflect.DeepEqual(got, []string{"shadow_name"}) {
		t.Fatalf("loaded result names = %v, want only deferred tool named shadow_name", got)
	}
	if len(provider.calls) != 2 || !reflect.DeepEqual(requestToolNames(provider.calls[1]), []string{"load_tools", "shadow_name"}) {
		t.Fatalf("second request tools = %v, want only the tool, not the same-named group", requestToolNames(provider.calls[1]))
	}
}

func TestDeferredUnloadedDirectCallLoadsAndUnknownStillInBand(t *testing.T) {
	called := false
	deferred := agentkit.RawTool("cold_lookup", "Cold lookup", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
		called = true
		return "should not run", nil
	})
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_cold", Name: "cold_lookup", Input: json.RawMessage(`{"guessed":true}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("recovered"),
	)
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "cold",
			Blurb: "Cold tools",
			Tools: []agentkit.Tool{deferred},
		}},
	}
	events := drain(conv.Send(context.Background(), "guess"))

	// R-DALE-R5UM
	result := toolResultByName(t, events, "cold_lookup")
	if !result.IsError || !strings.Contains(result.Output, "load_tools") || called {
		t.Fatalf("direct deferred result/called = %#v/%v, want load_tools error and no real call", result, called)
	}
	if !reflect.DeepEqual(requestToolNames(provider.calls[1]), []string{"load_tools", "cold_lookup"}) {
		t.Fatalf("second request tools = %v, want deferred tool loaded as side effect", requestToolNames(provider.calls[1]))
	}

	unknownProvider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_unknown", Name: "not_anywhere", Input: json.RawMessage(`{}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("still recovered"),
	)
	unknownEvents := drain((&agentkit.Conversation{Provider: unknownProvider, Model: testModel, DeferredTools: conv.DeferredTools}).Send(context.Background(), "unknown"))

	// R-DE93-WH2P
	unknownResult := toolResultByName(t, unknownEvents, "not_anywhere")
	if !unknownResult.IsError || !strings.Contains(unknownResult.Output, "unknown tool: not_anywhere") || len(unknownProvider.calls) != 2 {
		t.Fatalf("unknown result/calls = %#v/%d, want in-band unknown tool and continued turn", unknownResult, len(unknownProvider.calls))
	}
}

func TestDeferredToolOrderFreezesBaseAndAppendsLoads(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	aBase := agentkit.RawTool("a_base", "base a", schema, func(context.Context, json.RawMessage) (string, error) { return "a", nil })
	zBase := agentkit.RawTool("z_base", "base z", schema, func(context.Context, json.RawMessage) (string, error) { return "z", nil })
	alpha := agentkit.RawTool("alpha_deferred", "alpha", schema, func(context.Context, json.RawMessage) (string, error) { return "alpha", nil })
	gamma := agentkit.RawTool("gamma_deferred", "gamma", schema, func(context.Context, json.RawMessage) (string, error) { return "gamma", nil })
	provider := newFakeProvider(
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_alpha", Name: "load_tools", Input: json.RawMessage(`{"tools":["alpha_deferred"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: "toolu_gamma", Name: "load_tools", Input: json.RawMessage(`{"tools":["gamma_deferred"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("done"),
	)
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		Tools:    []agentkit.Tool{zBase, aBase},
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "ordered",
			Blurb: "Ordered loads",
			Tools: []agentkit.Tool{gamma, alpha},
		}},
	}
	drain(conv.Send(context.Background(), "order"))

	// R-DBTB-4XLB
	if len(provider.calls) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(provider.calls))
	}
	baseNames := []string{"a_base", "load_tools", "z_base"}
	if !reflect.DeepEqual(requestToolNames(provider.calls[0]), baseNames) {
		t.Fatalf("first request tools = %v, want sorted base %v", requestToolNames(provider.calls[0]), baseNames)
	}
	if !reflect.DeepEqual(requestToolNames(provider.calls[1]), []string{"a_base", "load_tools", "z_base", "alpha_deferred"}) {
		t.Fatalf("second request tools = %v, want base plus alpha", requestToolNames(provider.calls[1]))
	}
	if !reflect.DeepEqual(requestToolNames(provider.calls[2]), []string{"a_base", "load_tools", "z_base", "alpha_deferred", "gamma_deferred"}) {
		t.Fatalf("third request tools = %v, want base plus alpha then gamma", requestToolNames(provider.calls[2]))
	}
	wantPrefix := toolSerializations(t, provider.calls[0].Tools)
	for i := 1; i < len(provider.calls); i++ {
		if got := toolSerializations(t, provider.calls[i].Tools[:len(baseNames)]); !reflect.DeepEqual(got, wantPrefix) {
			t.Fatalf("call %d base serialization = %v, want frozen prefix %v", i, got, wantPrefix)
		}
	}
}

func TestDeferredToolInvalidConfigFailsAtSendBoundary(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	cases := []struct {
		name string
		conv agentkit.Conversation
	}{
		{
			name: "eager duplicates deferred",
			conv: agentkit.Conversation{
				Tools: []agentkit.Tool{agentkit.RawTool("same", "eager", schema, func(context.Context, json.RawMessage) (string, error) { return "eager", nil })},
				DeferredTools: []agentkit.DeferredToolGroup{{
					Name:  "dup",
					Blurb: "dup",
					Tools: []agentkit.Tool{agentkit.RawTool("same", "deferred", schema, func(context.Context, json.RawMessage) (string, error) { return "deferred", nil })},
				}},
			},
		},
		{
			name: "reserved load_tools",
			conv: agentkit.Conversation{
				DeferredTools: []agentkit.DeferredToolGroup{{
					Name:  "reserved",
					Blurb: "reserved",
					Tools: []agentkit.Tool{agentkit.RawTool("load_tools", "reserved", schema, func(context.Context, json.RawMessage) (string, error) { return "reserved", nil })},
				}},
			},
		},
		{
			name: "invalid deferred schema",
			conv: agentkit.Conversation{
				DeferredTools: []agentkit.DeferredToolGroup{{
					Name:  "invalid",
					Blurb: "invalid",
					Tools: []agentkit.Tool{agentkit.RawTool("bad_schema", "bad", json.RawMessage(`{`), func(context.Context, json.RawMessage) (string, error) { return "bad", nil })},
				}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := newFakeProvider(textRoundTrip("never"))
			tc.conv.Provider = provider
			tc.conv.Model = testModel
			stream := tc.conv.Send(context.Background(), "hello")
			drain(stream)

			// R-DD17-IPC0
			if !errors.Is(stream.Err(), agentkit.ErrInvalidConfig) {
				t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
			}
			if len(tc.conv.History) != 0 {
				t.Fatalf("History len = %d, want unchanged", len(tc.conv.History))
			}
			if len(provider.calls) != 0 {
				t.Fatalf("provider calls = %d, want none", len(provider.calls))
			}
		})
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
		var mu sync.Mutex
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/responses" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			mu.Lock()
			requests++
			n := requests
			mu.Unlock()

			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Connection", "close")
			switch n {
			case 1:
				fmt.Fprint(w, openAIToolUseSSE())
			case 2:
				fmt.Fprint(w, openAITextOnlySSE("next ok"))
			default:
				t.Errorf("unexpected request count: %d", n)
			}
		}))
		defer server.Close()

		transport := &trackingTransport{base: &http.Transport{DisableKeepAlives: true}}
		defer transport.closeIdleConnections()
		client := &http.Client{Transport: transport}
		conv := &agentkit.Conversation{
			Provider: openai.New("test-key", openai.WithBaseURL(server.URL), openai.WithHTTPClient(client)),
			Model:    openai.ModelGPT55,
		}
		baselineGoroutines := runtime.NumGoroutine()

		stream := conv.Send(context.Background(), "first")
		for range stream.Events() {
			break
		}

		// R-CCI4-0UEA
		if !transport.allClosed() {
			t.Fatalf("tracked response bodies were not closed after early break")
		}

		next := conv.Send(context.Background(), "next")
		drain(next)
		if err := next.Err(); err != nil {
			t.Fatalf("next Err() = %v, want nil after early-break cleanup", err)
		}
		if !transport.allClosed() {
			t.Fatalf("tracked response bodies were not closed after drained follow-up turn")
		}
		transport.closeIdleConnections()
		server.CloseClientConnections()
		waitForGoroutineBaseline(t, baselineGoroutines)

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
	return agentkit.NewRoundTrip(message, finish, usage, nil, err, 0, false)
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

func lastMessageDoneText(t *testing.T, events []agentkit.Event) string {
	t.Helper()
	var last *agentkit.Message
	for _, ev := range events {
		done, ok := ev.(agentkit.MessageDone)
		if ok {
			message := done.Message
			last = &message
		}
	}
	if last == nil {
		t.Fatalf("MessageDone not found in %v", events)
	}
	return messageText(*last)
}

type trackingTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	body []*trackedBody
}

func (t *trackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body := &trackedBody{ReadCloser: resp.Body}
	resp.Body = body
	t.mu.Lock()
	t.body = append(t.body, body)
	t.mu.Unlock()
	return resp, nil
}

func (t *trackingTransport) allClosed() bool {
	t.mu.Lock()
	bodies := append([]*trackedBody(nil), t.body...)
	t.mu.Unlock()
	if len(bodies) == 0 {
		return false
	}
	for _, body := range bodies {
		if !body.isClosed() {
			return false
		}
	}
	return true
}

func (t *trackingTransport) closeIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type trackedBody struct {
	io.ReadCloser
	mu     sync.Mutex
	closed bool
}

func (b *trackedBody) Close() error {
	err := b.ReadCloser.Close()
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return err
}

func (b *trackedBody) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func waitForGoroutineBaseline(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		runtime.GC()
		if got := runtime.NumGoroutine(); got <= baseline {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines = %d, want <= pre-turn baseline %d", runtime.NumGoroutine(), baseline)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func openAIToolUseSSE() string {
	return strings.Join([]string{
		openAISSEData(`{"type":"response.output_item.added","item":{"id":"fc_early","type":"function_call","call_id":"call_early","name":"lookup"}}`),
		openAISSEData(`{"type":"response.function_call_arguments.delta","item_id":"fc_early","delta":"{}"}`),
		openAISSEData(`{"type":"response.output_item.done","item":{"id":"fc_early","type":"function_call","call_id":"call_early","name":"lookup"}}`),
		openAISSEData(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`),
		"data: [DONE]\n\n",
	}, "")
}

func openAITextOnlySSE(text string) string {
	return strings.Join([]string{
		openAISSEData(fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q}`, text)),
		openAISSEData(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`),
		"data: [DONE]\n\n",
	}, "")
}

func openAISSEData(data string) string {
	return "data: " + data + "\n\n"
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

func toolResultByName(t *testing.T, events []agentkit.Event, name string) agentkit.ToolResult {
	t.Helper()
	for _, ev := range events {
		result, ok := ev.(agentkit.ToolResult)
		if ok && result.Name == name {
			return result
		}
	}
	t.Fatalf("ToolResult %q not found in %v", name, events)
	return agentkit.ToolResult{}
}

type testLoadToolsResult struct {
	Loaded []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	} `json:"loaded"`
	Unknown []string `json:"unknown,omitempty"`
}

func decodeLoadToolsResult(t *testing.T, output string) testLoadToolsResult {
	t.Helper()
	var result testLoadToolsResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode load_tools output %q: %v", output, err)
	}
	return result
}

func loadResultNames(result testLoadToolsResult) []string {
	names := make([]string, len(result.Loaded))
	for i, loaded := range result.Loaded {
		names[i] = loaded.Name
	}
	return names
}

func requestToolNames(call agentkit.Request) []string {
	names := make([]string, len(call.Tools))
	for i, tool := range call.Tools {
		names[i] = tool.Name()
	}
	return names
}

func toolSerializations(t *testing.T, tools []agentkit.Tool) []string {
	t.Helper()
	out := make([]string, len(tools))
	for i, tool := range tools {
		data, err := json.Marshal(struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Schema      json.RawMessage `json:"schema"`
		}{
			Name:        tool.Name(),
			Description: tool.Description(),
			Schema:      tool.JSONSchema(),
		})
		if err != nil {
			t.Fatalf("marshal tool signature: %v", err)
		}
		out[i] = string(data)
	}
	return out
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
	rt := agentkit.NewRoundTrip(assistant(agentkit.ToolUseBlock{ID: testToolUseID, Name: "lookup", Input: raw}), agentkit.FinishStop, agentkit.Usage{Total: 1}, warnings, nil, 7, true)
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
	if cost, ok := rt.ReportedCost(); cost != 7 || !ok {
		t.Fatalf("ReportedCost() = %d/%v, want 7/true", cost, ok)
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
