# Phase 86 — Catalog completeness invariants and the audited glm OpenRouter routes

*Realizes design Decision 26 (advisory model catalog) — slice: R-E5FU-SAHM, R-E6NR-628B.*

The tiered-audit rule is deleted from the shipped catalog: every shipped offering
becomes complete, and the shipped-data tests enforce it. The four blank glm
OpenRouter offerings (`glm-5.2`, `glm-5.1`, `glm-4.7`, `glm-4.6`) are filled with
their audited terms — pricing and context from the OpenRouter route table in
research §6.5, reasoning specs from the resolved offering table in research
§14.7 (glm-5.2: enum `effort` `[high, xhigh]`, `CanDisable: true`, default fixed
`high`; glm-5.1/4.7/4.6: toggle `thinking`, `CanEnable`/`CanDisable` both true,
default fixed enabled). The `routerOffering` blank-offering helper in
`catalog/data.go` goes with the rule it embodied.

New shipped-data tests in `catalog`:
- R-E5FU-SAHM — every shipped chat offering (default and alternatives alike)
  carries a non-nil `Pricing` with ≥1 `RateTier`, a non-nil `Reasoning`, and
  `Context > 0`; every shipped embedding offering carries `Context > 0`.
- R-E6NR-628B — no shipped offering, on any route, carries
  `Default.Mode == DefaultUnaudited`.

The superseded behaviors leave with their ids: the tests tagged `R-LSJR-XP4J`
(blank-secondary tier), `R-DMG6-B26E` (`Offerings[0]`-only unaudited check), and
`R-DL89-XAFP` (unscoped range derivation — its scoped replacement `R-EABG-BDGE`
lands in Phase 87 together with the OpenRouter offering that motivates the
scoping) are removed, and the golden reference table (R-DQ16-AQWE) is updated
for the now-complete glm cells.

**Done when:**
- `go test ./catalog/...` contains passing tests tagged `R-E5FU-SAHM` and
  `R-E6NR-628B`.
- `grep -rn 'R-LSJR-XP4J\|R-DMG6-B26E\|R-DL89-XAFP' --include='*_test.go' .`
  returns nothing (stale ids fully retired from the suite).
- The suite is green: `go build ./...` and `go test ./...` exit 0.
