# Phase 90 — The canonical subset gate, and the end of the residue machinery

*Realizes design Decision 34 (canonical tool-schema subset) and a slice of Decision 22 (the removal of the residue path: R-XWSR-ZK6J). Depends on Phase 89.*

Root gains `validateToolSchema(json.RawMessage) error`, the single authority on subset membership, and the `Send` boundary enforces it over the whole tool universe — eager `Tools`, MCP tools, `load_tools`, and every deferred group's tools — replacing the old parseability-only `validJSONSchema`. A violation fails the entire `Send` with `ErrInvalidConfig`, `History` untouched and no provider call, with error text naming the offending construct and its location; an MCP tool's violation is attributed `<server>.<originalName>`.

The residue machinery is deleted in the same phase, since it is what the gate replaces: the `ToolSchemaTranslator` interface, the `WarnToolSchemaLossy` code, `toolSchemaWarnings`, and `google`'s `UntranslatableSchemaConstructs`/`untranslatableSchemaConstructs`. `WarningCode` keeps `WarnToolChoiceForced` and `WarnCostUnknown`. Tests for the deleted ids (`R-SX1B-XRK2`, `R-X2NE-SE3E`, `R-X3VB-65U3`, `R-6ZTS-NFNZ`, `R-SKVI-TSZQ`, `R-SNBB-LCH4`, `R-SOJ7-Z47T`, `R-9QWF-E6VI`, `R-9S4B-RYM7`, `R-9TC8-5QCW`, `R-9UK4-JI3L`) are deleted with them.

Google's `convertSchema` is left untouched here; Phase 93 rewrites it.

**Done when:**
- `R-XLTO-JMIA` — an out-of-subset construct (`minimum`, `not`, `allOf`, `patternProperties`, or an authored `additionalProperties`) surfaces `ErrInvalidConfig` via `errors.Is` through the `Stream`, leaves `History` unchanged, issues no provider call, and names the construct and its location; a subset-only schema passes.
- `R-XRX6-GH7R` — the identical out-of-subset schema is rejected under every provider, including one whose own dialect accepts the construct.
- `R-XN1K-XE8Z` — a schema carrying a schema-valued `additionalProperties`, supplied from a source that bypasses `NewTool` (MCP tool or test-only helper), is rejected at the gate naming that construct.
- `R-XO9H-B5ZO` — a schema carrying a recursive `$ref`, supplied from a source that bypasses `NewTool`, is rejected at the gate naming the recursive reference, with no dangling `$ref` emitted.
- `R-XQPA-2PH2` — an MCP tool whose server schema leaves the subset fails the whole `Send` with `ErrInvalidConfig` attributed `<server>.<tool>`.
- `R-U3C5-4A1V` — a `format` outside the nine-value allowlist (e.g. `uri`) is rejected at the gate naming the format; all nine allowlisted values pass.
- `R-6QNT-WR7C` — a schema with a non-string `enum` value or a non-string `const` is rejected at the gate naming the construct; string-valued forms pass.
- `R-ZPPN-6FV9` — a schema whose root is not `"type": "object"` (a root `anyOf`, or a root `"type": "string"`) is rejected at the gate naming the root shape, even when every construct inside is subset-legal.
- `R-XWSR-ZK6J` — no warning with `Setting == "tool_schema"` is producible under any provider; unrelated warnings still surface.
- `grep -rn "ToolSchemaTranslator\|WarnToolSchemaLossy\|UntranslatableSchemaConstructs" --include='*.go' --exclude-dir=project . | wc -l` reports `0`.
- `go build ./...` and `go test ./...` both exit 0.
