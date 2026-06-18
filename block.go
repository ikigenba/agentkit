package agentkit

import "encoding/json"

// Role is the author of a message in canonical form.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one turn: an author plus an ordered list of content blocks.
type Message struct {
	Role   Role
	Blocks []Block
}

// Block is one piece of a message.
//
// The union is sealed: only this package can add implementations because
// isBlock is unexported.
type Block interface {
	isBlock()
}

// TextBlock is visible text content.
type TextBlock struct {
	Text string
}

// ToolUseBlock is the model asking to run a tool.
//
// ID is AgentKit-minted in Anthropic's strict charset. Name is carried
// alongside so history stays portable across a provider switch.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResultBlock is the result AgentKit fed back for a ToolUseBlock.
type ToolResultBlock struct {
	ToolUseID string
	Name      string
	Content   string
	IsError   bool
}

// ReasoningBlock preserves a model's reasoning output for verbatim replay on
// the next tool-loop turn.
type ReasoningBlock struct {
	Opaque    json.RawMessage
	Summary   string
	BoundToID string
}

func (TextBlock) isBlock()       {}
func (ToolUseBlock) isBlock()    {}
func (ToolResultBlock) isBlock() {}
func (ReasoningBlock) isBlock()  {}
