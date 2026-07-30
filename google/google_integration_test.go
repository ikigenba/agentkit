//go:build integration

package google

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestGoogleIntegrationPlainRoundTrip(t *testing.T) {
	// R-CNPC-VKWF
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
	}
	if key == "" {
		t.Skip("GEMINI_API_KEY or GOOGLE_API_KEY is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{
		Provider: New(APIKey(key)),
		Model:    "gemini-2.5-flash",
	}).Send(ctx, "Reply with one short sentence.")
	var text strings.Builder
	for event := range stream.Events() {
		if done, ok := event.(agentkit.MessageDone); ok {
			text.WriteString(messageText(done.Message))
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if strings.TrimSpace(text.String()) == "" {
		t.Fatal("completed stream has no assistant text")
	}
}

func TestGoogleIntegrationToolRoundTrip(t *testing.T) {
	// R-Y5C2-NYDE
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
	}
	if key == "" {
		t.Skip("GEMINI_API_KEY or GOOGLE_API_KEY is not set")
	}

	tool := testRawTool("integration_echo", "Return the supplied value.", json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "google-tool-ok", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{
		Provider: New(APIKey(key)),
		Model:    "gemini-2.5-flash",
		Tools:    []agentkit.Tool{tool},
	}).Send(ctx, "Call integration_echo with value test, then report its result.")

	var sawUse, sawResult, sawFinal bool
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.ToolUse:
			sawUse = event.Name == "integration_echo"
		case agentkit.ToolResult:
			sawResult = event.Name == "integration_echo" && event.Output == "google-tool-ok" && !event.IsError
		case agentkit.MessageDone:
			if sawResult && strings.TrimSpace(messageText(event.Message)) != "" {
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

func messageText(message agentkit.Message) string {
	var b strings.Builder
	for _, block := range message.Blocks {
		if text, ok := block.(agentkit.TextBlock); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}
