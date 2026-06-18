package agentkit

import (
	"encoding/json"
	"errors"
	"time"
)

// LogRecord is one JSONL protocol record emitted by Conversation.Log.
type LogRecord struct {
	Type     string      `json:"type"`
	Time     time.Time   `json:"time"`
	Seq      int         `json:"seq"`
	Message  *Message    `json:"message,omitempty"`
	ToolUse  *ToolUse    `json:"tool_use,omitempty"`
	Result   *ToolResult `json:"tool_result,omitempty"`
	Usage    *Usage      `json:"usage,omitempty"`
	Warning  *Warning    `json:"warning,omitempty"`
	Error    *Error      `json:"error,omitempty"`
	Turns    int         `json:"turns,omitempty"`
	Cost     *Cost       `json:"cost,omitempty"`
	Provider string      `json:"provider,omitempty"`
	Model    string      `json:"model,omitempty"`
	Status   string      `json:"status,omitempty"`
}

func (s *Stream) log(c *Conversation, record LogRecord) {
	if c == nil || c.Log == nil {
		return
	}
	s.logSeq++
	record.Time = c.logNow()
	record.Seq = s.logSeq
	_ = json.NewEncoder(c.Log).Encode(record)
}

func (s *Stream) logError(c *Conversation, recordType string, err error) {
	var providerErr *Error
	record := LogRecord{Type: recordType}
	if errors.As(err, &providerErr) {
		record.Error = providerErr
	}
	s.log(c, record)
}

func (c *Conversation) logNow() time.Time {
	if c != nil && c.retryClock != nil {
		return c.retryClock.Now()
	}
	return time.Now()
}
