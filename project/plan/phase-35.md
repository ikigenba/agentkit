# Phase 35 — Observe early-break resource release on a real HTTP body (de-proxy R-CCI4-0UEA)

*Realizes design Decision 2 (the consumption surface — the early-break cleanup claim R-CCI4-0UEA). Depends on Phase 05 (the `Stream`/`Events()` early-break cleanup) and Phase 08 (the real-HTTP fake-server / SSE harness this test is built on).*

Close audit Finding A/`R-CCI4-0UEA` (`project/audit/findings.md`, section A and cross-cutting theme 2): the test (`orchestration_test.go:471`) asserts only that a later `Send` succeeds — which is already `R-Y4JJ-1J5G`'s rollback claim — and never asserts the actual claim that early break **closes the HTTP body without leaking a goroutine**. The synchronous fake provider returns a fully-materialized round-trip with no real body or goroutine to observe, so a stronger assertion against that fake is impossible: the fix needs a **different test substrate**, a real HTTP round-trip.

End state:

- A clearly-named test stands up an `httptest.Server` streaming an SSE turn (reusing the Phase 08 SSE fake-server harness) and drives it through a **real provider adapter** wired to a `Conversation`, so the turn performs a genuine HTTP round-trip with a real response body.
- The adapter's `http.Client` uses an instrumented `http.RoundTripper` that wraps each response body in a close-tracking `io.ReadCloser` recording whether `Close()` was called.
- The turn is constructed so an HTTP body is genuinely open at the moment the consumer breaks out of `Events()` early (e.g. a tool-loop turn broken between round-trips). The test then asserts: every tracked response body was `Close()`d after the early break, and `runtime.NumGoroutine()` returns to its pre-turn baseline (within a short stabilization poll) — i.e. no goroutine leaked. No `go.uber.org/goleak` dependency is added; the goroutine check is a hand-rolled baseline poll and the body-close assertion via the tracking transport is the deterministic core.

**Done when:** R-CCI4-0UEA is covered by a clearly-named test that observes, on a real HTTP turn, the response body being `Close()`d on early break and the goroutine count returning to baseline; the existing `R-Y4JJ-1J5G` rollback assertion is preserved (here or in its own test); the other D2 ids stay green; and the suite is green per design Conventions. If the genuine substrate exposes a real leak (an unclosed body or a stranded goroutine on early break), fix the production code.
