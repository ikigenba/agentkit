# Phase 31 — Shared `internal/retry` executor (de-duplicate the four retry copies)

*Realizes design Decision 21 (the shared retry executor). Depends on Phase 06 (D11 retry policy), Phase 13 (MCP integration), and Phases 28–29 (the OpenAI and Google embedding adapters).*

Introduce one leaf package `internal/retry` and route every retrying call site through it, deleting the three duplicate copies and the fourth hand-rolled loop. The package imports only the standard library (it must not import root, which would cycle — root calls the executor for the chat and MCP loops); the D7 retryable-set and `*Error.RetryAfter` reading stay in `agentkit` and are injected as the `classify` function.

End state:

- `internal/retry` exists with `Do[T]`, the `Clock` interface, `RealClock`, `Policy`, and `Decision` (per D21), applying the defaults `4 / 500ms / 30s` against zero `Policy` fields inside `Do`.
- The four call sites are thin `retry.Do` calls: chat `roundTripWithRetry` and `mcp.go discoverMCPTools` (root), and the `internal/openaicompat` and `google` embedding chunk loops. Chat passes an `onRetry` that writes its existing "retry" log record; MCP's `attempt` maps via `mcpError(serverName, …)`; embeddings pass `onRetry: nil`.
- The duplicates are gone: root's `retry.go` clock/defaults/backoff helpers, `internal/openaicompat`'s `embeddingRetry*` helpers and its exported `EmbeddingRetryClock`, and `google`'s `embeddingRetryClock`/`realEmbeddingRetryClock`/`embedding*` delay helpers.
- `Conversation.retryClock` is retyped to `retry.Clock`; `log.go`'s `logNow()` still reads `.Now()` off it (logging does not move into `internal/retry`). The three duplicate fake clocks across the test suites collapse onto one `retry.Clock` fake.
- No behavior changes at any call site: D11's, D17's, and D19's end-to-end retry ids (R-P3LQ-QY2X, R-P4TN-4PTM, R-P61J-IHKB, R-Y878-6UDJ, R-P79F-W9B0, R-P8HC-A11P, R-6XDZ-VW6L, R-6YLW-9NXA, R-YO2D-MV88) still pass unchanged.

**Done when:** D21's Verification ids are covered by clearly-named `internal/retry` unit tests driven by a fake clock — R-IUBG-95CC (full-jitter backoff caps and honored/ignored `RetryAfter`), R-IWR9-0OTQ (stops at `MaxAttempts`; non-retryable returns with zero sleeps), R-IXZ5-EGKF (`MaxElapsed` budget stops retrying; zero means no cap), R-IZ71-S8B4 (ctx cancellation during a sleep returns the context error), R-J0EY-601T (zero policy fields default to 4 / 500ms / 30s) — and the full suite is green.
