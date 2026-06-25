//go:build integration

package google

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestGoogleIntegrationAcceptsRefOneOfToolSchemaAndRoundTripsToolCall(t *testing.T) {
	// R-9UK4-JI3L
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
	}
	if key == "" {
		t.Skip("GEMINI_API_KEY or GOOGLE_API_KEY is not set")
	}

	const toolOutput = "round-trip-ok: overnight to Austin, US"
	tool := agentkit.RawTool("select_shipping", "Select a shipping speed for a destination.", json.RawMessage(`{
		"type":"object",
		"properties":{
			"destination":{"$ref":"#/$defs/destination"},
			"delivery":{"oneOf":[
				{"type":"string","enum":["standard","overnight"]},
				{
					"type":"object",
					"properties":{"speed":{"type":"string","enum":["standard","overnight"]}},
					"required":["speed"]
				}
			]}
		},
		"required":["destination","delivery"],
		"$defs":{
			"destination":{
				"type":"object",
				"properties":{
					"city":{"type":"string"},
					"country":{"type":"string"}
				},
				"required":["city","country"]
			}
		}
	}`), func(ctx context.Context, input json.RawMessage) (string, error) {
		var payload struct {
			Destination struct {
				City    string `json:"city"`
				Country string `json:"country"`
			} `json:"destination"`
			Delivery json.RawMessage `json:"delivery"`
		}
		if err := json.Unmarshal(input, &payload); err != nil {
			return "", err
		}
		if payload.Destination.City != "Austin" || payload.Destination.Country != "US" {
			return "", fmt.Errorf("destination = %s, %s, want Austin, US", payload.Destination.City, payload.Destination.Country)
		}
		if !deliveryIsOvernight(payload.Delivery) {
			return "", fmt.Errorf("delivery = %s, want overnight", payload.Delivery)
		}
		return toolOutput, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	conv := &agentkit.Conversation{
		Provider: New(key),
		Model:    ModelFlash25,
		Tools:    []agentkit.Tool{tool},
	}
	stream := conv.Send(ctx, "Call select_shipping exactly once with destination city Austin, country US, and delivery overnight. After the tool result, reply with only the exact tool output string.")

	var sawUse bool
	var sawResult bool
	var sawFinalDone bool
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.ToolUse:
			if event.Name != "select_shipping" {
				t.Fatalf("ToolUse.Name = %q, want select_shipping", event.Name)
			}
			var input map[string]json.RawMessage
			if err := json.Unmarshal(event.Input, &input); err != nil {
				t.Fatalf("ToolUse.Input is invalid JSON: %s", event.Input)
			}
			if _, ok := input["destination"]; !ok {
				t.Fatalf("ToolUse.Input = %s, want destination from $ref schema", event.Input)
			}
			if _, ok := input["delivery"]; !ok {
				t.Fatalf("ToolUse.Input = %s, want delivery from oneOf schema", event.Input)
			}
			sawUse = true
		case agentkit.ToolResult:
			if event.Name != "select_shipping" || event.Output != toolOutput || event.IsError {
				t.Fatalf("ToolResult = %#v, want successful select_shipping result", event)
			}
			sawResult = true
		case agentkit.MessageDone:
			if sawResult && event.Message.Role == agentkit.RoleAssistant && messageText(event.Message) != "" {
				sawFinalDone = true
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	for _, warning := range stream.Warnings() {
		if warning.Code == agentkit.WarnToolSchemaLossy {
			t.Fatalf("unexpected lossy schema warning: %#v", warning)
		}
	}
	if !sawUse || !sawResult || !sawFinalDone {
		t.Fatalf("events did not include complete tool round trip: sawUse=%v sawResult=%v sawFinalDone=%v", sawUse, sawResult, sawFinalDone)
	}
}

func deliveryIsOvernight(raw json.RawMessage) bool {
	var speed string
	if err := json.Unmarshal(raw, &speed); err == nil {
		return strings.EqualFold(speed, "overnight")
	}

	var object struct {
		Speed string `json:"speed"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return false
	}
	return strings.EqualFold(object.Speed, "overnight")
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
