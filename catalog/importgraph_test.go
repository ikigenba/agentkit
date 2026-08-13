package catalog

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRequestPathImportGraphExcludesCatalog(t *testing.T) {
	// R-DR92-OIN3
	const catalogPath = "github.com/ikigenba/agentkit/catalog"
	roots := []string{
		"github.com/ikigenba/agentkit",
		"github.com/ikigenba/agentkit/anthropic",
		"github.com/ikigenba/agentkit/google",
		"github.com/ikigenba/agentkit/openai",
		"github.com/ikigenba/agentkit/openai/subscription",
		"github.com/ikigenba/agentkit/openrouter",
		"github.com/ikigenba/agentkit/xai",
		"github.com/ikigenba/agentkit/xai/subscription",
		"github.com/ikigenba/agentkit/zai",
	}
	for _, root := range roots {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", root)
		cmd.Dir = ".."
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list import graph for %s: %v", root, err)
		}
		for _, imported := range strings.Fields(string(out)) {
			if imported == catalogPath {
				t.Errorf("request-path package %s imports %s", root, catalogPath)
			}
		}
	}
}
