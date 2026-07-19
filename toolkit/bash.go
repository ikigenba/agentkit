package toolkit

import (
	"context"
	"fmt"

	"github.com/ikigenba/agentkit"
)

type bashInput struct{}

// Bash returns the standard shell tool. Command execution is added in Phase 71.
func Bash(_ string) agentkit.Tool {
	return agentkit.NewTool("Bash", "Run a shell command.", func(_ context.Context, _ bashInput) (string, error) {
		return "", fmt.Errorf("Bash is not implemented")
	})
}
