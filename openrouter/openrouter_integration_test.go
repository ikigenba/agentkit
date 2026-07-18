//go:build integration

package openrouter

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestOpenRouterIntegrationToolRoundTrip(t *testing.T) {
	// R-CTSU-SFLW
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	tool := agentkit.RawTool("integration_echo", "Return the supplied value.", json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "openrouter-tool-ok", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{Provider: New(APIKey(key)), Model: "openai/gpt-5.4-mini", Tools: []agentkit.Tool{tool}}).Send(ctx, "Call integration_echo with value test, then report its result.")
	var sawUse, sawResult, sawFinal bool
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.ToolUse:
			sawUse = event.Name == "integration_echo"
		case agentkit.ToolResult:
			sawResult = event.Name == "integration_echo" && event.Output == "openrouter-tool-ok" && !event.IsError
		case agentkit.MessageDone:
			if sawResult && strings.TrimSpace(openRouterMessageText(event.Message)) != "" {
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

func TestOpenRouterIntegrationReportedCost(t *testing.T) {
	// R-DF22-UT85
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	roundTrip := New(APIKey(key)).RoundTrip(ctx, &agentkit.Request{
		Model: "openai/gpt-5.4-mini",
		Messages: []agentkit.Message{{
			Role:   agentkit.RoleUser,
			Blocks: []agentkit.Block{agentkit.TextBlock{Text: "Reply with one short sentence."}},
		}},
	})
	if err := roundTrip.Err(); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if text := strings.TrimSpace(openRouterMessageText(roundTrip.Message())); text == "" {
		t.Fatal("assembled message has no assistant text")
	}
	if cost, ok := roundTrip.ReportedCost(); !ok || cost <= 0 {
		t.Fatalf("ReportedCost() = %d/%v, want a present cost greater than zero", cost, ok)
	}
}

func TestOpenRouterIntegrationCatalogAggregatorSlugs(t *testing.T) {
	// R-4NJ4-SJ41
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	slugs := []string{
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4-pro",
		"moonshotai/kimi-k2.6",
		"moonshotai/kimi-k2.7-code",
		"moonshotai/kimi-k3",
		"x-ai/grok-4.20",
		"x-ai/grok-4.20-multi-agent",
		"x-ai/grok-4.3",
		"x-ai/grok-4.5",
	}
	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			roundTrip := New(APIKey(key)).RoundTrip(ctx, &agentkit.Request{
				Model: slug,
				Messages: []agentkit.Message{{
					Role:   agentkit.RoleUser,
					Blocks: []agentkit.Block{agentkit.TextBlock{Text: "Reply with OK."}},
				}},
			})
			if err := roundTrip.Err(); err != nil {
				t.Fatalf("minimal round trip for %q: %v", slug, err)
			}
			if text := strings.TrimSpace(openRouterMessageText(roundTrip.Message())); text == "" {
				t.Fatalf("minimal round trip for %q returned no assistant text", slug)
			}
		})
	}
}

func openRouterMessageText(message agentkit.Message) string {
	var text strings.Builder
	for _, block := range message.Blocks {
		if block, ok := block.(agentkit.TextBlock); ok {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}
