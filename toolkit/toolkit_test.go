package toolkit

import "testing"

func TestAllReturnsStandardToolsInOrder(t *testing.T) {
	// R-LQ1Y-XASG
	want := []string{"Bash", "Read", "Write", "Edit", "Glob", "Grep"}
	tools := All(t.TempDir())
	if len(tools) != len(want) {
		t.Fatalf("All returned %d tools, want %d", len(tools), len(want))
	}
	for i, tool := range tools {
		if got := tool.Name(); got != want[i] {
			t.Errorf("tool %d name = %q, want %q", i, got, want[i])
		}
	}
}

func TestConstructorsDeclareMatchingNames(t *testing.T) {
	// R-LR9V-B2J5
	root := t.TempDir()
	tests := []struct {
		name string
		got  string
	}{
		{"Bash", Bash(root).Name()},
		{"Read", Read(root).Name()},
		{"Write", Write(root).Name()},
		{"Edit", Edit(root).Name()},
		{"Glob", Glob(root).Name()},
		{"Grep", Grep(root).Name()},
	}
	for _, tt := range tests {
		if tt.got != tt.name {
			t.Errorf("%s constructor name = %q", tt.name, tt.got)
		}
	}
}
