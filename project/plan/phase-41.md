# Phase 41 — Assert MCP discovery fails fast on non-retryable HTTP (complete R-6XDZ-VW6L)

*Realizes design Decision 11 (retry & backoff policy — the discovery fail-fast clause of R-6XDZ-VW6L). Depends on Phase 13 (MCP integration into the orchestrator) and Phase 06/Phase 31 (the retry policy and the shared `internal/retry` executor that drives discovery).*

Close audit Finding B/`R-6XDZ-VW6L` (`project/audit/findings.md`, section B; sketch in `project/audit/b-section-fixes.md`): the claim is that MCP discovery (`initialize`/`tools/list`) retries transient transport failures under the retry policy **but fails fast on `401`/`403`/`400` and non-MCP `4xx`**. The existing test (`mcp_integration_test.go:301`) covers the retry-on-`500` half (two list calls, one backoff). The fail-fast half is not asserted under this id — the `401`/`403` cases that exist (`R-6Q2L-L9QF`) check only the error category, never that there was **no retry / zero backoff**.

Production already fails fast: discovery retries via `discoverMCPTools` → `internalretry.Do` with `retryDecision`, and `mcpHTTPCategory` maps `401→ErrAuthentication`, `403→ErrPermission`, `400→ErrInvalidRequest` — all outside the retryable set — so `Do` makes exactly one attempt. The behavior is correct; it is simply unasserted with a counting clock. (Use `400` for the "non-MCP 4xx" case; avoid `404`, which the internal client treats specially as session-expiry re-init, not a retry-policy concern.)

End state:

- A clearly-named sub-test (extending the discovery-retry test) returns each fail-fast status — `400`, `401`, `403` — on `tools/list`, with an injected counting clock and `MaxAttempts > 1`.
- For each status the test asserts exactly **one** `tools/list` call and **zero** recorded sleeps, and that `Stream.Err()` carries the expected non-retryable category, with `History` unchanged and no provider call.
- The existing retry-on-`500` assertion (two calls, one backoff) is retained, so the two halves of the claim are exercised side by side under this id.

**Done when:** R-6XDZ-VW6L is covered by a clearly-named test asserting discovery retries transient `5xx` but makes a single attempt with no backoff on `400`/`401`/`403`, the other D11 ids stay green, and the suite is green per design Conventions. No production change is expected; if a fail-fast status is wrongly retried, fix the production code.
