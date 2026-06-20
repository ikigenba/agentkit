# Phase 12 — Internal MCP Streamable-HTTP client
*Realizes design Decision 17 (the MCP client portion), Decision 12 (`internal/mcp`), and Decision 13 (fake MCP server). Depends on Phase 1, Phase 2, and the `internal/sse` helper from Phase 8.*

The non-consumer-importable `internal/mcp` raw-HTTP Streamable-HTTP JSON-RPC client exists, targeting MCP revision `2025-11-25` and implementing exactly the four calls (`initialize`, `notifications/initialized`, `tools/list` paginated to exhaustion, `tools/call`). It reads the JSON-RPC response from whichever of `application/json` or `text/event-stream` arrives (reusing `internal/sse`), echoes any `Mcp-Session-Id` and always sends `MCP-Protocol-Version: <negotiated>`, transparently re-initializes on a `404` for idempotent discovery, and re-establishes (but does not replay) on a `404` mid-`tools/call`. Verified against a fake `httptest` MCP server.

**Done when:** R-711P-17EO, R-6MEW-FYIC, R-6OUP-7HZQ, and R-6RAH-Z1H4 are covered; suite green.

