package agentkit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

type weatherInput struct {
	City  string `json:"city" jsonschema:"description=City name"`
	Units string `json:"units,omitempty" jsonschema:"description=Unit system"`
}

func TestNewToolSchemaReflectsStructFieldsAndIsByteStable(t *testing.T) {
	// R-WYZP-N2VB
	tool := agentkit.NewTool("get_weather", "Look up current weather",
		func(context.Context, weatherInput) (string, error) {
			return "", nil
		})

	first := tool.JSONSchema()
	second := tool.JSONSchema()
	if string(first) != string(second) {
		t.Fatalf("schema changed across calls:\nfirst:  %s\nsecond: %s", first, second)
	}

	first[0] = ' '
	if got := string(tool.JSONSchema()); got != string(second) {
		t.Fatalf("mutating returned schema changed cached schema: got %s, want %s", got, second)
	}

	var schema struct {
		Type       string `json:"type"`
		Required   []string
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		}
	}
	if err := json.Unmarshal(second, &schema); err != nil {
		t.Fatalf("json.Unmarshal(schema) error = %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("schema type = %q, want object", schema.Type)
	}
	if !contains(schema.Required, "city") {
		t.Fatalf("schema required = %v, want city", schema.Required)
	}
	if got := schema.Properties["city"].Description; got != "City name" {
		t.Fatalf("city description = %q, want City name", got)
	}
	if got := schema.Properties["city"].Type; got != "string" {
		t.Fatalf("city type = %q, want string", got)
	}
	if got := schema.Properties["units"].Description; got != "Unit system" {
		t.Fatalf("units description = %q, want Unit system", got)
	}
}

func TestNewToolCallDecodesInputAndReturnsToolResultContent(t *testing.T) {
	// R-X07M-0UM0
	tool := agentkit.NewTool("get_weather", "Look up current weather",
		func(_ context.Context, in weatherInput) (string, error) {
			if in.City != "Tokyo" {
				t.Fatalf("decoded City = %q, want Tokyo", in.City)
			}
			if in.Units != "metric" {
				t.Fatalf("decoded Units = %q, want metric", in.Units)
			}
			return "21 C", nil
		})

	use := agentkit.ToolUseBlock{
		ID:    agentkit.NewToolUseID(),
		Name:  tool.Name(),
		Input: json.RawMessage(`{"city":"Tokyo","units":"metric"}`),
	}
	content, err := tool.Call(context.Background(), use.Input)
	if err != nil {
		t.Fatalf("tool.Call() error = %v", err)
	}

	result := agentkit.ToolResultBlock{
		ToolUseID: use.ID,
		Name:      use.Name,
		Content:   content,
	}
	if result.Content != "21 C" {
		t.Fatalf("ToolResultBlock.Content = %q, want tool return", result.Content)
	}
}

func TestNewToolRequiredFieldsFollowJSONShape(t *testing.T) {
	// R-XZ8K-R3NX
	type input struct {
		Plain    string  `json:"plain"`
		Omitted  string  `json:"omitted,omitempty"`
		Pointer  *string `json:"pointer"`
		Required string  `json:"required" jsonschema:"description=No required marker"`
	}
	schema := decodeToolSchema(t, agentkit.NewTool("required", "", func(context.Context, input) (string, error) {
		return "", nil
	}))
	if got, want := schema["required"], []any{"plain", "required"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required = %#v, want %#v", got, want)
	}
}

func TestNewToolSchemaTagsUseCanonicalSubset(t *testing.T) {
	// R-Y0GH-4VEM
	type input struct {
		Value string   `json:"value" jsonschema:"description=A value,title=Value,enum=alpha,enum=beta,const=alpha,format=slug,pattern=^[a-z]+$,minLength=2,maxLength=8,default=alpha"`
		Items []string `json:"items,omitempty" jsonschema:"minItems=1"`
	}
	schema := decodeToolSchema(t, agentkit.NewTool("tags", "", func(context.Context, input) (string, error) {
		return "", nil
	}))
	properties := schema["properties"].(map[string]any)
	value := properties["value"].(map[string]any)
	want := map[string]any{
		"type":        "string",
		"description": "A value",
		"title":       "Value",
		"enum":        []any{"alpha", "beta"},
		"const":       "alpha",
		"format":      "slug",
		"pattern":     "^[a-z]+$",
		"minLength":   float64(2),
		"maxLength":   float64(8),
		"default":     "alpha",
	}
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("value schema = %#v, want %#v", value, want)
	}
	if got := properties["items"].(map[string]any)["minItems"]; got != float64(1) {
		t.Fatalf("items minItems = %#v, want 1", got)
	}

	assertToolPanic(t, []string{"minimum"}, func() {
		type badInput struct {
			Count int `json:"count" jsonschema:"minimum=1"`
		}
		agentkit.NewTool("bad", "", func(context.Context, badInput) (string, error) { return "", nil })
	})
}

