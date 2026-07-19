package toolkit

import (
	"reflect"
	"strings"
	"testing"
)

func TestGrepDirectoryReturnsSortedMatchingLines(t *testing.T) {
	// R-MKJG-HKBT
	root := t.TempDir()
	writeTestFile(t, root+"/z.txt", "no\nneedle z\n")
	writeTestFile(t, root+"/a.txt", "needle a1\nno\nneedle a3\n")
	got := callPathList(t, Grep(root), map[string]any{"pattern": "needle ."})
	want := []string{"a.txt:1:needle a1", "a.txt:3:needle a3", "z.txt:2:needle z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Grep = %#v, want %#v", got, want)
	}
}

func TestGrepFileAndBaseNameGlobSelection(t *testing.T) {
	// R-MLRC-VC2I
	root := t.TempDir()
	writeTestFile(t, root+"/one.go", "target\n")
	writeTestFile(t, root+"/two.txt", "target\n")
	got := callPathList(t, Grep(root), map[string]any{"pattern": "target", "path": "two.txt"})
	if want := []string{"two.txt:1:target"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("single-file Grep = %#v, want %#v", got, want)
	}
	got = callPathList(t, Grep(root), map[string]any{"pattern": "target", "glob": "*.go"})
	if want := []string{"one.go:1:target"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("glob-filtered Grep = %#v, want %#v", got, want)
	}
}

func TestGrepSkipsGitDirectories(t *testing.T) {
	// R-MMZ9-93T7
	root := t.TempDir()
	writeTestFile(t, root+"/.git/config", "secret match\n")
	writeTestFile(t, root+"/nested/.git/config", "secret match\n")
	writeTestFile(t, root+"/.github/workflow", "public match\n")
	got := callPathList(t, Grep(root), map[string]any{"pattern": "match"})
	if want := []string{".github/workflow:1:public match"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Grep = %#v, want %#v", got, want)
	}
}

func TestGrepSkipsBinaryFiles(t *testing.T) {
	// R-MO75-MVJW
	root := t.TempDir()
	writeTestFile(t, root+"/binary.dat", "needle\x00more\n")
	writeTestFile(t, root+"/text.txt", "needle\n")
	got := callPathList(t, Grep(root), map[string]any{"pattern": "needle"})
	if want := []string{"text.txt:1:needle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Grep = %#v, want %#v", got, want)
	}
}

func TestGrepInvalidRegexpReturnsError(t *testing.T) {
	// R-MPF2-0NAL
	got, err := callTool(t, Grep(t.TempDir()), map[string]any{"pattern": "[unterminated"})
	if err == nil {
		t.Fatalf("Grep = %q, want regexp error", got)
	}
	if !strings.Contains(err.Error(), "[unterminated") || !strings.Contains(err.Error(), "regexp") {
		t.Fatalf("error = %q, want pattern and regexp context", err)
	}
}
