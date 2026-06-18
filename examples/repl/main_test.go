package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestModelCommandSwitchesProviderAndRetainsHistory(t *testing.T) {
	// R-WCNR-SQFT
	conv := &agentkit.Conversation{
		Provider: fakeProvider{name: "old"},
		Model:    "old-model",
		History: []agentkit.Message{{
			Role:   agentkit.RoleUser,
			Blocks: []agentkit.Block{agentkit.TextBlock{Text: "keep me"}},
		}},
	}
	newProvider := &fakeProvider{name: "openai"}
	choices := map[string]providerChoice{
		"openai": {
			envKey:  "OPENAI_API_KEY",
			factory: func(string) agentkit.Provider { return newProvider },
		},
	}

	err := applyModelCommand(conv, "/model openai:gpt-5.4-mini", choices, func(key string) string {
		if key != "OPENAI_API_KEY" {
			t.Fatalf("unexpected env key %q", key)
		}
		return "test-key"
	})
	if err != nil {
		t.Fatalf("applyModelCommand returned error: %v", err)
	}
	if conv.Provider != newProvider {
		t.Fatalf("provider was not switched")
	}
	if conv.Model != "gpt-5.4-mini" {
		t.Fatalf("model = %q, want %q", conv.Model, "gpt-5.4-mini")
	}
	if len(conv.History) != 1 {
		t.Fatalf("history length = %d, want 1", len(conv.History))
	}
	text, ok := conv.History[0].Blocks[0].(agentkit.TextBlock)
	if !ok || text.Text != "keep me" {
		t.Fatalf("history was not retained: %#v", conv.History)
	}
}

func TestBashToolRunsThroughNormalToolLoop(t *testing.T) {
	// R-WDVO-6I6I
	provider := &toolLoopProvider{}
	conv := &agentkit.Conversation{
		Provider: provider,
		Model:    "fake-model",
		Tools:    []agentkit.Tool{bashTool()},
	}

	var out strings.Builder
	if err := sendAndPrint(context.Background(), conv, "use bash", &out); err != nil {
		t.Fatalf("sendAndPrint returned error: %v", err)
	}
	if !strings.Contains(out.String(), "saw: agentkit-bash") {
		t.Fatalf("stream output = %q, want final response with bash output", out.String())
	}
	if len(provider.requests) != 2 {
		t.Fatalf("round trips = %d, want 2", len(provider.requests))
	}
	if !provider.sawBashOutput {
		t.Fatalf("provider did not receive bash output in the follow-up request")
	}
}

type fakeProvider struct {
	name string
}

func (p fakeProvider) Name() string {
	return p.name
}

func (p fakeProvider) Pricing(string) (agentkit.Pricing, bool) {
	return agentkit.Pricing{Tiers: []agentkit.RateTier{{}}}, true
}

func (p fakeProvider) RoundTrip(context.Context, *agentkit.Request) *agentkit.RoundTrip {
	return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishStop, agentkit.Usage{}, nil, nil)
}

type toolLoopProvider struct {
	requests      []*agentkit.Request
	sawBashOutput bool
}

func (p *toolLoopProvider) Name() string {
	return "fake"
}

func (p *toolLoopProvider) Pricing(model string) (agentkit.Pricing, bool) {
	return agentkit.Pricing{Tiers: []agentkit.RateTier{{}}}, model == "fake-model"
}

func (p *toolLoopProvider) RoundTrip(_ context.Context, req *agentkit.Request) *agentkit.RoundTrip {
	p.requests = append(p.requests, req)
	switch len(p.requests) {
	case 1:
		if len(req.Tools) != 1 || req.Tools[0].Name() != "bash" {
			return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidConfig)
		}
		message := agentkit.Message{
			Role: agentkit.RoleAssistant,
			Blocks: []agentkit.Block{agentkit.ToolUseBlock{
				ID:    "toolu_01",
				Name:  "bash",
				Input: json.RawMessage(`{"command":"printf agentkit-bash"}`),
			}},
		}
		return agentkit.NewRoundTrip(nil, message, agentkit.FinishToolUse, agentkit.Usage{}, nil, nil)
	case 2:
		result := lastToolResult(req.Messages)
		if result == nil {
			return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidInput)
		}
		if result.Name == "bash" && result.ToolUseID == "toolu_01" && result.Content == "agentkit-bash" && !result.IsError {
			p.sawBashOutput = true
		}
		text := "saw: " + result.Content
		message := agentkit.Message{
			Role:   agentkit.RoleAssistant,
			Blocks: []agentkit.Block{agentkit.TextBlock{Text: text}},
		}
		return agentkit.NewRoundTrip(func(yield func(agentkit.Event) bool) {
			yield(agentkit.TextDelta{Text: text})
		}, message, agentkit.FinishStop, agentkit.Usage{}, nil, nil)
	default:
		return agentkit.NewRoundTrip(nil, agentkit.Message{}, agentkit.FinishOther, agentkit.Usage{}, nil, agentkit.ErrInvalidInput)
	}
}

func lastToolResult(messages []agentkit.Message) *agentkit.ToolResultBlock {
	for i := len(messages) - 1; i >= 0; i-- {
		for _, block := range messages[i].Blocks {
			if result, ok := block.(agentkit.ToolResultBlock); ok {
				return &result
			}
		}
	}
	return nil
}
