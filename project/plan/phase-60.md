# Phase 60 — the advisory catalog package

*Realizes design Decision 26. Depends on Phase 53.*

New package `agentkit/catalog`: the `Entry` shape keyed by unique model name (provider default, `Routes`, chat `Pricing`, `ReasoningSpec`, context, `EmbeddingInfo`, default options); `Lookup`, `Resolve`, `ListByProvider`, `Check` helpers; the maintained data (the former registry tables carried forward — chat rates incl. tiered models, reasoning specs, embedding rates/capabilities, routes) with a package-local golden reference; and `catalog/openrouterx` with the `Routing` builder. No import from root or any provider package.

**Done when:** the suite is green and each id is covered by a clearly-named test — R-DMDH-5FOB, R-DNLD-J7F0, R-DOT9-WZ5P, R-DQ16-AQWE (golden), R-DR92-OIN3 (import-graph assertion), R-DSGZ-2ADS (Routing builder output + merge round-trip).
