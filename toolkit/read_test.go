package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadReturnsExactContents(t *testing.T) {
	// R-LXDD-7X8M
	root := t.TempDir()
	want := "first\nsecond\n"
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := callTool(t, Read(root), map[string]any{"file_path": "file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Read = %q, want %q", got, want)
	}
}

func TestReadSelectsOneBasedLineRange(t *testing.T) {
	// R-LYL9-LOZB
	root := t.TempDir()
	content := "one\ntwo\nthree\nfour\nfive\n"
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := callTool(t, Read(root), map[string]any{"file_path": "file.txt", "offset": 3, "limit": 2})
	if err != nil {
		t.Fatal(err)
	}
	if got != "three\nfour\n" {
		t.Fatalf("selected lines = %q, want %q", got, "three\nfour\n")
	}
	got, err = callTool(t, Read(root), map[string]any{"file_path": "file.txt", "offset": 9})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("offset past EOF = %q, want empty", got)
	}
}

func TestReadMissingFileReturnsError(t *testing.T) {
	// R-LZT5-ZGQ0
	got, err := callTool(t, Read(t.TempDir()), map[string]any{"file_path": "missing.txt"})
	if err == nil {
		t.Fatalf("Read = %q, want error", got)
	}
}

func TestReadCapsLongOutputAndMarksTruncation(t *testing.T) {
	// R-LW5G-U5HX
	root := t.TempDir()
	long := strings.Repeat("x", maxOutputCharacters+17)
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := callTool(t, Read(root), map[string]any{"file_path": "long.txt"})
	if err != nil {
		t.Fatal(err)
	}
	wantMarker := "[output truncated: showing first 30000 of 30017 characters]"
	if !strings.HasPrefix(got, strings.Repeat("x", maxOutputCharacters)) || !strings.HasSuffix(got, wantMarker) {
		t.Fatalf("capped output did not contain expected prefix and marker")
	}
	exact := strings.Repeat("y", maxOutputCharacters)
	if err := os.WriteFile(filepath.Join(root, "exact.txt"), []byte(exact), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = callTool(t, Read(root), map[string]any{"file_path": "exact.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got != exact {
		t.Fatal("content at cap was changed or marked")
	}
}
