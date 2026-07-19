package toolkit

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ikigenba/agentkit"
)

type editInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// Edit returns a tool that replaces exact text in files beneath root.
func Edit(root string) agentkit.Tool {
	return agentkit.NewTool("Edit", "Replace exact text in a file.", func(_ context.Context, in editInput) (string, error) {
		if in.OldString == "" {
			return "", fmt.Errorf("old_string must not be empty")
		}
		path, err := confinePath(root, in.FilePath)
		if err != nil {
			return "", err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %q: %w", in.FilePath, err)
		}
		count := strings.Count(string(content), in.OldString)
		if count == 0 {
			return "", fmt.Errorf("old_string not found in %q", in.FilePath)
		}
		if !in.ReplaceAll && count != 1 {
			return "", fmt.Errorf("old_string occurs %d times in %q; set replace_all to replace every occurrence", count, in.FilePath)
		}
		replacements := 1
		if in.ReplaceAll {
			replacements = count
		}
		updated := strings.Replace(string(content), in.OldString, in.NewString, replacements)
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return "", fmt.Errorf("write %q: %w", in.FilePath, err)
		}
		return capOutput(fmt.Sprintf("edited %s: replaced %d occurrence(s)", in.FilePath, replacements)), nil
	})
}
