//go:build integration

package agentkit_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/anthropic"
)

func TestAnthropicLiveDeferredToolsLoadsAndCallsNativeTool(t *testing.T) {
	// R-DFH0-A8TE
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}

	const (
		toolName   = "reveal_deferred_probe"
		toolOutput = "deferred-live-result:a8te-6b925f14"
	)
	tool := agentkit.RawTool(toolName, "Return the hidden live deferred-tools verification token.", json.RawMessage(`{
		"type":"object",
		"properties":{
			"reason":{"type":"string"}
		},
		"required":["reason"],
		"additionalProperties":false
	}`), func(context.Context, json.RawMessage) (string, error) {
		return toolOutput, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	zero := 0.0
	conv := &agentkit.Conversation{
		Provider: anthropic.New(anthropic.APIKey(key)),
		Model:    "claude-haiku-4-5",
		System: "Use tools exactly as requested. For deferred tools, first call load_tools with the requested tool name. " +
			"After a tool result, answer with only the exact returned token.",
		Gen: agentkit.GenSettings{
			Temperature: &zero,
			MaxTokens:   512,
		},
		DeferredTools: []agentkit.DeferredToolGroup{{
			Name:  "live_deferred_probe",
			Blurb: "Tool for the live deferred-tools integration check.",
			Tools: []agentkit.Tool{tool},
		}},
		MaxToolIterations: 4,
	}
	stream := conv.Send(ctx, "First call load_tools for reveal_deferred_probe. Then call reveal_deferred_probe with reason set to live integration. Finally answer with only the tool result.")

	var sawLoadTools bool
	var sawLoadResult bool
	var sawDeferredUse bool
	var sawDeferredResult bool
	var finalText string
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.ToolUse:
			switch event.Name {
			case "load_tools":
				var input struct {
					Tools []string `json:"tools"`
				}
				if err := json.Unmarshal(event.Input, &input); err != nil {
					t.Fatalf("load_tools input is invalid JSON: %s", event.Input)
				}
				if !containsString(input.Tools, toolName) {
					t.Fatalf("load_tools input = %s, want %q", event.Input, toolName)
				}
				sawLoadTools = true
			case toolName:
				if !sawLoadResult {
					t.Fatalf("%s was called before a successful load_tools result", toolName)
				}
				sawDeferredUse = true
			default:
				t.Fatalf("unexpected tool use %q", event.Name)
			}
		case agentkit.ToolResult:
			switch event.Name {
			case "load_tools":
				if event.IsError {
					t.Fatalf("load_tools result is error: %s", event.Output)
				}
				if !strings.Contains(event.Output, toolName) {
					t.Fatalf("load_tools result = %s, want loaded %q", event.Output, toolName)
				}
				sawLoadResult = true
			case toolName:
				if event.Output != toolOutput || event.IsError {
					t.Fatalf("%s result = %#v, want successful hidden token", toolName, event)
				}
				sawDeferredResult = true
			default:
				t.Fatalf("unexpected tool result %q", event.Name)
			}
		case agentkit.MessageDone:
			finalText = integrationMessageText(event.Message)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if !sawLoadTools || !sawLoadResult || !sawDeferredUse || !sawDeferredResult {
		t.Fatalf("events did not include load_tools and native deferred tool call: loadUse=%v loadResult=%v deferredUse=%v deferredResult=%v",
			sawLoadTools, sawLoadResult, sawDeferredUse, sawDeferredResult)
	}
	if !strings.Contains(finalText, toolOutput) {
		t.Fatalf("final answer = %q, want hidden tool output %q", finalText, toolOutput)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func integrationMessageText(message agentkit.Message) string {
	var b strings.Builder
	for _, block := range message.Blocks {
		if text, ok := block.(agentkit.TextBlock); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}
