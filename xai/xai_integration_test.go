//go:build integration

package xai

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestXAIIntegrationPlainRoundTrip(t *testing.T) {
	// R-DMKT-ZAQ1
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{Provider: New(APIKey(key)), Model: "grok-4.20"}).Send(ctx, "Reply with one short sentence.")
	var text strings.Builder
	for event := range stream.Events() {
		if done, ok := event.(agentkit.MessageDone); ok {
			text.WriteString(messageText(done.Message))
		}
	}
	if stream.Err() != nil || strings.TrimSpace(text.String()) == "" {
		t.Fatalf("completed stream error/text = %v/%q", stream.Err(), text.String())
	}
}

type integrationEchoInput struct {
	Value string `json:"value"`
}

func TestXAIIntegrationToolRoundTrip(t *testing.T) {
	// R-DNSQ-D2GQ
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY is not set")
	}
	tool := agentkit.NewTool("integration_echo", "Return the supplied value.", func(_ context.Context, _ integrationEchoInput) (string, error) {
		return "xai-tool-ok", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{Provider: New(APIKey(key)), Model: "grok-4.20", Tools: []agentkit.Tool{tool}}).Send(ctx, "Call integration_echo with value test, then report its result.")
	var sawUse, sawResult, sawFinal bool
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.ToolUse:
			sawUse = event.Name == "integration_echo"
		case agentkit.ToolResult:
			sawResult = event.Name == "integration_echo" && event.Output == "xai-tool-ok" && !event.IsError
		case agentkit.MessageDone:
			if sawResult && strings.TrimSpace(messageText(event.Message)) != "" {
				sawFinal = true
			}
		}
	}
	if stream.Err() != nil || !sawUse || !sawResult || !sawFinal {
		t.Fatalf("tool round trip error/use/result/final = %v/%v/%v/%v", stream.Err(), sawUse, sawResult, sawFinal)
	}
}

func messageText(message agentkit.Message) string {
	var text strings.Builder
	for _, block := range message.Blocks {
		if block, ok := block.(agentkit.TextBlock); ok {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}
