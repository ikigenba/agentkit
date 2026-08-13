package responses

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResponsesCoreIsInternalAndOpenAIImportsIt(t *testing.T) {
	// R-DP0M-QU7F
	root := filepath.Join("..", "..")
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
	}

	type listedPackage struct {
		ImportPath string
		Imports    []string
	}
	decoder := json.NewDecoder(bytes.NewReader(out))
	foundOpenAIImport := false
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		if pkg.ImportPath == "github.com/ikigenba/agentkit/responses" {
			t.Fatal("consumer-importable responses package exists")
		}
		if pkg.ImportPath == "github.com/ikigenba/agentkit/openai" {
			for _, imported := range pkg.Imports {
				if imported == "github.com/ikigenba/agentkit/internal/responses" {
					foundOpenAIImport = true
				}
			}
		}
	}
	if !foundOpenAIImport {
		t.Fatal("openai does not import internal/responses")
	}
}
