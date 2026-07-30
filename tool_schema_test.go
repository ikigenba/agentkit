package agentkit_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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

func TestToolSchemaSubsetRejectsUnsupportedConstructsAtSendBoundary(t *testing.T) {
	tests := []struct {
		name      string
		construct string
		location  string
		schema    string
	}{
		{"minimum", "minimum", "#/properties/value/minimum", `{"type":"object","properties":{"value":{"type":"number","minimum":0}}}`},
		{"not", "not", "#/properties/value/not", `{"type":"object","properties":{"value":{"not":{"type":"string"}}}}`},
		{"allOf", "allOf", "#/properties/value/allOf", `{"type":"object","properties":{"value":{"allOf":[{"type":"string"}]}}}`},
		{"patternProperties", "patternProperties", "#/patternProperties", `{"type":"object","patternProperties":{"x":{"type":"string"}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// R-XLTO-JMIA
			assertSchemaRejected(t, "fake", json.RawMessage(test.schema), test.construct, test.location)
		})
	}

	provider := newFakeProvider()
	valid := json.RawMessage(`{
		"type":"object",
		"title":"canonical",
		"description":"all supported forms",
		"properties":{
			"name":{"type":"string","minLength":1,"maxLength":20,"pattern":"^[a-z]+$","enum":["a","b"],"const":"a","default":"a"},
			"when":{"type":"string","format":"date-time"},
			"values":{"type":"array","items":{"type":"integer"},"minItems":1},
			"choice":{"oneOf":[{"type":"string"},{"type":"null"}]},
			"shared":{"$ref":"#/$defs/shared"}
		},
		"required":["name"],
		"$defs":{"shared":{"type":"boolean"}}
	}`)
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Pricing: &agentkit.Pricing{}, Tools: []agentkit.Tool{schemaTestTool(valid)}}
	stream := conv.Send(context.Background(), "hello")
	drain(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("canonical subset Send Err() = %v, want nil", err)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.calls))
	}
}

func TestToolSchemaSubsetIsProviderIndependent(t *testing.T) {
	// R-XRX6-GH7R
	for _, providerName := range []string{"google", "anthropic", "openai", "openrouter", "zai"} {
		t.Run(providerName, func(t *testing.T) {
			assertSchemaRejected(t, providerName,
				json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer","minimum":1}}}`),
				"minimum", "#/properties/count/minimum")
		})
	}
}

func TestToolSchemaSubsetRejectsAuthoredAdditionalProperties(t *testing.T) {
	// R-XN1K-XE8Z
	assertSchemaRejected(t, "fake",
		json.RawMessage(`{"type":"object","additionalProperties":{"type":"string"}}`),
		"additionalProperties", "#/additionalProperties")
}

func TestToolSchemaSubsetRejectsRecursiveReference(t *testing.T) {
	// R-XO9H-B5ZO
	assertSchemaRejected(t, "fake", json.RawMessage(`{
		"type":"object",
		"properties":{"node":{"$ref":"#/$defs/node"}},
		"$defs":{"node":{"type":"object","properties":{"next":{"$ref":"#/$defs/node"}}}}
	}`), "recursive $ref", "$ref")
}

func TestToolSchemaSubsetFormatAllowlist(t *testing.T) {
	for _, format := range []string{"uri", "uri-reference", "regex"} {
		t.Run("reject_"+format, func(t *testing.T) {
			// R-U3C5-4A1V
			assertSchemaRejected(t, "fake",
				json.RawMessage(`{"type":"object","properties":{"value":{"type":"string","format":"`+format+`"}}}`),
				"format", "#/properties/value/format")
		})
	}

	for _, format := range []string{"date-time", "date", "time", "duration", "email", "hostname", "ipv4", "ipv6", "uuid"} {
		t.Run("accept_"+format, func(t *testing.T) {
			provider := newFakeProvider()
			schema := json.RawMessage(`{"type":"object","properties":{"value":{"type":"string","format":"` + format + `"}}}`)
			conv := &agentkit.Conversation{Provider: provider, Model: testModel, Pricing: &agentkit.Pricing{}, Tools: []agentkit.Tool{schemaTestTool(schema)}}
			stream := conv.Send(context.Background(), "hello")
			drain(stream)
			if err := stream.Err(); err != nil {
				t.Fatalf("format %q Send Err() = %v, want nil", format, err)
			}
		})
	}
}

