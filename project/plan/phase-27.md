# Phase 27 — Embeddings root surface: the `Embedder` object, the `Embed` verb, the `EmbeddingProvider` SPI, and the registry value types

*Realizes design Decision 18 (embeddings consumer surface), 19 (the `EmbeddingProvider` SPI), and 20 (embedding registry value types) — the root-package slices of each. Depends on Phase 5 (root chat surface), reusing `RetryPolicy` (D11), the root `Cost` and `Warning` types (D16/D2), and the D7 error sentinels by value.*

The root `agentkit` package gains its embeddings surface, parallel to the chat surface but sharing none of its machinery. New root files (e.g. `embed.go`, `embedding.go`) declare:

- **Consumer surface (D18):** the `Embedder` struct (`Provider`/`Model`/`Dimensions`/`Retry` plus unexported cumulative accounting), the `Embed(ctx, inputs, role) (*EmbedResult, error)` verb, `EmbedResult` with `Usage()`/`Cost()`, `Embedder.TotalUsage()`/`TotalCost()`, and the `InputType` enum (`InputUnspecified`/`InputQuery`/`InputDocument`).
- **The SPI (D19):** the exported `EmbeddingProvider` interface (`Embed`/`Name`/`Pricing`), `EmbedRequest`, and `EmbedRoundTrip` with its `Vectors()`/`Usage()`/`Warnings()`/`Err()` accessors.
- **Registry value types (D20):** `EmbeddingUsage`, `EmbeddingPricing` with its `Cost(EmbeddingUsage) Cost` method, `EmbeddingSpec`, and the `EmbeddingInspector` interface.

`Embed` validates the config it owns at its boundary — a missing `Provider`/`Model` and an empty/empty-string batch — then builds an `EmbedRequest`, delegates to `Provider.Embed`, and lifts the returned `EmbedRoundTrip` into an `EmbedResult` while updating cumulative usage/cost. Registry-dependent gates (unknown-model, dimension-producible, over-length, normalization, chunking) are adapter-owned and land in Phases 28–29; root cannot resolve a per-model spec because it must not import a sub-package. The phase is exercised end-to-end against an in-package **fake `EmbeddingProvider`** — no network, no real adapter — and leaves the suite green.

**Done when:** these root-slice ids are covered by clearly-named tests and the suite is green —
- R-Y87O-NUL7 — `Embed` on an `Embedder` missing `Provider` or `Model` returns `ErrInvalidConfig` (`errors.Is`) and issues no provider call. *(The unknown-model clause of this id is discharged by the adapter phases.)*
- R-Y9FL-1MBW — `Embed` with a nil/empty `inputs` slice, or a batch containing an empty string, returns `ErrInvalidInput` (`errors.Is`) and issues no provider call.
- R-YFJ2-YH1D — `EmbedResult.Usage()`/`Cost()` report this call's figures and `Embedder.TotalUsage()`/`TotalCost()` equal the running sum across successful `Embed` calls (asserted against the fake provider).
- R-YQI6-EEPM — `EmbeddingPricing.Cost(u)` equals `u.InputTokens × InputToken` in integer nano-USD, and `EmbedResult.Cost()`/`Embedder.TotalCost()` derive from it.

The `EmbeddingProvider`, `EmbedRequest`/`EmbedRoundTrip`, `EmbeddingSpec`, and `EmbeddingInspector` types this phase declares carry no behavior of their own here; their remaining D18/D19/D20 ids are fully realized by the OpenAI (Phase 28) and Google (Phase 29) adapters.
