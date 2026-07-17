# Phase 61 — embeddings root seam: free-flow, supplied pricing, dimension verification

*Realizes design Decisions 18 and 20 (new ids). Depends on Phase 53.*

The root embeddings surface moves to the dumb-layer rules: `Embedder` gains `Pricing *EmbeddingPricing` beside `Model`; boundary validation keeps only missing-config (`ErrInvalidConfig`) and empty-input (`ErrInvalidInput`) — the registry-unknown-model rejection is gone; the requested-dimension promise is enforced by the post-return length check; cost resolves supplied-else-zero+`WarnCostUnknown` per D16's philosophy. The `EmbeddingProvider` SPI loses its `Pricing` method (mechanical accommodation in the two adapters; their behavioral rework is Phase 62).

**Done when:** the suite is green and each id is covered by a clearly-named test — R-D5AV-SNAL (missing-config only; present-but-unknown model reaches the wire), R-D6IS-6F1A (mis-sized return fails loud), R-D2V3-13T7 (supplied pricing → cost + accumulation), R-D42Z-EVJW (nil pricing → 0 + WarnCostUnknown per call), R-D8YK-XYIO (EmbeddingPricing.Cost math).
