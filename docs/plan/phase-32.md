# Phase 32 — `ToolSchemaLimiter` capability interface (kill the `"google"` name-dispatch)

*Realizes design Decision 22 (provider-driven tool-schema limits). Depends on Phase 13 (MCP integration into the orchestrator) and Phase 11 (Google adapter).*

Replace the `c.Provider.Name() != "google"` branch in `mcp.go` with an optional capability interface the core discovers by type assertion, and move the Gemini-specific schema-keyword knowledge out of root into the `google` adapter. Root and `google` change together so schema-lossiness warnings never disappear between the two edits.

End state:

- Root defines `ToolSchemaLimiter` (optional interface: `UnsupportedSchemaKeywords(json.RawMessage) []string`, sorted output, empty = full fidelity).
- `mcpSchemaWarnings` type-asserts `c.Provider.(ToolSchemaLimiter)`; when present it asks the provider which keywords are dropped and assembles the `WarnToolSchemaLossy` warning, keeping the `<server>.<originalName>` attribution it already owns. There is no branch on a provider name anywhere in the core.
- `unsupportedGeminiSchemaKeywords` and `collectUnsupportedSchemaKeywords` are removed from root; the `google` adapter implements `ToolSchemaLimiter`, reporting exactly the keyword set (`$ref`, `additionalProperties`, `oneOf`) its `convertSchema` already strips — one source of truth in the adapter.
- Anthropic/OpenAI/Z.ai do not implement the interface and so produce no schema warnings — structurally (interface absent), not by name-exclusion.
- Warning timing is unchanged: warnings still surface at the `Send`/discovery boundary through the existing cached paths, not deferred to a round-trip.

**Done when:** D22's Verification ids are covered by clearly-named tests — R-SKVI-TSZQ (a `ToolSchemaLimiter` provider whose `Name()` is **not** `"google"` still produces a `WarnToolSchemaLossy` naming the keywords and attributing `<server>.<originalName>`, proving interface-not-name dispatch), R-SNBB-LCH4 (a provider not implementing the interface yields no schema warnings; no provider-name branch in the core), R-SOJ7-Z47T (the `google` adapter's `UnsupportedSchemaKeywords` returns exactly the set `convertSchema` strips for a schema using all three, and empty for one using none) — and the full suite is green. These are also `WarnToolSchemaLossy`'s first coverage.
