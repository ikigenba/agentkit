package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditReplacesUniqueOccurrence(t *testing.T) {
	// R-M3GV-4RY3
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	before := "prefix old suffix\nuntouched"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := callTool(t, Edit(root), map[string]any{"file_path": "file.txt", "old_string": "old", "new_string": "new"}); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "prefix new suffix\nuntouched")
}

func TestEditRejectsMissingOccurrenceWithoutChanges(t *testing.T) {
	// R-M4OR-IJOS
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	before := "original"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := callTool(t, Edit(root), map[string]any{"file_path": "file.txt", "old_string": "absent", "new_string": "new"}); err == nil {
		t.Fatal("Edit succeeded with a missing old_string")
	}
	assertFileContent(t, path, before)
}

func TestEditRejectsAmbiguousOccurrenceWithoutChanges(t *testing.T) {
	// R-M5WN-WBFH
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	before := "old middle old"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := callTool(t, Edit(root), map[string]any{"file_path": "file.txt", "old_string": "old", "new_string": "new"}); err == nil {
		t.Fatal("Edit succeeded with an ambiguous old_string")
	}
	assertFileContent(t, path, before)
}

func TestEditReplaceAllReportsCount(t *testing.T) {
	// R-M74K-A366
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("old middle old"), 0o644); err != nil {
		t.Fatal(err)
	}
	confirmation, err := callTool(t, Edit(root), map[string]any{"file_path": "file.txt", "old_string": "old", "new_string": "new", "replace_all": true})
	if err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "new middle new")
	if !strings.Contains(confirmation, "2") {
		t.Fatalf("confirmation = %q, want occurrence count 2", confirmation)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("file content = %q, want %q", got, want)
	}
}
