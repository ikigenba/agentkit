//go:build integration

package anthropic

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

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

func TestAnthropicIntegrationSkipsWithoutCredential(t *testing.T) {
	// R-WJLM-7QRP
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}
	conv := &agentkit.Conversation{Provider: New(APIKey(key)), Model: "claude-haiku-4-5"}
	stream := conv.Send(context.Background(), "Reply with one short sentence.")
	for range stream.Events() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
}

func TestAnthropicIntegrationPlainRoundTrip(t *testing.T) {
	// R-CL9K-41F1
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{
		Provider: New(APIKey(key)),
		Model:    "claude-haiku-4-5",
	}).Send(ctx, "Reply with one short sentence.")

	var text strings.Builder
	for event := range stream.Events() {
		if done, ok := event.(agentkit.MessageDone); ok {
			text.WriteString(anthropicMessageText(done.Message))
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if strings.TrimSpace(text.String()) == "" {
		t.Fatal("completed stream has no assistant text")
	}
}

func TestAnthropicIntegrationToolRoundTrip(t *testing.T) {
	// R-CMHG-HT5Q
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}

	tool := testRawTool("integration_echo", "Return the supplied value.", json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`), func(_ context.Context, input json.RawMessage) (string, error) {
		return "anthropic-tool-ok", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{
		Provider: New(APIKey(key)),
		Model:    "claude-haiku-4-5",
		Tools:    []agentkit.Tool{tool},
	}).Send(ctx, "Call integration_echo with value test, then report its result.")

	var sawUse, sawResult, sawFinal bool
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.ToolUse:
			sawUse = event.Name == "integration_echo"
		case agentkit.ToolResult:
			sawResult = event.Name == "integration_echo" && event.Output == "anthropic-tool-ok" && !event.IsError
		case agentkit.MessageDone:
			if sawResult && strings.TrimSpace(anthropicMessageText(event.Message)) != "" {
				sawFinal = true
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if !sawUse || !sawResult || !sawFinal {
		t.Fatalf("incomplete tool round trip: use=%v result=%v final=%v", sawUse, sawResult, sawFinal)
	}
}

func anthropicMessageText(message agentkit.Message) string {
	var text strings.Builder
	for _, block := range message.Blocks {
		if block, ok := block.(agentkit.TextBlock); ok {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}
