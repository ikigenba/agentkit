package agentkit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
)

type schemaTranslatorProvider struct {
	fakeProvider
	schemas []json.RawMessage
}

func newSchemaTranslatorProvider() *schemaTranslatorProvider {
	provider := &schemaTranslatorProvider{}
	provider.name = "schema-translator"
	provider.models = map[string]agentkit.Pricing{testModel: testPricing}
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
	tool := agentkit.RawTool("custom_lossy", "custom", json.RawMessage(`{
		"type":"object",
		"additionalProperties":false
	}`), func(context.Context, json.RawMessage) (string, error) { return "ok", nil })
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{tool}}

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
