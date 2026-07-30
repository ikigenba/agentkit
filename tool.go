package agentkit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Tool is a registered capability the model may invoke. It is sealed so only
// constructors in this package (and package-owned wrappers such as MCP tools)
// can add implementations to a Conversation.
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
//
// NewTool panics when In cannot be represented by AgentKit's canonical schema
// subset. Such a type is a programming error and is reported at construction.
func NewTool[In any](name, description string, fn func(ctx context.Context, in In) (string, error)) Tool {
	inputType := reflect.TypeFor[In]()
	schema := reflectToolSchema(inputType)
	encoded, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("agentkit: reflect tool input %s: %v", inputType, err))
	}

	return typedTool[In]{
		name:        name,
		description: description,
		schema:      append(json.RawMessage(nil), encoded...),
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

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[json.RawMessage]()
)

func reflectToolSchema(inputType reflect.Type) map[string]any {
	if inputType == nil {
		panic("agentkit: tool input <nil> has unsupported construct")
	}
	if inputType.Kind() != reflect.Struct {
		panic(fmt.Sprintf("agentkit: tool input %s has unsupported %s construct; input must be a struct", inputType, inputType.Kind()))
	}
	return reflectSchema(inputType, inputType.String(), make(map[reflect.Type]bool))
}

func reflectSchema(typ reflect.Type, path string, active map[reflect.Type]bool) map[string]any {
	if typ == rawMessageType {
		panic(fmt.Sprintf("agentkit: tool input field %s has unsupported json.RawMessage construct", path))
	}

	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
		if typ == rawMessageType {
			panic(fmt.Sprintf("agentkit: tool input field %s has unsupported json.RawMessage construct", path))
		}
	}

	if typ == timeType {
		return map[string]any{"type": "string", "format": "date-time"}
	}

	switch typ.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Array, reflect.Slice:
		if typ == rawMessageType {
			panic(fmt.Sprintf("agentkit: tool input field %s has unsupported json.RawMessage construct", path))
		}
		return map[string]any{
			"type":  "array",
			"items": reflectSchema(typ.Elem(), path+"[]", active),
		}
	case reflect.Struct:
		if active[typ] {
			panic(fmt.Sprintf("agentkit: tool input field %s has unsupported recursive struct construct (%s)", path, typ))
		}
		active[typ] = true
		defer delete(active, typ)

		properties := make(map[string]any)
		var required []string
		reflectStructFields(typ, path, active, properties, &required)
		schema := map[string]any{"type": "object", "properties": properties}
		if len(required) != 0 {
			schema["required"] = required
		}
		return schema
	case reflect.Interface:
		return panicUnsupported(path, "interface/any")
	case reflect.Map:
		return panicUnsupported(path, "map")
	case reflect.Chan:
		return panicUnsupported(path, "chan")
	case reflect.Func:
		return panicUnsupported(path, "func")
	case reflect.Complex64, reflect.Complex128:
		return panicUnsupported(path, "complex")
	default:
		return panicUnsupported(path, typ.Kind().String())
	}
}

func panicUnsupported(path, construct string) map[string]any {
	panic(fmt.Sprintf("agentkit: tool input field %s has unsupported %s construct", path, construct))
}

func reflectStructFields(typ reflect.Type, path string, active map[reflect.Type]bool, properties map[string]any, required *[]string) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name, options := jsonFieldName(field)
		if name == "-" {
			continue
		}

		embeddedType := field.Type
		for embeddedType.Kind() == reflect.Pointer {
			embeddedType = embeddedType.Elem()
		}
		if field.Anonymous && name == "" && embeddedType.Kind() == reflect.Struct && embeddedType != timeType {
			if active[embeddedType] {
				panic(fmt.Sprintf("agentkit: tool input field %s.%s has unsupported recursive struct construct (%s)", path, field.Name, embeddedType))
			}
			active[embeddedType] = true
			reflectStructFields(embeddedType, path+"."+field.Name, active, properties, required)
			delete(active, embeddedType)
			continue
		}
		if field.PkgPath != "" {
			continue
		}

		if name == "" {
			name = field.Name
		}
		fieldPath := path + "." + field.Name
		fieldSchema := reflectSchema(field.Type, fieldPath, active)
		applySchemaTag(fieldSchema, field, fieldPath)
		properties[name] = fieldSchema
		if !options["omitempty"] && field.Type.Kind() != reflect.Pointer {
			*required = append(*required, name)
		}
	}
}

func jsonFieldName(field reflect.StructField) (string, map[string]bool) {
	parts := strings.Split(field.Tag.Get("json"), ",")
	options := make(map[string]bool, len(parts))
	for _, option := range parts[1:] {
		options[option] = true
	}
	return parts[0], options
}

func applySchemaTag(schema map[string]any, field reflect.StructField, path string) {
	tag := field.Tag.Get("jsonschema")
	if tag == "" {
		return
	}

	for _, item := range strings.Split(tag, ",") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			panic(fmt.Sprintf("agentkit: tool input field %s has invalid jsonschema tag %q", path, item))
		}
		switch key {
		case "description", "title", "format", "pattern":
			schema[key] = value
		case "enum":
			schema[key] = append(schemaStrings(schema[key]), scalarTagValue(field.Type, value, path, key))
		case "const", "default":
			schema[key] = scalarTagValue(field.Type, value, path, key)
		case "minLength", "maxLength", "minItems":
			number, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				panic(fmt.Sprintf("agentkit: tool input field %s has invalid jsonschema %s value %q", path, key, value))
			}
			schema[key] = number
		default:
			panic(fmt.Sprintf("agentkit: tool input field %s has unsupported jsonschema key %q", path, key))
		}
	}
}

func schemaStrings(existing any) []any {
	if existing == nil {
		return nil
	}
	return existing.([]any)
}

func scalarTagValue(typ reflect.Type, value, path, key string) any {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	var (
		result any
		err    error
	)
	switch typ.Kind() {
	case reflect.String:
		result = value
	case reflect.Bool:
		result, err = strconv.ParseBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result, err = strconv.ParseInt(value, 10, 64)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result, err = strconv.ParseUint(value, 10, 64)
	case reflect.Float32, reflect.Float64:
		result, err = strconv.ParseFloat(value, 64)
	default:
		err = fmt.Errorf("not a scalar")
	}
	if err != nil {
		panic(fmt.Sprintf("agentkit: tool input field %s has invalid jsonschema %s value %q for %s", path, key, value, typ))
	}
	return result
}
