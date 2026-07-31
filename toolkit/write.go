package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ikigenba/agentkit"
)

type writeInput struct {
	FilePath string `json:"file_path" jsonschema:"description=Path of the file to write relative to the tool root."`
	Content  string `json:"content,omitempty" jsonschema:"description=Complete content to write to the file."`
}

// Write returns a tool that creates or replaces files beneath root. It
// currently honors no options.
func Write(root string, _ ...Option) agentkit.Tool {
	return agentkit.NewTool("Write", "Write a file, creating its parent directories when needed.", func(_ context.Context, in writeInput) (string, error) {
		path, err := confinePath(root, in.FilePath)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("create parents for %q: %w", in.FilePath, err)
		}
		if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
			return "", fmt.Errorf("write %q: %w", in.FilePath, err)
		}
		return capOutput(fmt.Sprintf("wrote %s", in.FilePath)), nil
	})
}
