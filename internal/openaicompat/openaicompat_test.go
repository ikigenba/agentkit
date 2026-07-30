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
