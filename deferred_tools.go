package agentkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const loadToolsName = "load_tools"

// DeferredToolGroup is a set of tools advertised through load_tools instead of
// being sent to the provider immediately.
type DeferredToolGroup struct {
	Name  string
	Blurb string
	Tools []Tool
}

type loadToolsTool struct {
	description string
}

func (t loadToolsTool) Name() string {
	return loadToolsName
}

func (t loadToolsTool) Description() string {
	return t.description
}

func (t loadToolsTool) JSONSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tools":{"type":"array","items":{"type":"string"}}},"required":["tools"],"additionalProperties":false}`)
}

func (t loadToolsTool) Call(context.Context, json.RawMessage) (string, error) {
	return "", errors.New("load_tools is handled by orchestration")
}

func (t loadToolsTool) isTool() {}

func (c *Conversation) resolveDeferredTools(base []Tool) ([]Tool, error) {
	if len(c.DeferredTools) == 0 {
		return validateAndSortTools(base)
	}
	catalog, err := c.deferredToolCatalog(base)
	if err != nil {
		return nil, err
	}

	baseWithLoader := append(append([]Tool(nil), base...), loadToolsTool{description: loadToolsDescription(c.DeferredTools)})
	sortedBase, err := validateAndSortTools(baseWithLoader)
	if err != nil {
		return nil, err
	}

	tools := append([]Tool(nil), sortedBase...)
	loaded := make(map[string]struct{}, len(c.loadedDeferredNames))
	for _, name := range c.loadedDeferredNames {
		if _, ok := loaded[name]; ok {
			continue
		}
		tool, ok := catalog[name]
		if !ok {
			continue
		}
		loaded[name] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, nil
}

func (c *Conversation) deferredToolCatalog(base []Tool) (map[string]Tool, error) {
	if len(c.DeferredTools) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(base)+1)
	for _, tool := range base {
		if tool == nil || tool.Name() == "" || !validJSONSchema(tool.JSONSchema()) {
			return nil, ErrInvalidConfig
		}
		name := tool.Name()
		if name == loadToolsName {
			return nil, ErrInvalidConfig
		}
		if _, ok := seen[name]; ok {
			return nil, ErrInvalidConfig
		}
		seen[name] = struct{}{}
	}
	seen[loadToolsName] = struct{}{}

	catalog := make(map[string]Tool)
	for _, group := range c.DeferredTools {
		if group.Name == "" {
			return nil, ErrInvalidConfig
		}
		for _, tool := range group.Tools {
			if tool == nil || tool.Name() == "" || !validJSONSchema(tool.JSONSchema()) {
				return nil, ErrInvalidConfig
			}
			name := tool.Name()
			if _, ok := seen[name]; ok {
				return nil, ErrInvalidConfig
			}
			seen[name] = struct{}{}
			catalog[name] = tool
		}
	}
	return catalog, nil
}

func loadToolsDescription(groups []DeferredToolGroup) string {
	var b strings.Builder
	b.WriteString("Load deferred tools by name so they are available for later tool calls. Available groups:")
	for _, group := range groups {
		names := make([]string, 0, len(group.Tools))
		for _, tool := range group.Tools {
			if tool != nil {
				names = append(names, tool.Name())
			}
		}
		sort.Strings(names)
		b.WriteString("\n- ")
		b.WriteString(group.Name)
		if group.Blurb != "" {
			b.WriteString(": ")
			b.WriteString(group.Blurb)
		}
		if len(names) > 0 {
			b.WriteString(" (tools: ")
			b.WriteString(strings.Join(names, ", "))
			b.WriteString(")")
		}
	}
	return b.String()
}

func (c *Conversation) runLoadTools(_ context.Context, catalog map[string]Tool, use ToolUseBlock) (ToolResultBlock, []Tool, error) {
	names, err := parseLoadToolNames(use.Input)
	if err != nil {
		return ToolResultBlock{
			ToolUseID: use.ID,
			Name:      use.Name,
			Content:   err.Error(),
			IsError:   true,
		}, nil, nil
	}

	newlyLoaded, loaded, unknown := c.loadDeferredTools(catalog, names)
	content, err := marshalLoadToolsResult(loaded, unknown)
	if err != nil {
		return ToolResultBlock{}, nil, err
	}
	return ToolResultBlock{
		ToolUseID: use.ID,
		Name:      use.Name,
		Content:   content,
		IsError:   len(unknown) > 0,
	}, newlyLoaded, nil
}

func (c *Conversation) runDeferredToolMiss(ctx context.Context, catalog map[string]Tool, use ToolUseBlock) (ToolResultBlock, []Tool, error) {
	if _, ok := catalog[use.Name]; !ok {
		result, err := runTool(ctx, nil, use)
		return result, nil, err
	}

	newlyLoaded, _, _ := c.loadDeferredTools(catalog, []string{use.Name})
	return ToolResultBlock{
		ToolUseID: use.ID,
		Name:      use.Name,
		Content:   fmt.Sprintf("tool %q is deferred; call %s before using it", use.Name, loadToolsName),
		IsError:   true,
	}, newlyLoaded, nil
}

func (c *Conversation) loadDeferredTools(catalog map[string]Tool, names []string) ([]Tool, []Tool, []string) {
	loadedSet := make(map[string]struct{}, len(c.loadedDeferredNames))
	for _, name := range c.loadedDeferredNames {
		loadedSet[name] = struct{}{}
	}

	var newlyLoaded []Tool
	var loaded []Tool
	var unknown []string
	for _, name := range names {
		tool, ok := catalog[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		loaded = append(loaded, tool)
		if _, ok := loadedSet[name]; ok {
			continue
		}
		loadedSet[name] = struct{}{}
		c.loadedDeferredNames = append(c.loadedDeferredNames, name)
		newlyLoaded = append(newlyLoaded, tool)
	}
	return newlyLoaded, loaded, unknown
}

func parseLoadToolNames(input json.RawMessage) ([]string, error) {
	var direct []string
	if err := json.Unmarshal(input, &direct); err == nil {
		if len(direct) == 0 {
			return nil, errors.New("load_tools requires at least one tool name")
		}
		return direct, nil
	}

	var payload struct {
		Tools     []string `json:"tools"`
		Names     []string `json:"names"`
		ToolNames []string `json:"tool_names"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return nil, fmt.Errorf("invalid load_tools input: %w", err)
	}

	names := payload.Tools
	if names == nil {
		names = payload.Names
	}
	if names == nil {
		names = payload.ToolNames
	}
	if len(names) == 0 {
		return nil, errors.New("load_tools requires at least one tool name")
	}
	return names, nil
}

type loadToolsResult struct {
	Loaded  []loadedToolResult `json:"loaded"`
	Unknown []string           `json:"unknown,omitempty"`
}

type loadedToolResult struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func marshalLoadToolsResult(loaded []Tool, unknown []string) (string, error) {
	result := loadToolsResult{
		Loaded:  make([]loadedToolResult, 0, len(loaded)),
		Unknown: append([]string(nil), unknown...),
	}
	for _, tool := range loaded {
		result.Loaded = append(result.Loaded, loadedToolResult{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.JSONSchema(),
		})
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
