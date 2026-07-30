package toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGlobSchemaDescribesPropertiesAndRequiresPattern(t *testing.T) {
	// R-Y446-A6MP
	assertToolSchema(t, Glob(t.TempDir()), []string{"pattern"})
}

func TestGlobPlainPatternMatchesFilepathGlob(t *testing.T) {
	// R-MEFY-KPMC
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "b.txt"), "")
	writeTestFile(t, filepath.Join(root, "a.txt"), "")
	writeTestFile(t, filepath.Join(root, "c.go"), "")
	got := callPathList(t, Glob(root), map[string]any{"pattern": "*.txt"})
	want := []string{"a.txt", "b.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Glob = %#v, want %#v", got, want)
	}
}

func TestGlobDoubleStarMatchesEveryDepth(t *testing.T) {
	// R-MFNU-YHD1
	root := t.TempDir()
	for _, name := range []string{"base.go", "one/child.go", "one/two/deep.go", "one/no.txt"} {
		writeTestFile(t, filepath.Join(root, name), "")
	}
	got := callPathList(t, Glob(root), map[string]any{"pattern": "**/*.go"})
	want := []string{"base.go", "one/child.go", "one/two/deep.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Glob = %#v, want %#v", got, want)
	}
}

func TestGlobDoubleStarSkipsGitOnly(t *testing.T) {
	// R-MGVR-C93Q
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".git", "hidden.go"), "")
	writeTestFile(t, filepath.Join(root, "nested", ".git", "hidden.go"), "")
	writeTestFile(t, filepath.Join(root, ".github", "workflow.go"), "")
	got := callPathList(t, Glob(root), map[string]any{"pattern": "**/*.go"})
	want := []string{".github/workflow.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Glob = %#v, want %#v", got, want)
	}
}

func TestGlobNoMatchesReturnsEmptyJSONArray(t *testing.T) {
	// R-MJBK-3SL4
	got, err := callTool(t, Glob(t.TempDir()), map[string]any{"pattern": "**/*.absent"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "[]" {
		t.Fatalf("Glob = %q, want []", got)
	}
}

func callPathList(t *testing.T, tool interface {
	Call(context.Context, json.RawMessage) (string, error)
}, input any) []string {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err := json.Unmarshal([]byte(encoded), &paths); err != nil {
		t.Fatalf("decode %q: %v", encoded, err)
	}
	return paths
}

func writeTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
