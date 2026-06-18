package agentkit

import (
	"context"
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// Tool is a registered capability the model may invoke.
//
// Tool is sealed so consumers construct tools with NewTool or RawTool, while
// orchestration can hold a heterogeneous []Tool.
type Tool interface {
	Name() string
	Description() string
	JSONSchema() json.RawMessage
	Call(ctx context.Context, input json.RawMessage) (string, error)
	isTool()
}

type typedTool[In any] struct {
	name        string
	description string
	schema      json.RawMessage
	fn          func(context.Context, In) (string, error)
}

// NewTool builds a Tool from a typed input struct. The JSON Schema is derived
// once from In and cached; Call decodes input into In before invoking fn.
func NewTool[In any](name, description string, fn func(ctx context.Context, in In) (string, error)) Tool {
	reflector := jsonschema.Reflector{
		Anonymous:                  true,
		ExpandedStruct:             true,
		RequiredFromJSONSchemaTags: true,
	}

	schema, err := reflector.Reflect((*In)(nil)).MarshalJSON()
	if err != nil {
		panic(err)
	}

	return typedTool[In]{
		name:        name,
		description: description,
		schema:      append(json.RawMessage(nil), schema...),
		fn:          fn,
	}
}

func (t typedTool[In]) Name() string {
	return t.name
}

func (t typedTool[In]) Description() string {
	return t.description
}

func (t typedTool[In]) JSONSchema() json.RawMessage {
	return append(json.RawMessage(nil), t.schema...)
}

func (t typedTool[In]) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in In
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	return t.fn(ctx, in)
}

func (t typedTool[In]) isTool() {}

type rawTool struct {
	name        string
	description string
	schema      json.RawMessage
	fn          func(context.Context, json.RawMessage) (string, error)
}

// RawTool builds a Tool with a hand-written JSON Schema and raw input bytes.
func RawTool(name, description string, schema json.RawMessage, fn func(ctx context.Context, input json.RawMessage) (string, error)) Tool {
	return rawTool{
		name:        name,
		description: description,
		schema:      append(json.RawMessage(nil), schema...),
		fn:          fn,
	}
}

func (t rawTool) Name() string {
	return t.name
}

func (t rawTool) Description() string {
	return t.description
}

func (t rawTool) JSONSchema() json.RawMessage {
	return append(json.RawMessage(nil), t.schema...)
}

func (t rawTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.fn(ctx, append(json.RawMessage(nil), input...))
}

func (t rawTool) isTool() {}
