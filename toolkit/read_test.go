package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSchemaDescribesPropertiesAndRequiresFilePath(t *testing.T) {
	// R-Y446-A6MP
	assertToolSchema(t, Read(t.TempDir()), []string{"file_path"})
}

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

func TestReadRejectsPDFWithDetectedContentType(t *testing.T) {
	// R-VGH5-GE0Q
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "document.pdf"), []byte("%PDF-1.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := callTool(t, Read(root), map[string]any{"file_path": "document.pdf"})
	if err == nil {
		t.Fatalf("Read = %q, want error", got)
	}
	if got != "" {
		t.Fatalf("Read returned file content %q with error", got)
	}
	if !strings.Contains(err.Error(), "application/pdf") {
		t.Fatalf("error = %q, want detected type application/pdf", err)
	}
}

func TestReadRejectsImagesWithDetectedContentType(t *testing.T) {
	// R-VHP1-U5RF
	tests := []struct {
		name        string
		fileName    string
		content     []byte
		contentType string
	}{
		{
			name:        "PNG",
			fileName:    "image.png",
			content:     []byte("\x89PNG\r\n\x1a\n"),
			contentType: "image/png",
		},
		{
			name:        "JPEG",
			fileName:    "image.jpg",
			content:     []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"),
			contentType: "image/jpeg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, tt.fileName), tt.content, 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := callTool(t, Read(root), map[string]any{"file_path": tt.fileName})
			if err == nil {
				t.Fatalf("Read = %q, want error", got)
			}
			if got != "" {
				t.Fatalf("Read returned file content %q with error", got)
			}
			if !strings.Contains(err.Error(), tt.contentType) {
				t.Fatalf("error = %q, want detected type %s", err, tt.contentType)
			}
		})
	}
}

func TestReadAcceptsExtensionlessUTF8Text(t *testing.T) {
	// R-VIWY-7XI4
	root := t.TempDir()
	want := "extensionless UTF-8: café\n日本語\n"
	if err := os.WriteFile(filepath.Join(root, "README"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := callTool(t, Read(root), map[string]any{"file_path": "README"})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Read = %q, want %q", got, want)
	}
}

func TestReadRejectsBinaryContentWithTextExtension(t *testing.T) {
	// R-VK4U-LP8T
	root := t.TempDir()
	content := []byte("\x89PNG\r\n\x1a\n")
	if err := os.WriteFile(filepath.Join(root, "not-text.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := callTool(t, Read(root), map[string]any{"file_path": "not-text.txt"})
	if err == nil {
		t.Fatalf("Read = %q, want error", got)
	}
	if got != "" {
		t.Fatalf("Read returned binary content %q with error", got)
	}
	if !strings.Contains(err.Error(), "image/png") {
		t.Fatalf("error = %q, want detected type image/png", err)
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
