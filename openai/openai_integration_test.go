//go:build integration

package openai

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestOpenAIIntegrationPlainRoundTrip(t *testing.T) {
	// R-COX9-9CN4
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{Provider: New(APIKey(key)), Model: "gpt-5.4-mini"}).Send(ctx, "Reply with one short sentence.")
	var text strings.Builder
	for event := range stream.Events() {
		if done, ok := event.(agentkit.MessageDone); ok {
			text.WriteString(openAIMessageText(done.Message))
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if strings.TrimSpace(text.String()) == "" {
		t.Fatal("completed stream has no assistant text")
	}
}

func TestOpenAIIntegrationToolRoundTrip(t *testing.T) {
	// R-CQ55-N4DT
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	tool := agentkit.RawTool("integration_echo", "Return the supplied value.", json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "openai-tool-ok", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{Provider: New(APIKey(key)), Model: "gpt-5.4-mini", Tools: []agentkit.Tool{tool}}).Send(ctx, "Call integration_echo with value test, then report its result.")
	assertOpenAIToolRoundTrip(t, stream, "openai-tool-ok")
}

func assertOpenAIToolRoundTrip(t *testing.T, stream *agentkit.Stream, output string) {
	t.Helper()
	var sawUse, sawResult, sawFinal bool
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.ToolUse:
			sawUse = event.Name == "integration_echo"
		case agentkit.ToolResult:
			sawResult = event.Name == "integration_echo" && event.Output == output && !event.IsError
		case agentkit.MessageDone:
			if sawResult && strings.TrimSpace(openAIMessageText(event.Message)) != "" {
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

func openAIMessageText(message agentkit.Message) string {
	var text strings.Builder
	for _, block := range message.Blocks {
		if block, ok := block.(agentkit.TextBlock); ok {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}
