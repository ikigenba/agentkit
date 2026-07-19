package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/ikigenba/agentkit"
)

const defaultBashTimeout = 120_000

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// Bash returns a tool that runs shell commands with root as its working directory.
func Bash(root string) agentkit.Tool {
	return agentkit.NewTool("Bash", "Run a shell command.", func(ctx context.Context, in bashInput) (string, error) {
		if strings.TrimSpace(in.Command) == "" {
			return "", fmt.Errorf("command must not be blank")
		}
		if in.Timeout < 0 {
			return "", fmt.Errorf("timeout must not be negative")
		}
		timeout := in.Timeout
		if timeout == 0 {
			timeout = defaultBashTimeout
		}

		cmd := exec.Command("bash", "-c", in.Command)
		cmd.Dir = root
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("start bash: %w", err)
		}

		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
		defer timer.Stop()

		select {
		case err := <-done:
			if err == nil {
				return capOutput(output.String()), nil
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return capOutput(appendResultMarker(output.String(), fmt.Sprintf("[exit status %d]", exitErr.ExitCode()))), nil
			}
			return "", fmt.Errorf("wait for bash: %w", err)
		case <-timer.C:
			killProcessGroup(cmd.Process.Pid)
			<-done
			return capOutput(appendResultMarker(output.String(), fmt.Sprintf("[command timed out after %dms]", timeout))), nil
		case <-ctx.Done():
			killProcessGroup(cmd.Process.Pid)
			<-done
			return capOutput(appendResultMarker(output.String(), "[command cancelled]")), nil
		}
	})
}

func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func appendResultMarker(output, marker string) string {
	if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return output + marker
}
