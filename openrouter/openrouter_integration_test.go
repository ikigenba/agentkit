//go:build integration

package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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

func TestOpenRouterIntegrationEnableReasoningTurnsOnGrok(t *testing.T) {
	// R-DGCO-E7GX
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	const prompt = "Find the smallest positive integer that is congruent to 2 modulo 3, 3 modulo 5, and 2 modulo 7. Think through the calculation carefully for at least 200 internal reasoning tokens, then give only the final integer."
	tests := []struct {
		name      string
		reasoning agentkit.ReasoningValue
		check     func(int64) bool
		want      string
	}{
		{name: "unset", check: func(tokens int64) bool { return tokens == 0 }, want: "zero"},
		{name: "enabled", reasoning: agentkit.EnableReasoning(), check: func(tokens int64) bool { return tokens > 100 }, want: "more than 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			capture := &responseCaptureTransport{base: http.DefaultTransport}
			roundTrip := New(APIKey(key), WithHTTPClient(&http.Client{Transport: capture})).RoundTrip(ctx, &agentkit.Request{
				Model: strings.Join([]string{"x-ai", "grok-4.20"}, "/"),
				Messages: []agentkit.Message{{
					Role:   agentkit.RoleUser,
					Blocks: []agentkit.Block{agentkit.TextBlock{Text: prompt}},
				}},
				Gen: agentkit.GenSettings{MaxTokens: 2048, Reasoning: tt.reasoning},
			})
			if err := roundTrip.Err(); err != nil {
				t.Fatalf("RoundTrip() error = %v", err)
			}
			if text := strings.TrimSpace(openRouterMessageText(roundTrip.Message())); text == "" {
				t.Fatal("completed response has no assistant text")
			}
			got, ok := openRouterReasoningTokens(capture.raw)
			if !ok {
				t.Fatalf("completed response did not report reasoning_tokens")
			}
			if !tt.check(got) {
				t.Fatalf("reasoning tokens = %d, want %s", got, tt.want)
			}
		})
	}
}

func openRouterReasoningTokens(raw []byte) (int64, bool) {
	var tokens int64
	var found bool
	for _, line := range bytes.Split(raw, []byte("\n")) {
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		var frame struct {
			Usage *struct {
				CompletionDetails struct {
					ReasoningTokens int64 `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage"`
		}
		if json.Unmarshal(data, &frame) == nil && frame.Usage != nil {
			tokens = frame.Usage.CompletionDetails.ReasoningTokens
			found = true
		}
	}
	return tokens, found
}

type responseCaptureTransport struct {
	base http.RoundTripper
	raw  []byte
}

func (t *responseCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.raw, err = io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(t.raw))
	return resp, nil
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
