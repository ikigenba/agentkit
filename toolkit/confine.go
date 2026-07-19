package toolkit

import (
	"fmt"
	"os"
	"path/filepath"
)

func confinePath(root, input string) (string, error) {
	rootPath := root
	if rootPath == "" {
		var err error
		rootPath, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
	}

	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve root %q: %w", root, err)
	}

	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootPath, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", input, err)
	}
	resolvedCandidate, err := resolveExistingPrefix(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", input, err)
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || startsWithParent(rel) {
		return "", fmt.Errorf("path %q escapes root", input)
	}
	return resolvedCandidate, nil
}

func resolveExistingPrefix(path string) (string, error) {
	current := filepath.Clean(path)
	var missing []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func startsWithParent(path string) bool {
	separator := string(filepath.Separator)
	return len(path) > 3 && path[:3] == ".."+separator
}
