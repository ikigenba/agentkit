# Phase 6 — Retry and backoff
*Realizes design Decision 11 (retry & backoff policy). Depends on Phase 5.*

`RetryPolicy` and `Conversation.Retry` exist, and the orchestrator wraps each `RoundTrip` with the single cross-provider retry policy: full-jitter exponential backoff over the fixed retryable category set, per-round-trip budget, the no-retry-after-first-byte streaming-idempotency rule, server `Retry-After` honored (toggleable), context-aware waits, and an injectable unexported clock for deterministic tests. Verified against fake `Provider` doubles that fail N times then succeed, with the injected clock asserting attempt counts and delays.

**Done when:** R-P3LQ-QY2X, R-P4TN-4PTM, R-P61J-IHKB, R-Y878-6UDJ, R-P79F-W9B0, and R-P8HC-A11P are covered; suite green.

