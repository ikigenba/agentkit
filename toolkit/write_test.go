package toolkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesParentsAndExactFile(t *testing.T) {
	// R-M28Y-R07E
	root := t.TempDir()
	_, err := callTool(t, Write(root), map[string]any{"file_path": "missing/parents/file.txt", "content": "exact content"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "missing", "parents", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "exact content" {
		t.Fatalf("written file = %q, want exact content", got)
	}
}
