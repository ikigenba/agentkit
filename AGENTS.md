# AgentKit

A Go library for driving LLM agents across providers behind one interface. The
root `agentkit` package holds the provider-agnostic core (orchestration, tools,
blocks, streaming); provider-specific clients live in their own subpackages.
Module path: `github.com/ikigenba/agentkit`.

## How changes are made

Changes go through the spec under `project/`, not direct edits — settle the
spec, then let the build loop realize it. The spec itself is direction-gated:
`project/**` is written only inside an operator-invoked move (the `$open-spec`
→ `$grill-me` → `$seal-spec` arc, or the build loop's completion mutations).
In any other session `project/` is read-only reference — a stale or wrong spec
is a finding to report, not a license to edit, and a settled discussion is not
direction: say what should change and wait. Edit code directly only on
explicit operator instruction. See the `$ikispec` skill for the `project/`
spec contracts and `$ralph` for the unattended build workflow.

## Consumers and breaking changes

Every consumer of this library is first-party software we own. There is no
outside caller, and there never will be. The same holds on the other side of
the MCP seam: every MCP server this library connects to is first-party
software we own — there are no third-party servers, so a server whose tool
schemas are rejected is fixed at the server, never accommodated here.
Breaking changes are therefore not a cost and are never a reason to prefer
one design over another.

Never trade the optimal shape for backwards compatibility, migration effort, a
deprecation path, or less work. If the right design is different from what
exists, change it and fix every call site. Only the optimal solution is on the
table. This applies to the spec under `project/` as much as to the code: a
decision hedged for hypothetical third-party consumers is a finding to report.

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
`VERSION` file, no version constant, no `git describe` suffix. Latest is
`git tag --sort=-v:refname | head -1`.

### Cutting a release

A release is not real until the tag is on `origin` — the tag *is* the release, so
pushing is part of cutting it, not an optional follow-up. Do this only when the
operator asks for a release; never tag on your own initiative. On `main`, with a
clean tree and the suite green (`go test ./...`, and the integration suite
`go test -tags integration ./...` when the change touches provider/catalog wire
behavior):

1. Pick the next version from the latest tag — patch for fixes, minor for
   additive/backward-compatible changes (new models, new exported constants),
   major for breaking changes.
2. Add a `## vX.Y.Z` section at the top of `CHANGELOG.md` (above the previous
   release), summarizing the change; commit it alone as `Changelog for vX.Y.Z`.
3. Tag that commit: `git tag -a vX.Y.Z -m "vX.Y.Z"` (the tag points at the
   changelog commit, mirroring every prior release).
4. Push both the branch and the tag: `git push origin main && git push origin vX.Y.Z`.
   A tag left only on the local machine is not a release.
