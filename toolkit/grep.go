package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ikigenba/agentkit"
)

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Glob    string `json:"glob,omitempty"`
}

// Grep returns a tool that searches files beneath root line by line.
func Grep(root string) agentkit.Tool {
	return agentkit.NewTool("Grep", "Search file contents for a pattern.", func(_ context.Context, in grepInput) (string, error) {
		if strings.TrimSpace(in.Pattern) == "" {
			return "", fmt.Errorf("pattern must not be blank")
		}
		re, err := regexp.Compile(in.Pattern)
		if err != nil {
			return "", fmt.Errorf("invalid regexp %q: %w", in.Pattern, err)
		}
		target, err := confinePath(root, defaultPath(in.Path))
		if err != nil {
			return "", err
		}
		info, err := os.Stat(target)
		if err != nil {
			return "", fmt.Errorf("stat %q: %w", in.Path, err)
		}

		matches := []string{}
		if !info.IsDir() {
			if err := grepFile(target, filepath.Dir(target), re, &matches); err != nil {
				return "", err
			}
		} else {
			if in.Glob != "" {
				if _, err := filepath.Match(in.Glob, "glob-pattern-validation"); err != nil {
					return "", fmt.Errorf("invalid file glob %q: %w", in.Glob, err)
				}
			}
			err = filepath.WalkDir(target, func(current string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() && entry.Name() == ".git" {
					return filepath.SkipDir
				}
				if entry.IsDir() || (in.Glob != "" && !baseMatches(in.Glob, entry.Name())) {
					return nil
				}
				return grepFile(current, target, re, &matches)
			})
			if err != nil {
				return "", fmt.Errorf("search %q: %w", in.Path, err)
			}
		}
		sort.Strings(matches)
		encoded, err := json.Marshal(matches)
		if err != nil {
			return "", fmt.Errorf("encode grep matches: %w", err)
		}
		return capOutput(string(encoded)), nil
	})
}

func baseMatches(pattern, name string) bool {
	matched, _ := filepath.Match(pattern, name)
	return matched
}

func grepFile(filename, base string, re *regexp.Regexp, matches *[]string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read %q: %w", filename, err)
	}
	prefix := content
	if len(prefix) > 8*1024 {
		prefix = prefix[:8*1024]
	}
	if bytes.IndexByte(prefix, 0) >= 0 {
		return nil
	}
	rel, err := filepath.Rel(base, filename)
	if err != nil {
		return fmt.Errorf("make %q relative: %w", filename, err)
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for index, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if re.MatchString(line) {
			*matches = append(*matches, fmt.Sprintf("%s:%d:%s", filepath.ToSlash(rel), index+1, line))
		}
	}
	return nil
}
