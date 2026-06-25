package agentkit_test

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/ikigenba/agentkit"
)

var strictToolUseID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func TestToolUseIDMintsStrictCharsetAndPairsWithResult(t *testing.T) {
	// R-IKKQ-Z3B4
	id := agentkit.NewToolUseID()
	if !strictToolUseID.MatchString(id) {
		t.Fatalf("NewToolUseID() = %q, want Anthropic strict charset", id)
	}

	tool := agentkit.NewTool("lookup", "look up a query", func(_ context.Context, in struct {
		Q string `json:"q"`
	}) (string, error) {
		if in.Q != "agentkit" {
			t.Fatalf("decoded q = %q, want agentkit", in.Q)
		}
		return "found", nil
	})
	provider := newFakeProvider(
		newRoundTrip(
			assistant(agentkit.ToolUseBlock{ID: id, Name: "lookup", Input: json.RawMessage(`{"q":"agentkit"}`)}),
			agentkit.FinishToolUse,
			agentkit.Usage{},
			nil,
		),
		textRoundTrip("done"),
	)
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{tool}}

	stream := conv.Send(context.Background(), "find it")
	events := drain(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	useIndex, resultIndex := eventIndexes[agentkit.ToolUse](events), eventIndexes[agentkit.ToolResult](events)
	if useIndex < 0 || resultIndex < 0 || useIndex > resultIndex {
		t.Fatalf("ToolUse/ToolResult indexes = %d/%d, want paired events in order", useIndex, resultIndex)
	}
	use := events[useIndex].(agentkit.ToolUse)
	if use.ID != id || !strictToolUseID.MatchString(use.ID) {
		t.Fatalf("ToolUse.ID = %q, want minted strict-charset ID %q", use.ID, id)
	}
	result := events[resultIndex].(agentkit.ToolResult)
	if result.ID != use.ID || result.Name != use.Name || result.Output != "found" || result.IsError {
		t.Fatalf("ToolResult = %#v, want production result paired to tool use %#v", result, use)
	}

	if len(conv.History) != 4 {
		t.Fatalf("History len = %d, want user, assistant(tool_use), user(tool_result), assistant(final)", len(conv.History))
	}
	resultBlock, ok := conv.History[2].Blocks[0].(agentkit.ToolResultBlock)
	if !ok {
		t.Fatalf("History[2].Blocks[0] = %T, want ToolResultBlock", conv.History[2].Blocks[0])
	}
	if resultBlock.ToolUseID != use.ID || resultBlock.Name != use.Name || resultBlock.Content != result.Output || resultBlock.IsError {
		t.Fatalf("History tool result = %#v, want paired result for tool use %#v", resultBlock, use)
	}
}

func TestConcreteBlocksImplementSealedBlockUnion(t *testing.T) {
	var blocks []agentkit.Block
	blocks = append(blocks,
		agentkit.TextBlock{Text: "visible"},
		agentkit.ToolUseBlock{ID: agentkit.NewToolUseID(), Name: "lookup", Input: json.RawMessage(`{}`)},
		agentkit.ToolResultBlock{ToolUseID: agentkit.NewToolUseID(), Name: "lookup", Content: "ok"},
		agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"signature":"opaque"}`), Summary: "summary"},
	)
	if len(blocks) != 4 {
		t.Fatalf("len(blocks) = %d, want 4", len(blocks))
	}
}
