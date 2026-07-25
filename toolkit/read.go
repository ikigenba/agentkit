package toolkit

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ikigenba/agentkit"
)

type readInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// Read returns a tool that reads files beneath root.
func Read(root string) agentkit.Tool {
	return agentkit.NewTool("Read", "Read a file, optionally selecting a range of lines.", func(_ context.Context, in readInput) (string, error) {
		if in.Offset < 0 || in.Limit < 0 {
			return "", fmt.Errorf("offset and limit must not be negative")
		}
		path, err := confinePath(root, in.FilePath)
		if err != nil {
			return "", err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %q: %w", in.FilePath, err)
		}
		contentType := http.DetectContentType(content)
		if !strings.HasPrefix(contentType, "text/") {
			return "", fmt.Errorf("read %q: detected non-text content type %s", in.FilePath, contentType)
		}
		result := string(content)
		if in.Offset != 0 || in.Limit != 0 {
			result = selectLines(result, in.Offset, in.Limit)
		}
		return capOutput(result), nil
	})
}

func selectLines(content string, offset, limit int) string {
	if content == "" {
		return ""
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := 0
	if offset > 0 {
		start = offset - 1
	}
	if start >= len(lines) {
		return ""
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return strings.Join(lines[start:end], "")
}

func capOutput(output string) string {
	total := utf8.RuneCountInString(output)
	if total <= maxOutputCharacters {
		return output
	}
	runes := []rune(output)
	return string(runes[:maxOutputCharacters]) + fmt.Sprintf("\n[output truncated: showing first %d of %d characters]", maxOutputCharacters, total)
}
