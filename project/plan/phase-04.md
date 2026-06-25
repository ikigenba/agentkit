# Phase 4 — Tool definition and registration surface
*Realizes design Decision 4 (tool definition & registration). Depends on Phase 1.*

The sealed `Tool` interface, the generic `NewTool[In]` constructor (JSON Schema derived once from `In` via `invopop/jsonschema` and cached), and the `RawTool` escape hatch exist; a `Tool`'s `Call` decodes input and invokes the consumer's `fn`, returning the string result. Send-boundary validation (bad `RawTool` schema, duplicate names), the Gemini lossy-schema conversion, and the MCP-schema warning are proven in later phases.

**Done when:** R-WYZP-N2VB, R-X07M-0UM0, and R-X2NE-SE3E are covered (schema reflects the struct and is byte-stable/cached; typed and raw `Call` decode-and-invoke paths); suite green.

