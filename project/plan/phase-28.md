# Phase 28 — OpenAI embeddings adapter (`/v1/embeddings`), the `internal/openaicompat` embeddings variant, and the OpenAI embedding registry

*Realizes design Decision 19 (the `EmbeddingProvider` SPI), 20 (embedding registry), and 18 (consumer surface) — the OpenAI slices of each. Depends on Phase 27 (root embeddings surface), reusing `internal/httpx`, the D7 `Error` taxonomy + classification, the D11 retry executor, and the fake-server test harness established by the chat adapters (Phases 8–10).*

The `openai` sub-package gains an embeddings constructor and registry, with the shared `/v1/embeddings` request shape factored into an embeddings variant of `internal/openaicompat` (parallel to the chat compat helper, never public; its sole v1 consumer is this adapter):

- `openai.NewEmbedder(apiKey, opts...) agentkit.EmbeddingProvider` — bearer auth → `/v1/embeddings`, reusing the existing `WithBaseURL`/`WithHTTPClient` options and credential injection.
- The OpenAI embedding registry co-locating, per model, the `EmbeddingPricing` rate and the `EmbeddingSpec`, with exported constants `EmbedModel3Small` (`text-embedding-3-small`) and `EmbedModel3Large` (`text-embedding-3-large`), plus the credential-blind package-level `Embeddings` inspector value over that registry.
- Adapter-owned guarantees on the OpenAI path: model-validity and dimension-producible gates before any call (Matryoshka `1 .. native`), batch auto-chunk (≤2048 items/call) with order-preserving reassembly and summed usage, client-side L2-normalization, `InputType` ignored on the wire, 400-mapped `ErrContextLength`, and per-chunk D11 retry (no first-byte carve-out).

**Done when:** the OpenAI slice of these ids is covered by clearly-named tests against a fake OpenAI embeddings server and the suite is green —
- R-YGQZ-C8S2 — `NewEmbedder` value is assignable to `Embedder.Provider`; an `Embed` issues a bearer-authenticated `/v1/embeddings` request and decodes one vector per input from the fixture.
- R-YJ6S-3S9G — a batch larger than the per-call item limit is split into multiple round-trips and reassembled to exactly `len(inputs)` vectors in order, usage summed across chunks (asserted on recorded chunk count).
- R-YKEO-HK05 — an over-length input maps OpenAI's 400 to a `*Error` with `Category == ErrContextLength`; no truncated vector is returned.
- R-YLMK-VBQU — the OpenAI adapter emits no task/role field for any `InputType` (asserted on the recorded body).
- R-YMUH-93HJ — an unknown `Model` surfaces `ErrInvalidConfig` (`errors.Is`) and issues no call (also discharging the unknown-model clause of R-Y87O-NUL7 for OpenAI).
- R-YO2D-MV88 — a retryable-category failure on a chunk round-trip is retried under `Embedder.Retry` per D11 (injected clock asserts attempts); a non-retryable failure is not.
- R-YBVD-T5TA — `Dimensions == 0` yields native-dimension vectors; a producible non-zero `Dimensions` yields vectors of exactly that length.
- R-YD3A-6XJZ — a `Dimensions` value the model cannot produce returns `ErrInvalidConfig` (fail-loud) and issues no call.
- R-YPAA-0MYX — `usage.prompt_tokens` maps to `EmbeddingUsage` with `Total == InputTokens`, summed across chunks (OpenAI).
- R-YRQ2-S6GB — the `Embeddings` inspector returns specs with no handle and no network (credential-blind).
- R-YSXZ-5Y70 — `EmbeddingSpec(model)` returns `(spec, true)` for a supported id and `(_, false)` for an unknown one; `SupportedEmbeddings()` is keyed by exactly the exported constants.
- R-YU5V-JPXP / R-YVDR-XHOE — the OpenAI registry's pricing rates and capability specs equal D20's tables (golden).
- R-YWLO-B9F3 — every exported OpenAI embedding constant resolves to both an `EmbeddingPricing` and an `EmbeddingSpec`.
- The OpenAI run of the cross-provider ids R-Y5RV-WB3T, R-Y6ZS-A2UI, R-YANH-FE2L, R-YHYV-Q0IR (fully discharged in Phase 29).