func TestNewToolInlinesNestedStructsWithoutRendererKeywords(t *testing.T) {
	// R-Y1OD-IN5B
	type location struct {
		City string `json:"city"`
	}
	type input struct {
		Location location `json:"location"`
	}
	schema := decodeToolSchema(t, agentkit.NewTool("nested", "", func(context.Context, input) (string, error) {
		return "", nil
	}))
	locationSchema := schema["properties"].(map[string]any)["location"].(map[string]any)
	if got := locationSchema["properties"].(map[string]any)["city"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("nested city type = %#v, want string", got)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"$schema"`, `"$defs"`, `"$ref"`, `"additionalProperties"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("schema contains forbidden keyword %s: %s", forbidden, encoded)
		}
	}
}

func TestNewToolReflectsTimeEmbeddedAndIgnoredFields(t *testing.T) {
	// R-DIVW-07P0
	type common struct {
		Query string `json:"query"`
	}
	type input struct {
		common
		At     time.Time `json:"at"`
		Secret string    `json:"-"`
	}
	schema := decodeToolSchema(t, agentkit.NewTool("edges", "", func(context.Context, input) (string, error) {
		return "", nil
	}))
	properties := schema["properties"].(map[string]any)
	if _, ok := properties["query"]; !ok {
		t.Fatalf("embedded query absent from properties: %#v", properties)
	}
	if _, ok := properties["common"]; ok {
		t.Fatalf("embedded struct was not flattened: %#v", properties)
	}
	if _, ok := properties["Secret"]; ok {
		t.Fatalf("json:\"-\" field present: %#v", properties)
	}
	wantTime := map[string]any{"type": "string", "format": "date-time"}
	if got := properties["at"]; !reflect.DeepEqual(got, wantTime) {
		t.Fatalf("time schema = %#v, want %#v", got, wantTime)
	}
}

type directRecursive struct {
	Next *directRecursive `json:"next,omitempty"`
}

type mutualA struct {
	B *mutualB `json:"b,omitempty"`
}

type mutualB struct {
	A *mutualA `json:"a,omitempty"`
}

func TestNewToolRejectsUnrepresentableInputTypes(t *testing.T) {
	// R-AIWI-P5JP
	tests := []struct {
		name      string
		field     string
		construct string
		build     func()
	}{
		{"any", "Value", "interface/any", func() {
			type input struct {
				Value any `json:"value"`
			}
			agentkit.NewTool("bad", "", func(context.Context, input) (string, error) { return "", nil })
		}},
		{"raw message", "Value", "json.RawMessage", func() {
			type input struct {
				Value json.RawMessage `json:"value"`
			}
			agentkit.NewTool("bad", "", func(context.Context, input) (string, error) { return "", nil })
		}},
		{"map", "Value", "map", func() {
			type input struct {
				Value map[string]string `json:"value"`
			}
			agentkit.NewTool("bad", "", func(context.Context, input) (string, error) { return "", nil })
		}},
		{"direct recursion", "Next", "recursive struct", func() {
			agentkit.NewTool("bad", "", func(context.Context, directRecursive) (string, error) { return "", nil })
		}},
		{"mutual recursion", "B", "recursive struct", func() {
			agentkit.NewTool("bad", "", func(context.Context, mutualA) (string, error) { return "", nil })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolPanic(t, []string{tt.field, tt.construct}, tt.build)
		})
	}
}

func TestRawToolIsAbsentAndNewToolIsSoleToolConstructor(t *testing.T) {
	// R-Y2W9-WEW0
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	file, err := parser.ParseFile(token.NewFileSet(), strings.TrimSuffix(filename, "_test.go")+".go", nil, 0)
	if err != nil {
		t.Fatalf("parse tool.go: %v", err)
	}
	var constructors []string
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Type.Results == nil {
			continue
		}
		for _, result := range fn.Type.Results.List {
			if ident, ok := result.Type.(*ast.Ident); ok && ident.Name == "Tool" {
				constructors = append(constructors, fn.Name.Name)
			}
		}
		if fn.Name.Name == "RawTool" {
			t.Fatal("RawTool remains in the public API")
		}
	}
	if want := []string{"NewTool"}; !reflect.DeepEqual(constructors, want) {
		t.Fatalf("exported Tool constructors = %v, want %v", constructors, want)
	}
}

func decodeToolSchema(t *testing.T, tool agentkit.Tool) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(tool.JSONSchema(), &schema); err != nil {
		t.Fatalf("json.Unmarshal(schema) error = %v", err)
	}
	return schema
}

func assertToolPanic(t *testing.T, contains []string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("NewTool did not panic; want message containing %q", contains)
		}
		message := fmt.Sprint(recovered)
		for _, part := range contains {
			if !strings.Contains(message, part) {
				t.Fatalf("panic = %q, want message containing %q", message, part)
			}
		}
	}()
	fn()
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
