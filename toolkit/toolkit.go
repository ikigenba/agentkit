// Package toolkit provides standard tools for local coding agents.
//
// File tools confine their paths to the root supplied to their constructor.
// This confinement protects against accidental filesystem access; it is not a
// security sandbox. In particular, Bash runs with root as its working
// directory but does not restrict what commands can access.
package toolkit

import (
	"net/http"
	"strings"

	"github.com/ikigenba/agentkit"
)

const maxOutputCharacters = 30_000

type config struct {
	baseURL    string
	httpClient *http.Client
}

// Option adjusts an optional setting on a toolkit tool. Each constructor
// honors the options named in its documentation and ignores the rest.
type Option func(*config)

// WithBaseURL sets the endpoint root used by WebSearch.
func WithBaseURL(baseURL string) Option {
	return func(cfg *config) {
		cfg.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient sets the client used by WebSearch and WebFetch.
func WithHTTPClient(client *http.Client) Option {
	return func(cfg *config) {
		cfg.httpClient = client
	}
}

func toolConfig(defaultBaseURL string, opts ...Option) config {
	cfg := config{baseURL: defaultBaseURL}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// All returns the standard coding tools in their conventional order.
func All(root string) []agentkit.Tool {
	return []agentkit.Tool{
		Bash(root),
		Read(root),
		Write(root),
		Edit(root),
		Glob(root),
		Grep(root),
	}
}
