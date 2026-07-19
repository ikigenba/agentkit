package toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
)

func callTool(t *testing.T, tool agentkit.Tool, input any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Call(context.Background(), raw)
}

func TestFileToolsRejectRelativeEscapeWithoutFilesystemEffect(t *testing.T) {
	// R-LSHR-OU9U
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(parent, "escape.txt")
	if err := os.WriteFile(escape, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		tool  agentkit.Tool
		input any
	}{
		{"Read", Read(root), map[string]any{"file_path": "../escape.txt"}},
		{"Write", Write(root), map[string]any{"file_path": "../escape.txt", "content": "changed"}},
		{"Edit", Edit(root), map[string]any{"file_path": "../escape.txt", "old_string": "outside", "new_string": "changed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := callTool(t, tt.tool, tt.input)
			if err == nil || !strings.Contains(err.Error(), "../escape.txt") {
				t.Fatalf("error = %v, want error naming escaping input", err)
			}
		})
	}
	got, err := os.ReadFile(escape)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "outside" {
		t.Fatalf("outside file = %q, want unchanged", got)
	}
}

func TestFileToolsRejectSymlinkEscape(t *testing.T) {
	// R-LTPO-2M0J
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "file.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		tool  agentkit.Tool
		input any
	}{
		{"Read", Read(root), map[string]any{"file_path": "link/file.txt"}},
		{"Write", Write(root), map[string]any{"file_path": "link/new.txt", "content": "changed"}},
		{"Edit", Edit(root), map[string]any{"file_path": "link/file.txt", "old_string": "outside", "new_string": "changed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := callTool(t, tt.tool, tt.input); err == nil {
				t.Fatal("call succeeded through escaping symlink")
			}
		})
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside new file stat error = %v, want not exist", err)
	}
	got, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "outside" {
		t.Fatalf("outside file = %q, want unchanged", got)
	}
}

func TestEmptyRootUsesWorkingDirectory(t *testing.T) {
	// R-LUXK-GDR8
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("local.txt", []byte("from cwd"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := callTool(t, Read(""), map[string]any{"file_path": "local.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "from cwd" {
		t.Fatalf("Read = %q, want %q", got, "from cwd")
	}
}
