package agentkit

import (
	"crypto/rand"
	"encoding/base64"
	"sync/atomic"
)

var toolUseIDFallback uint64

// NewToolUseID mints a neutral tool-call ID in Anthropic's strict charset.
func NewToolUseID() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "ak_" + base64.RawURLEncoding.EncodeToString(b[:])
	}

	n := atomic.AddUint64(&toolUseIDFallback, 1)
	var fallback [8]byte
	for i := len(fallback) - 1; i >= 0; i-- {
		fallback[i] = byte(n)
		n >>= 8
	}
	return "ak_" + base64.RawURLEncoding.EncodeToString(fallback[:])
}
