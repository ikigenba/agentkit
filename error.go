package agentkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Sentinel categories classify provider failures. Branch with errors.Is,
// never by string-matching Error.Message.
var (
	ErrAuthentication = errors.New("agentkit: authentication")
	ErrPermission     = errors.New("agentkit: permission")
	ErrInvalidRequest = errors.New("agentkit: invalid request")
	ErrNotFound       = errors.New("agentkit: not found")
	ErrRateLimited    = errors.New("agentkit: rate limited")
	ErrOverloaded     = errors.New("agentkit: overloaded")
	ErrServerError    = errors.New("agentkit: server error")
	ErrTimeout        = errors.New("agentkit: timeout")
	ErrNetwork        = errors.New("agentkit: network")
	ErrContextLength  = errors.New("agentkit: context length exceeded")
	ErrContentFilter  = errors.New("agentkit: content filtered")
	ErrBilling        = errors.New("agentkit: billing")
	ErrUnknown        = errors.New("agentkit: unknown")
)

// Error is the uniform provider error. Branch on Category via errors.Is;
// inspect provider details and the raw body via errors.As.
type Error struct {
	Category   error
	Provider   string
	MCPServer  string
	StatusCode int
	Type       string
	Message    string
	RequestID  string
	RetryAfter time.Duration
	Raw        json.RawMessage
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	subject := e.Provider
	if subject == "" {
		subject = e.MCPServer
	}

	detail := e.Message
	if detail == "" && e.Err != nil {
		detail = e.Err.Error()
	}
	if detail == "" && e.Category != nil {
		detail = e.Category.Error()
	}
	if detail == "" {
		detail = "provider error"
	}

	if subject == "" && e.StatusCode == 0 && e.Type == "" {
		return detail
	}
	if subject == "" {
		return fmt.Sprintf("%s (status=%d type=%s)", detail, e.StatusCode, e.Type)
	}
	if e.StatusCode == 0 && e.Type == "" {
		return fmt.Sprintf("%s: %s", subject, detail)
	}
	return fmt.Sprintf("%s: %s (status=%d type=%s)", subject, detail, e.StatusCode, e.Type)
}

func (e *Error) Is(target error) bool {
	return e != nil && target == e.Category
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
