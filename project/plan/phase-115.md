# Phase 115 — Changelog for v0.17.0

*Realizes design Decision — (structural; the release note for the work built in Phases 112 to 114). Depends on Phase 114.*

`CHANGELOG.md` gains a `## v0.17.0` section at the top, above `## v0.16.0`, describing the change in consumer-facing terms: tools are no longer sent with a strict flag on any provider, AgentKit now validates tool arguments against the tool's declared schema before invoking it and feeds an error result back to the model on a violation, optional properties are rendered as optional on OpenAI rather than as required-and-nullable, and a conversation carrying a large tool inventory no longer fails against Anthropic. No exported API changes, so this is a minor bump rather than a major one.

Cutting the annotated tag is an operator action, not part of this phase: per `AGENTS.md`, releases are `git tag -a vX.Y.Z -m "vX.Y.Z"` on `main`, and no phase creates a tag.

**Done when:** `go build ./...` and `go test ./...` both exit 0 with no failing package, and `grep -c '^## v0.17.0' CHANGELOG.md` returns exactly `1`.
