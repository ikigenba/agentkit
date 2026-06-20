# Phase 7 — Structured JSONL event log and conversation lifecycle
*Realizes design Decision 15 (JSONL event log & lifecycle); completes the one-idiom error proof and cumulative cost. Depends on Phase 5 and Phase 6.*

`Conversation.Log io.Writer`, the `LogRecord` schema, `Close()`, `TotalUsage()`, `TotalCost()`, and the `ErrClosed` sentinel exist. A turn writes one JSONL record per protocol event in stream order (`Time` from the injected clock, `Seq` monotonic), writes are best-effort, `Close()` emits exactly one cumulative `summary` (idempotent) and `Send`-after-`Close` returns `ErrClosed`. With every sentinel family now present (provider, orchestration, boundary, and `ErrClosed`), the `*Error`-versus-bare-sentinel distinction is fully provable.

**Done when:** R-PH7W-BVH0, R-PIFS-PN7P, R-PJNP-3EYE, R-PKVL-H6P3, R-PM3H-UYFS, R-PNBE-8Q6H, R-POJA-MHX6, R-PPR7-09NV, R-PVUO-X4DC (TotalCost + summary), R-I5VJ-CTXE, and R-7CYE-KS40 are covered; suite green.

