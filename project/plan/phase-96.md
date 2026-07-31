# Phase 96 — Google adapter parses the whole stream once

*Realizes design Decision 35 (transport-shape independence), the Google slice: R-QUWY-MLCD, R-QW4V-0D32, R-QXCR-E4TR, R-QYKN-RWKG.*

`google/google.go` stops interpreting SSE frames individually. `parseResponse`
decodes the body into frames as it does today, then concatenates every frame's
`candidate.Content.Parts` into one slice and calls `parseParts` **once** over
that slice. Finish reason and usage still fold across frames exactly as now.
Adjacent text parts within the single parses slice merge into one `TextBlock`
(the same rule Phase 97 enforces centrally; realizing it here is what makes the
Google-side ids assertable without depending on that phase).

The end state is that a multi-frame SSE body and a single-frame body carrying
the same content produce byte-identical `Message` values, and that the
end-of-parse `flushPending("")` fires once for the whole stream instead of at
every frame boundary — so a `thoughtSignature` in one frame binds to a
`functionCall` in the next.

`google/google_test.go`'s `writeSSE` helper becomes variadic
(`writeSSE(t *testing.T, w http.ResponseWriter, data ...string)`, one `data: …`
frame written per argument). Its twelve existing single-frame call sites are
unchanged by the signature change and must keep passing.

**Done when:**
- `R-QUWY-MLCD` — a multi-frame body splitting one reply mid-word across frames
  assembles to exactly one `TextBlock` equal to the verbatim concatenation, and
  the same content in a single frame assembles to a byte-identical `Message`.
- `R-QW4V-0D32` — a single frame carrying two adjacent `text` parts assembles to
  one `TextBlock` holding both.
- `R-QXCR-E4TR` — a `thoughtSignature` in one frame and a `functionCall` in a
  later frame produce a `ReasoningBlock` whose `BoundToID` equals that
  `ToolUseBlock`'s minted non-empty `ID`.
- `R-QYKN-RWKG` — text, `functionCall`, text delivered in three separate frames
  assemble to exactly three blocks in that order, the text runs unmerged across
  the tool call.
- Each id above appears verbatim as a `// R-XXXX-XXXX` comment on the test that
  asserts it, in `google/google_test.go`.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
- `grep -c 'data ...string' google/google_test.go` returns at least 1, proving
  the helper can express a multi-frame body.
