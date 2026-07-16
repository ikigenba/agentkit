# AgentKit

A Go library for driving LLM agents across providers behind one interface. The
root `agentkit` package holds the provider-agnostic core (orchestration, tools,
blocks, streaming); provider-specific clients live in their own subpackages.
Module path: `github.com/ikigenba/agentkit`.

## Layout

- Root `.` — core `agentkit` package: `orchestration.go`, `tool.go`, `block.go`,
  `mcp.go`, `stream.go`, `reasoning.go`, `retry.go`.
- `anthropic/`, `openai/`, `google/`, `zai/` — per-provider client packages.
- `internal/` — shared helpers: `httpx`, `sse`, `retry`, `mcp`, `openaicompat`.
- `project/` — the spec (product/design/plan) the build loop works from.

## Tests

- Unit: `go test ./...`
- Integration (real provider calls, needs API keys): `go test -tags integration ./...`

## Versioning

Versions are annotated git tags only, `vMAJOR.MINOR.PATCH` (e.g. `v0.1.4`) — no
`VERSION` file, no version constant, no `git describe` suffix. Cut a release with
`git tag -a vX.Y.Z -m "vX.Y.Z"` on `main`. Latest is `git tag --sort=-v:refname | head -1`.
