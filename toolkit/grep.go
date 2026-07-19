package toolkit

import (
	"context"
	"fmt"

	"github.com/ikigenba/agentkit"
)

type grepInput struct{}

// Grep returns the standard content search tool. Searching is added in Phase 71.
func Grep(_ string) agentkit.Tool {
	return agentkit.NewTool("Grep", "Search file contents for a pattern.", func(_ context.Context, _ grepInput) (string, error) {
		return "", fmt.Errorf("Grep is not implemented")
	})
}
