# Phase 94 — Deterministic tool ordering across every adapter

*Realizes a slice of design Decision 22 (R-XY0O-DBX8). Depends on Phase 91, Phase 92, and Phase 93.*

Every adapter serializes its tools sorted by name at send time, so the tool block is byte-identical regardless of the order the consumer appended them — preserving the cache prefix (D10). `google` already sorts; this phase makes the guarantee uniform and tested across `anthropic`, `openai`, `internal/openaicompat` (covering `zai` and `openrouter`), and `google` now that all four render rather than pass through.

**Done when:**
- `R-XY0O-DBX8` — for each adapter, two `Send` calls supplying the same tool set in different slice orders produce byte-identical serialized tool payloads.
- `go build ./...` and `go test ./...` both exit 0.
