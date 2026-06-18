package agentkit_test

import (
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

	use := agentkit.ToolUseBlock{
		ID:    id,
		Name:  "lookup",
		Input: json.RawMessage(`{"q":"agentkit"}`),
	}
	result := agentkit.ToolResultBlock{
		ToolUseID: use.ID,
		Name:      use.Name,
		Content:   "found",
	}
	if result.ToolUseID != use.ID {
		t.Fatalf("ToolResultBlock.ToolUseID = %q, want paired ID %q", result.ToolUseID, use.ID)
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
