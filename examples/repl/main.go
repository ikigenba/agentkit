package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/anthropic"
	"github.com/ikigenba/agentkit/google"
	"github.com/ikigenba/agentkit/openai"
	"github.com/ikigenba/agentkit/zai"
)

type providerFactory func(apiKey string) agentkit.Provider

type providerChoice struct {
	envKey  string
	factory providerFactory
}

var providerChoices = map[string]providerChoice{
	"anthropic": {envKey: "ANTHROPIC_API_KEY", factory: func(apiKey string) agentkit.Provider { return anthropic.New(apiKey) }},
	"google":    {envKey: "GOOGLE_API_KEY", factory: func(apiKey string) agentkit.Provider { return google.New(apiKey) }},
	"openai":    {envKey: "OPENAI_API_KEY", factory: func(apiKey string) agentkit.Provider { return openai.New(apiKey) }},
	"zai":       {envKey: "ZAI_API_KEY", factory: func(apiKey string) agentkit.Provider { return zai.New(apiKey) }},
}

type bashInput struct {
	Command string `json:"command" jsonschema_description:"Shell command to run with bash -lc."`
}

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Stdin, os.Stdout, os.Stderr, providerChoices, os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "repl: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, in io.Reader, out, errOut io.Writer, choices map[string]providerChoice, getenv func(string) string) error {
	conv := &agentkit.Conversation{
		Tools: []agentkit.Tool{bashTool()},
	}
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case line == "/quit" || line == "/exit":
			return nil
		case strings.HasPrefix(line, "/model "):
			if err := applyModelCommand(conv, line, choices, getenv); err != nil {
				fmt.Fprintf(errOut, "%v\n", err)
				continue
			}
			fmt.Fprintf(out, "model: %s\n", conv.Model)
			continue
		case strings.HasPrefix(line, "/"):
			fmt.Fprintf(errOut, "unknown command: %s\n", strings.Fields(line)[0])
			continue
		}

		if conv.Provider == nil || conv.Model == "" {
			fmt.Fprintln(errOut, "set a model first with /model <provider>:<name>")
			continue
		}
		if err := sendAndPrint(ctx, conv, line, out); err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
		}
	}
}

func applyModelCommand(conv *agentkit.Conversation, line string, choices map[string]providerChoice, getenv func(string) string) error {
	spec := strings.TrimSpace(strings.TrimPrefix(line, "/model"))
	if spec == "" {
		return errors.New("usage: /model <provider>:<name>")
	}
	providerName, model, ok := strings.Cut(spec, ":")
	if !ok || providerName == "" || model == "" {
		return errors.New("usage: /model <provider>:<name>")
	}

	choice, ok := choices[providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", providerName)
	}
	apiKey := getenv(choice.envKey)
	if apiKey == "" {
		return fmt.Errorf("missing %s", choice.envKey)
	}

	conv.Provider = choice.factory(apiKey)
	conv.Model = model
	return nil
}

func sendAndPrint(ctx context.Context, conv *agentkit.Conversation, userText string, out io.Writer) error {
	stream := conv.Send(ctx, userText)
	for event := range stream.Events() {
		switch event := event.(type) {
		case agentkit.TextDelta:
			fmt.Fprint(out, event.Text)
		case agentkit.ToolUse:
			fmt.Fprintf(out, "\n[%s]\n", event.Name)
		case agentkit.ToolResult:
			if event.IsError {
				fmt.Fprintf(out, "[%s error]\n", event.Name)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return err
	}
	fmt.Fprintln(out)
	return nil
}

func bashTool() agentkit.Tool {
	return agentkit.NewTool("bash", "Run a shell command with bash -lc and return stdout and stderr.", func(ctx context.Context, in bashInput) (string, error) {
		if runtime.GOOS == "windows" {
			return "", errors.New("bash tool requires a Unix-like shell")
		}
		cmd := exec.CommandContext(ctx, "bash", "-lc", in.Command)
		output, err := cmd.CombinedOutput()
		return string(output), err
	})
}
