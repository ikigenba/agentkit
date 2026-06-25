# Phase 42 — Exercise unreachable MCP server and cross-server isolation (complete R-6L70-26RN)

*Realizes design Decision 17 (MCP servers as a tool source — the unreachable-server and cross-server-isolation clauses of R-6L70-26RN). Depends on Phase 13 (MCP integration into the orchestrator) and Phase 12 (the internal MCP client).*

Close audit Finding B/`R-6L70-26RN` (`project/audit/findings.md`, section B; sketch in `project/audit/b-section-fixes.md`): the claim is that a server unreachable at the `Send` boundary (or whose handshake/discovery fails) surfaces a uniform classifiable error before any provider call with `History` unchanged, **and one failing server is isolated to its own attribution and does not corrupt other servers' tools**. The existing test (`mcp_integration_test.go:178`) covers a single-server discovery RPC error only; the **unreachable-server** case and the **cross-server isolation** clause are never exercised — every failure test uses a single server.

Production isolates per server: `resolveMCPTools` (mcp.go:116-152) iterates servers, and on the first discovery failure `closeMCP`s and returns the per-server attributed error (`mcpError(serverName, …)`) before any provider call; an unreachable host yields a classifiable `ErrNetwork` attributed to that server.

End state:

- An **unreachable-server** assertion: a `Send` against a server whose endpoint refuses connection (e.g. an `httptest` server captured then closed) surfaces an `*Error` of a transport category attributed to that server's name, with `History` unchanged and zero provider calls. The conversation uses an injected clock and `MaxAttempts: 1` so the retryable network category does not back off against the real clock.
- A **cross-server isolation** assertion: with two servers attached (one healthy, one unreachable), the failure is attributed to the unreachable server's name only and no provider call is made; then, after detaching the failing server, a subsequent `Send` discovers and serves the healthy server's tool normally — demonstrating the healthy server's tools were not corrupted by the other's failure.
- The existing single-server RPC-error attribution assertion is retained.

**Done when:** R-6L70-26RN is covered by a clearly-named test exercising an unreachable server (classifiable, attributed error, History unchanged, no provider call) and cross-server isolation (failure attributed to the bad server only; the healthy server remains usable), the other D17 ids stay green, and the suite is green per design Conventions. No production change is expected; if isolation or attribution is wrong, fix the production code.
