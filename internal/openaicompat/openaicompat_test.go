package openaicompat

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ikigenba/agentkit"
)

type requestTestTool struct {
	agentkit.Tool
	name        string
	description string
	schema      json.RawMessage
}

func (t requestTestTool) Name() string                { return t.name }
func (t requestTestTool) Description() string         { return t.description }
func (t requestTestTool) JSONSchema() json.RawMessage { return t.schema }
func (t requestTestTool) Call(context.Context, json.RawMessage) (string, error) {
	return "", nil
}

func TestBuildRequestToolsSortedByName(t *testing.T) {
	// R-XY0O-DBX8
	alpha := requestTestTool{name: "alpha", description: "first", schema: json.RawMessage(`{"type":"object"}`)}
	zulu := requestTestTool{name: "zulu", description: "last", schema: json.RawMessage(`{"type":"object"}`)}
	provider := New(Config{})

	buildTools := func(tools []agentkit.Tool) ([]byte, []toolDef) {
		t.Helper()
		request, _, err := provider.buildRequest(&agentkit.Request{Model: "compat-test", Tools: tools})
		if err != nil {
			t.Fatalf("buildRequest() error = %v", err)
		}
		raw, err := json.Marshal(request.Tools)
		if err != nil {
			t.Fatalf("marshal tools: %v", err)
		}
		return raw, request.Tools
	}

	forward, tools := buildTools([]agentkit.Tool{alpha, zulu})
	reversed, _ := buildTools([]agentkit.Tool{zulu, alpha})
	if string(forward) != string(reversed) {
		t.Fatalf("serialized tools differ by input order:\nforward: %s\nreverse: %s", forward, reversed)
	}
	if got := []string{tools[0].Function.Name, tools[1].Function.Name}; !reflect.DeepEqual(got, []string{"alpha", "zulu"}) {
		t.Fatalf("tool order = %#v, want [alpha zulu]", got)
	}
}

func TestBuildRequestRendersStrictOpenAISchema(t *testing.T) {
	// R-2W35-53BH
	schema := json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$defs":{"detail":{"type":"object","properties":{"code":{"type":"string","pattern":"^[A-Z]+$"}}}},
		"type":"object",
		"properties":{
			"detail":{"$ref":"#/$defs/detail"},
			"choice":{"oneOf":[{"const":"a"},{"const":"b"}]}
		},
		"required":["detail"]
	}`)
	provider := New(Config{})
	request, _, err := provider.buildRequest(&agentkit.Request{
		Model: "compat-test",
		Tools: []agentkit.Tool{requestTestTool{
			name:        "lookup",
			description: "look up a value",
			schema:      schema,
		}},
	})
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(request.Tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(request.Tools))
	}
	function := request.Tools[0].Function
	if !function.Strict {
		t.Fatal("function strict = false, want true")
	}
	if want := RenderSchema(schema); !reflect.DeepEqual(function.Parameters, want) {
		t.Fatalf("parameters = %#v, want %#v", function.Parameters, want)
	}
	if _, exists := function.Parameters["$defs"]; exists {
		t.Fatal("rendered parameters retained $defs")
	}
	detail := function.Parameters["properties"].(map[string]any)["detail"].(map[string]any)
	if _, exists := detail["$ref"]; exists {
		t.Fatal("rendered detail retained $ref")
	}
	choice := function.Parameters["properties"].(map[string]any)["choice"].(map[string]any)
	if _, exists := choice["oneOf"]; exists {
		t.Fatal("rendered choice retained oneOf")
	}
	if _, exists := choice["anyOf"]; !exists {
		t.Fatal("rendered choice omitted anyOf")
	}
}
