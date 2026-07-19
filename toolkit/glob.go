package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ikigenba/agentkit"
)

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// Glob returns a tool that finds paths beneath root matching a glob pattern.
func Glob(root string) agentkit.Tool {
	return agentkit.NewTool("Glob", "Find files matching a glob pattern.", func(_ context.Context, in globInput) (string, error) {
		if strings.TrimSpace(in.Pattern) == "" {
			return "", fmt.Errorf("pattern must not be blank")
		}
		base, err := confinePath(root, defaultPath(in.Path))
		if err != nil {
			return "", err
		}

		var matches []string
		if !strings.Contains(in.Pattern, "**") {
			matches, err = plainGlob(base, in.Pattern)
		} else {
			matches, err = recursiveGlob(base, filepath.ToSlash(in.Pattern))
		}
		if err != nil {
			return "", fmt.Errorf("glob pattern %q: %w", in.Pattern, err)
		}
		sort.Strings(matches)
		encoded, err := json.Marshal(matches)
		if err != nil {
			return "", fmt.Errorf("encode glob matches: %w", err)
		}
		return capOutput(string(encoded)), nil
	})
}

func plainGlob(base, pattern string) ([]string, error) {
	absMatches, err := filepath.Glob(filepath.Join(base, pattern))
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0, len(absMatches))
	for _, match := range absMatches {
		rel, err := filepath.Rel(base, match)
		if err != nil {
			return nil, err
		}
		matches = append(matches, filepath.ToSlash(rel))
	}
	return matches, nil
}

func recursiveGlob(base, pattern string) ([]string, error) {
	if _, err := path.Match(pattern, "glob-pattern-validation"); err != nil {
		return nil, err
	}
	matches := []string{}
	err := filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(base, current)
		if err != nil {
			return err
		}
		if rel != "." && matchGlobSegments(strings.Split(pattern, "/"), strings.Split(filepath.ToSlash(rel), "/")) {
			matches = append(matches, filepath.ToSlash(rel))
		}
		return nil
	})
	return matches, err
}

func matchGlobSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		return matchGlobSegments(pattern[1:], name) || (len(name) > 0 && matchGlobSegments(pattern, name[1:]))
	}
	if len(name) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], name[0])
	return err == nil && matched && matchGlobSegments(pattern[1:], name[1:])
}

func defaultPath(input string) string {
	if input == "" {
		return "."
	}
	return input
}
