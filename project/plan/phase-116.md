# Phase 116 — Catalog grok-4.6 and rewrite the grok-4.5 offering

*Realizes design Decision 26 (advisory model catalog) — slice: R-DMDH-5FOB, R-DQ16-AQWE, R-E5FU-SAHM, R-DK0D-JIP0, R-DHKK-RZ7M, R-DISH-5QYB.*

The advisory catalog's shipped data matches the current Grok offerings (research §6.5 rates, §7.1 reasoning). No new Decision, no new verification id, no provider or adapter change — lowering stays by value shape.

- New chat entry `grok-4.6`: `VendorXAI`, default (only) offering `ProviderOpenRouter`, empty `WireName` (derive `x-ai/grok-4.6`), `Context: 500_000`. Pricing is two `RateTier`s: `{InputUncached: 2000, CacheReadInput: 500, Output: 6000}` and `{MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 1000, Output: 12000}`. Reasoning is `Kind: Enum`, `Term: "effort"`, `Levels: low, medium, high, xhigh`, `CanEnable: false`, `CanDisable: false`, `DefaultFixed` `Level("high")`.
- Existing `grok-4.5` offering rewritten to the same shape except: cache-read 300 / 600, levels `low, medium, high` (no `xhigh`), context 500_000 (was 256_000), rates as above instead of the flat 3000/15000 toggle row.
- `grok-4.3`, `grok-4.20`, and `grok-4.20-multi-agent` are untouched.
- The golden reference table (R-DQ16-AQWE) is regenerated against the new shipped data. Completeness (R-E5FU-SAHM) holds for the new row. `CanEnable` stays false on both enum specs (R-DK0D-JIP0). Each `DefaultFixed` high is accepted by its own spec (R-DISH-5QYB) and carries a non-zero value exactly when the mode is fixed (R-DHKK-RZ7M). `Lookup("grok-4.6")` and `Lookup("grok-4.5")` return the rows above (R-DMDH-5FOB). The existing catalog-enumerated live checks (R-4NJ4-SJ41 wire names, R-DOVZ-2LNS CanDisable) extend automatically to `grok-4.6` and stay integration-gated.

**Done when:**
- `go test ./catalog/...` contains passing tests tagged `R-DMDH-5FOB`, `R-DQ16-AQWE`, `R-E5FU-SAHM`, `R-DK0D-JIP0`, `R-DHKK-RZ7M`, and `R-DISH-5QYB`.
- `grep -n 'grok-4.6' catalog/data.go` is non-empty.
- `Lookup("grok-4.6")` and `Lookup("grok-4.5")` (via the tagged tests or an equivalent assertion in them) report the cells listed above — including 4.5 as an effort enum, not a toggle.
- The suite is green: `go build ./...` and `go test ./...` exit 0.
