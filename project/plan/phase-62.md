# Phase 62 — embedding adapters to the dumb layer

*Realizes design Decision 19 (new id). Depends on Phases 61 and 57.*

The two embedding adapters move to credential constructors and lose their model knowledge: `openai.NewEmbedder(cred, opts...)` (APIKey only — a `Subscription` credential fails loud at construction) and `google.NewEmbedder(cred, opts...)`; the `internal/openaicompat` embeddings variant drops its `Specs`/`Pricing` maps and pre-call gates; `Model` and `Dimensions` pass through verbatim. Normalization, chunking, `autoTruncate:false`, role lowering, and retry behavior are unchanged and stay green.

**Done when:** the suite is green and R-D7QO-K6RZ is covered by a clearly-named test (credential construction + authenticated request for both providers; Subscription rejected at NewEmbedder), with the existing D19 ids (R-YHYV-Q0IR, R-YJ6S-3S9G, R-YKEO-HK05, R-YLMK-VBQU, R-YO2D-MV88) still green.
