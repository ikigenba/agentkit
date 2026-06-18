package agentkit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
)

type weatherInput struct {
	City  string `json:"city" jsonschema:"required,description=City name"`
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

func TestRawToolCallsWithRawInputAndReturnsToolResultContent(t *testing.T) {
	// R-X2NE-SE3E
	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)
	tool := agentkit.RawTool("search", "Search by raw query", schema,
		func(_ context.Context, input json.RawMessage) (string, error) {
			if got := string(input); got != `{"query":"agentkit"}` {
				t.Fatalf("raw input = %s, want original JSON bytes", got)
			}
			return strings.ToUpper("found"), nil
		})

	if got := string(tool.JSONSchema()); got != string(schema) {
		t.Fatalf("RawTool.JSONSchema() = %s, want hand-written schema %s", got, schema)
	}

	use := agentkit.ToolUseBlock{
		ID:    agentkit.NewToolUseID(),
		Name:  tool.Name(),
		Input: json.RawMessage(`{"query":"agentkit"}`),
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
	if result.Content != "FOUND" {
		t.Fatalf("ToolResultBlock.Content = %q, want raw tool return", result.Content)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
