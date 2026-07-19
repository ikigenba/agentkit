// Package toolkit provides standard tools for local coding agents.
//
// File tools confine their paths to the root supplied to their constructor.
// This confinement protects against accidental filesystem access; it is not a
// security sandbox. In particular, Bash runs with root as its working
// directory but does not restrict what commands can access.
package toolkit

import "github.com/ikigenba/agentkit"

const maxOutputCharacters = 30_000

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
