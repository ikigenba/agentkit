package toolkit

import (
	"context"
	"fmt"

	"github.com/ikigenba/agentkit"
)

type globInput struct{}

// Glob returns the standard file globbing tool. Globbing is added in Phase 71.
func Glob(_ string) agentkit.Tool {
	return agentkit.NewTool("Glob", "Find files matching a glob pattern.", func(_ context.Context, _ globInput) (string, error) {
		return "", fmt.Errorf("Glob is not implemented")
	})
}
