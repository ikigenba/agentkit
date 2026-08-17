# Phase 122 — add claude-opus-5 to the catalog

*Realizes design Decision 26 (advisory model catalog). No phase dependency — the catalog package already exists; this appends one tracked chat model as data.*

Add `claude-opus-5` as one tracked chat model in `catalog/data.go`, a sibling of
the existing `claude-opus-4-8` row. The single `entries` map entry is authored via
`chatEntry(...)` with the native Anthropic offering; `chatEntry` auto-appends the
OpenRouter secondary offering (no explicit alternative is passed), so both the
native `ProviderAnthropic` route and the `ProviderOpenRouter` route ship for this
model. All values are the live-verified/audited figures now recorded in research:

- **Vendor/provider:** `VendorAnthropic`, native `agentkit.ProviderAnthropic`.
- **Context:** `1_000_000` (research §6.5).
- **Pricing** (nano-USD `RateTier`, research §6.5): `InputUncached: 5000,
  CacheReadInput: 500, CacheWrite5m: 6250, CacheWrite1h: 10000, Output: 25000` —
  identical to `claude-opus-4-8`.
- **Reasoning** (research §7.1): enum `effort` over `low, medium, high, xhigh, max`,
  default **fixed `medium`**, disable-able — i.e.
  `enumReasoning("effort", []string{"low","medium","high","xhigh","max"}, "medium", true)`.
  This differs from `claude-opus-4-8` (which defaults off) and from `claude-fable-5`
  (which cannot disable); it matches `claude-sonnet-5`.
- **OpenRouter offering:** auto-appended by `chatEntry`; its wire name is **derived,
  not overridden** — `WireModel` joins `anthropic/claude-opus-5`, which is the served
  slug (research §14.7), so no `openRouterWireName` case and no `WireName` override is
  added. Its pricing and reasoning mirror the native offering (research §6.5 OpenRouter
  route table row "= native rates"; §14.7 resolved-offering row).

Then regenerate the golden reference digest (`recordedReference`) in
`catalog/data_test.go` so the shipped Go data and the recorded reference table agree
(R-DQ16-AQWE) — the only test-file change, mirroring how prior tracked-model rows
were added.

**Done when:**
- `catalog/data.go` contains a `"claude-opus-5"` entry:
  `grep -c '"claude-opus-5"' catalog/data.go` ≥ 1, and
  `grep -c 'openRouterWireName' catalog/data.go` is unchanged from before this phase
  (no override case added).
- The catalog data invariants hold with opus-5 present and are what green covers:
  the golden reference test (R-DQ16-AQWE), the shipped-offering completeness check
  — non-nil pricing/reasoning, positive context on both offerings (R-E5FU-SAHM),
  router coverage — the model carries a `ProviderOpenRouter` offering (R-EBJC-P573),
  the no-`DefaultUnaudited` check (R-E6NR-628B), and direct-route-sorts-first
  (R-LW7H-30CM). These ids are already realized by tagged tests in `catalog/` that
  range over every shipped model; this phase adds no new id, so its acceptance is
  those existing tests passing over the new row.
- The unit suite is green per design Conventions (`go test ./...`). The
  integration-gated live round-trip check (R-4NJ4-SJ41) enumerates its subjects from
  the catalog, so it exercises the new `anthropic/claude-opus-5` route when run under
  `-tags integration`.