func TestToolSchemaSubsetRequiresStringEnumAndConst(t *testing.T) {
	tests := []struct {
		name      string
		construct string
		schema    string
	}{
		{"numeric_enum", "enum", `{"type":"object","properties":{"value":{"type":"integer","enum":[1,2]}}}`},
		{"numeric_const", "const", `{"type":"object","properties":{"value":{"type":"integer","const":5}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// R-6QNT-WR7C
			assertSchemaRejected(t, "fake", json.RawMessage(test.schema), test.construct, "#/properties/value/"+test.construct)
		})
	}

	provider := newFakeProvider()
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		Pricing:  &agentkit.Pricing{},
		Tools: []agentkit.Tool{schemaTestTool(json.RawMessage(
			`{"type":"object","properties":{"value":{"type":"string","enum":["a","b"],"const":"a"}}}`,
		))},
	}
	stream := conv.Send(context.Background(), "hello")
	drain(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("string enum/const Send Err() = %v, want nil", err)
	}
}

func TestToolSchemaSubsetRequiresObjectRoot(t *testing.T) {
	tests := []struct {
		name   string
		schema string
	}{
		{"root_anyOf", `{"anyOf":[{"type":"object"},{"type":"object"}]}`},
		{"root_string", `{"type":"string"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// R-ZPPN-6FV9
			assertSchemaRejected(t, "fake", json.RawMessage(test.schema), "root shape", "#")
		})
	}
}

func TestToolSchemaWarningsAreRemovedWithoutAffectingOtherWarnings(t *testing.T) {
	// R-XWSR-ZK6J
	provider := newFakeProvider(agentkit.NewRoundTrip(
		assistant(agentkit.TextBlock{Text: "ok"}),
		agentkit.FinishStop,
		agentkit.Usage{},
		[]agentkit.Warning{{Setting: "tool_choice", Code: agentkit.WarnToolChoiceForced, Detail: "forced"}},
		nil,
		0,
		false,
	))
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{schemaTestTool(json.RawMessage(`{"type":"object"}`))}}
	stream := conv.Send(context.Background(), "hello")
	drain(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Send Err() = %v, want nil", err)
	}
	var settings []string
	for _, warning := range stream.Warnings() {
		if warning.Setting == "tool_schema" {
			t.Fatalf("Warnings() contains removed tool_schema warning: %#v", stream.Warnings())
		}
		settings = append(settings, warning.Setting)
	}
	if !reflect.DeepEqual(settings, []string{"tool_choice", "cost"}) {
		t.Fatalf("warning settings = %v, want unrelated tool_choice and cost warnings", settings)
	}
}

func assertSchemaRejected(t *testing.T, providerName string, schema json.RawMessage, construct, location string) {
	t.Helper()
	provider := newFakeProvider()
	provider.name = providerName
	history := []agentkit.Message{{Role: agentkit.RoleAssistant, Blocks: []agentkit.Block{agentkit.TextBlock{Text: "prior"}}}}
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    testModel,
		History:  append([]agentkit.Message(nil), history...),
		Tools:    []agentkit.Tool{schemaTestTool(schema)},
	}
	stream := conv.Send(context.Background(), "hello")
	drain(stream)
	if !errors.Is(stream.Err(), agentkit.ErrInvalidConfig) {
		t.Fatalf("Err() = %v, want ErrInvalidConfig", stream.Err())
	}
	if text := stream.Err().Error(); !strings.Contains(text, construct) || !strings.Contains(text, location) {
		t.Fatalf("Err() = %q, want construct %q and location %q", text, construct, location)
	}
	if !reflect.DeepEqual(conv.History, history) {
		t.Fatalf("History changed: %#v", conv.History)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(provider.calls))
	}
}

func schemaTestTool(schema json.RawMessage) agentkit.Tool {
	return testRawTool("schema_test", "schema test", schema, func(context.Context, json.RawMessage) (string, error) {
		return "ok", nil
	})
}
