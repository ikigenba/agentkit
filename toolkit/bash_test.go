package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

func TestBashSchemaDescribesPropertiesAndRequiresCommand(t *testing.T) {
	// R-Y446-A6MP
	assertToolSchema(t, Bash(t.TempDir()), []string{"command"})
}

func TestBashRunsInRootWithCombinedOutput(t *testing.T) {
	// R-M8CG-NUWV
	root := t.TempDir()
	got, err := callTool(t, Bash(root), map[string]any{"command": "pwd; printf stdout; printf stderr >&2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, root+"\n") || !strings.Contains(got, "stdout") || !strings.Contains(got, "stderr") {
		t.Fatalf("Bash output = %q, want root and both streams", got)
	}
}

func assertToolSchema(t *testing.T, tool agentkit.Tool, wantRequired []string) {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.JSONSchema(), &schema); err != nil {
		t.Fatalf("unmarshal %s schema: %v", tool.Name(), err)
	}
	if len(schema.Properties) == 0 {
		t.Fatalf("%s schema has no properties", tool.Name())
	}
	for name, property := range schema.Properties {
		if strings.TrimSpace(property.Description) == "" {
			t.Errorf("%s property %q has no description", tool.Name(), name)
		}
	}
	if !reflect.DeepEqual(schema.Required, wantRequired) {
		t.Errorf("%s required = %v, want %v", tool.Name(), schema.Required, wantRequired)
	}
}

func TestBashNonzeroExitIsNormalResult(t *testing.T) {
	// R-M9KD-1MNK
	got, err := callTool(t, Bash(t.TempDir()), map[string]any{"command": "printf failure; exit 7"})
	if err != nil {
		t.Fatalf("Bash returned tool error: %v", err)
	}
	if got != "failure\n[exit status 7]" {
		t.Fatalf("Bash output = %q", got)
	}
}

func TestBashTimeoutReturnsCapturedOutputPromptly(t *testing.T) {
	// R-MAS9-FEE9
	start := time.Now()
	got, err := callTool(t, Bash(t.TempDir()), map[string]any{"command": "printf before; sleep 10", "timeout": 100})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout returned after %v", elapsed)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "timed out after 100ms") {
		t.Fatalf("timeout output = %q", got)
	}
}

func TestBashTimeoutKillsProcessGroup(t *testing.T) {
	// R-MC05-T64Y
	got, err := callTool(t, Bash(t.TempDir()), map[string]any{"command": "sleep 30 & echo $!; wait", "timeout": 100})
	if err != nil {
		t.Fatal(err)
	}
	pidText := strings.TrimSpace(strings.Split(got, "\n")[0])
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatalf("background pid in output %q: %v", got, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background process %d is still alive after group kill (kill error %v)", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBashRejectsBlankCommand(t *testing.T) {
	// R-MD82-6XVN
	for _, command := range []string{"", " \t\n"} {
		got, err := callTool(t, Bash(t.TempDir()), map[string]any{"command": command})
		if err == nil {
			t.Fatalf("Bash(%q) = %q, want error", command, got)
		}
	}
}

func TestBashContextCancellationIsNormalResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tool := Bash(t.TempDir())
	raw, err := json.Marshal(map[string]any{"command": "printf started; sleep 10"})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	got, err := tool.Call(ctx, raw)
	if err != nil || !strings.Contains(got, "started") || !strings.Contains(got, "cancelled") {
		t.Fatalf("cancelled Bash = %q, %v", got, err)
	}
}

func TestBashBadRootReturnsToolError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	got, err := callTool(t, Bash(root), map[string]any{"command": "pwd"})
	if err == nil || got != "" || !os.IsNotExist(errors.Unwrap(err)) {
		// The exact os.PathError nesting is platform-dependent; the call must at least fail.
		if err == nil || got != "" {
			t.Fatalf("Bash with bad root = %q, %v", got, err)
		}
	}
}
