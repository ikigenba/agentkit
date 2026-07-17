# Phase 64 — Release: minor version bump to v0.3.0

*Realizes — (structural). Depends on Phases 53–63.*

The free-flow/cost-seam/openrouter/subscription/catalog surface ships as `v0.3.0` (pre-stable v0.x line; breaking constructor and SPI changes are sanctioned by the product's versioning constant). Changelog documents: the credential constructors, free-flow models, the cost resolution seam and `WarnCostUnknown`, the openrouter provider, subscription auth (with the notional-cost documentation), the catalog package, and the removed registries/constants/inspectors.

**Done when:** the module tags build green at `v0.3.0`; `git tag` lists `v0.3.0`; the changelog entry exists; `grep -R "ReasoningInspector\|EmbeddingInspector" --include='*.go' .` returns no matches (the removed surfaces are gone from shipped code).
