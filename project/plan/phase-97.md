# Phase 97 — The adjacent-text invariant, enforced at the SPI boundary

*Realizes design Decision 35 (transport-shape independence), the core slice: R-QZSK-5OB5. Depends on Phase 96.*

Root `orchestration.go` gains an unexported `mergeAdjacentText(blocks []Block)
[]Block` that replaces each maximal run of adjacent `TextBlock`s with a single
`TextBlock` holding their verbatim concatenation (no separator, no trimming),
passing every non-text block through untouched and in order.
`agentkit.NewRoundTrip` applies it to the `Message` it stores, alongside the
clone it already performs.

Because `NewRoundTrip` is the single constructor every adapter uses
(`anthropic`, `google`, `openai`, `internal/openaicompat`, and root itself), the
invariant then holds for every current and future adapter without each one
restating it. Anthropic, OpenAI, and the compat core already satisfy it by
construction; this phase is what makes that a guarantee rather than a
coincidence, and what stops a future adapter from reintroducing the defect
Phase 96 fixed.

**Done when:**
- `R-QZSK-5OB5` — a `Message` built with adjacent `TextBlock`s emerges from
  `RoundTrip.Message()` with them joined verbatim into one, and a `Message`
  whose text blocks are separated by a `ToolUseBlock` or a `ReasoningBlock`
  emerges with both text blocks intact and in original order. Asserted directly
  against `NewRoundTrip`, independently of any provider parse.
- The id appears verbatim as a `// R-QZSK-5OB5` comment on the test asserting
  it, in a root-package `*_test.go` file.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
