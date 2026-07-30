package agentkit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
)

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

type schemaTranslatorProvider struct {
	fakeProvider
	schemas []json.RawMessage
}

func newSchemaTranslatorProvider() *schemaTranslatorProvider {
	provider := &schemaTranslatorProvider{}
	provider.name = "schema-translator"
	return provider
}

func (p *schemaTranslatorProvider) UntranslatableSchemaConstructs(schema json.RawMessage) []string {
	p.schemas = append(p.schemas, append(json.RawMessage(nil), schema...))
	if strings.Contains(string(schema), "additionalProperties") {
		return []string{"additionalProperties"}
	}
	return nil
}

func TestCustomToolSchemaWarningAtSendBoundary(t *testing.T) {
	provider := newSchemaTranslatorProvider()
	tool := testRawTool("custom_lossy", "custom", json.RawMessage(`{
		"type":"object",
		"additionalProperties":false
	}`), func(context.Context, json.RawMessage) (string, error) { return "ok", nil })
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Pricing: &agentkit.Pricing{}, Tools: []agentkit.Tool{tool}}

	stream := conv.Send(context.Background(), "hello")
	drain(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	warnings := stream.Warnings()
	// R-9TC8-5QCW
	if len(warnings) != 1 || warnings[0].Setting != "tool_schema" || warnings[0].Code != agentkit.WarnToolSchemaLossy {
		t.Fatalf("Warnings() = %#v, want one lossy tool_schema warning", warnings)
	}
	if detail := warnings[0].Detail; !strings.Contains(detail, "custom_lossy") || !strings.Contains(detail, "additionalProperties") {
		t.Fatalf("warning detail %q does not attribute custom tool and construct", detail)
	}
	if len(provider.schemas) != 1 || string(provider.schemas[0]) != string(tool.JSONSchema()) {
		t.Fatalf("translator schemas = %#v, want custom tool schema", provider.schemas)
	}
}

func TestDeferredToolSchemaWarningAppearsAfterLoad(t *testing.T) {
	provider := newSchemaTranslatorProvider()
	provider.roundTrips = []*agentkit.RoundTrip{
		newRoundTrip(assistant(agentkit.ToolUseBlock{ID: testToolUseID, Name: "load_tools", Input: json.RawMessage(`{"tools":["deferred_lossy"]}`)}), agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("loaded"),
		textRoundTrip("next"),
	}
	tool := testRawTool("deferred_lossy", "custom", json.RawMessage(`{
		"type":"object",
		"additionalProperties":false
	}`), func(context.Context, json.RawMessage) (string, error) { return "ok", nil })
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "schemas",
			Blurb: "Schema-heavy tools",
			Tools: []agentkit.Tool{tool},
		}},
	}

	first := conv.Send(context.Background(), "load")
	drain(first)
	if err := first.Err(); err != nil {
		t.Fatalf("first Err() = %v, want nil", err)
	}
	second := conv.Send(context.Background(), "next")
	drain(second)
	if err := second.Err(); err != nil {
		t.Fatalf("second Err() = %v, want nil", err)
	}

	var sawDeferred bool
	for _, warning := range second.Warnings() {
		if strings.Contains(warning.Detail, "deferred_lossy") && warning.Code == agentkit.WarnToolSchemaLossy {
			sawDeferred = true
		}
	}
	if !sawDeferred {
		t.Fatalf("second Warnings() = %#v, want loaded deferred tool schema warning", second.Warnings())
	}
}
