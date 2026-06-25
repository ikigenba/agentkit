# Phase 29 — Google embeddings adapter (`:batchEmbedContents`) and registry; discharge the cross-provider embeddings ids

*Realizes design Decision 19 (the `EmbeddingProvider` SPI), 20 (embedding registry), and 18 (consumer surface) — the Google slices, plus final discharge of the shared cross-provider ids. Depends on Phase 27 (root embeddings surface) and Phase 28 (the embeddings test harness and the now-fully-exercised root surface).*

The `google` sub-package gains its own embeddings adapter — the Google `embedContent`/`batchEmbedContents` wire shape needs bespoke handling that the OpenAI-compatible variant does not cover:

- `google.NewEmbedder(apiKey, opts...) agentkit.EmbeddingProvider` — `x-goog-api-key` (Gemini API key, not Vertex/OAuth) → `:batchEmbedContents`, reusing the existing options and credential injection.
- The Google embedding registry co-locating `EmbeddingPricing` + `EmbeddingSpec` per model, with the exported constant `EmbedModelGemini001` (`gemini-embedding-001`) and the credential-blind package-level `Embeddings` inspector.
- Google-specific adapter behavior: `InputType` lowered to `task_type` (`InputQuery → RETRIEVAL_QUERY`, `InputDocument → RETRIEVAL_DOCUMENT`, `InputUnspecified → omit`); `autoTruncate:false` so an over-length input errors rather than truncating; mandatory client-side L2-normalization that corrects `gemini-embedding-001`'s un-normalized vectors when output is reduced below native; batch auto-chunk against `batchEmbedContents`'s array limit with order-preserving reassembly and summed usage; the dimension-producible gate over `128 .. 3072`; and per-chunk D11 retry.

**Done when:** the Google slice of these ids is covered by clearly-named tests against a fake Google embeddings server and the suite is green —
- R-YGQZ-C8S2 — `NewEmbedder` value assignable to `Embedder.Provider`; an `Embed` issues an `x-goog-api-key`-authenticated `:batchEmbedContents` request and decodes one vector per input.
- R-YJ6S-3S9G — over-limit batch split and reassembled to `len(inputs)` vectors in order, usage summed (Google).
- R-YKEO-HK05 — `autoTruncate:false` makes an over-length input error, mapped to `ErrContextLength`; no truncated vector returned.
- R-YLMK-VBQU — `RETRIEVAL_QUERY`/`RETRIEVAL_DOCUMENT`/omitted `task_type` on the recorded body for the three `InputType` values.
- R-YMUH-93HJ — unknown `Model` → `ErrInvalidConfig`, no call (Google's unknown-model clause of R-Y87O-NUL7).
- R-YO2D-MV88 — retryable chunk failure retried under `Embedder.Retry`; non-retryable not (Google).
- R-YBVD-T5TA / R-YD3A-6XJZ — native and producible-target dimensions honored; an unproducible `Dimensions` fails loud with `ErrInvalidConfig`, no call (Google `128 .. 3072`).
- R-YPAA-0MYX — `usageMetadata.promptTokenCount` maps to `EmbeddingUsage` with `Total == InputTokens`, summed across chunks (Google).
- R-YRQ2-S6GB / R-YSXZ-5Y70 — credential-blind `Embeddings` inspector; `EmbeddingSpec`/`SupportedEmbeddings` over the exported Google constant.
- R-YU5V-JPXP / R-YVDR-XHOE — the Google registry's pricing rate and capability spec equal D20's tables (golden).
- R-YWLO-B9F3 — every exported Google embedding constant resolves to both an `EmbeddingPricing` and an `EmbeddingSpec`.

This phase is the **last contributing phase** for the cross-provider ids, which are fully discharged here once the identical calling code passes against both providers —
- R-Y5RV-WB3T — one vector per input in order, identical calling code working against both OpenAI and Google (config-only swap).
- R-Y6ZS-A2UI — switching `Provider`/`Model`/`Dimensions` between two calls runs the second against the newly selected provider with no other code change.
- R-YANH-FE2L — `role` defaults to `InputUnspecified` and each of the three values succeeds on both providers.
- R-YHYV-Q0IR — every returned vector is unit-length within tolerance for both providers, including a below-native Google request (provider returns un-normalized; AgentKit's client-side L2-normalization corrects it).
