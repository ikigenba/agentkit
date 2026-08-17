# Phase 123 — add NVIDIA Nemotron 3.5 Lightning and Qwen3.8 Max/27B to the catalog

*Realizes design Decision 26 (advisory model catalog). No phase dependency — the catalog package already exists; this adds two vendor constants and three tracked chat models as data.*

Add two `VendorID` constants and three OpenRouter-primary tracked chat models to
`catalog/data.go`. These vendors have no native AgentKit adapter, so each model's
single offering is its `ProviderOpenRouter` route (the deepseek/kimi pattern) —
authored via `chatEntry(model, vendor, agentkit.ProviderOpenRouter, ...)` with no
alternatives, so `chatEntry` appends no second offering. All values are the
live-verified figures now recorded in research §6.5 (rates) and §7.1 (reasoning).

- **New vendors** (`catalog/data.go` const block, matching D26's vendor list):
  `VendorNVIDIA VendorID = "nvidia"` and `VendorQwen VendorID = "qwen"`. Both are
  provider-package-less, so R-LXFD-GS3B asserts each has no matching `ProviderID`.
- **`nemotron-3.5-lightning`** — vendor `VendorNVIDIA`, provider `ProviderOpenRouter`,
  context `1_000_000`; pricing (nano-USD `RateTier`) `InputUncached: 80,
  CacheReadInput: 40, Output: 200`; reasoning a **toggle** on by default —
  `&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true,
  CanDisable: true, Default: fixed(agentkit.EnableReasoning())}` (the
  `deepseek-v4-pro`/`kimi-k3` shape).
- **`qwen3.8-max`** — vendor `VendorQwen`, provider `ProviderOpenRouter`, context
  `1_000_000`; pricing `InputUncached: 2000, CacheReadInput: 250, Output: 6000`
  (cache-write deliberately omitted, per §6.5 and the deepseek/kimi convention);
  reasoning a **mandatory** effort enum —
  `enumReasoning("effort", []string{"low","medium","xhigh"}, "xhigh", false)`
  (the `gpt-5.5-pro` shape: enum, `CanDisable: false`, default fixed `xhigh`).
- **`qwen3.8-27b`** — vendor `VendorQwen`, provider `ProviderOpenRouter`, context
  `262_144`; pricing `InputUncached: 450, Output: 3200` (no cache tier); reasoning a
  disable-able effort enum —
  `enumReasoning("effort", []string{"low","medium","xhigh"}, "xhigh", true)`.

Wire names are **derived, not overridden**: `WireModel` joins vendor and model
(`nvidia/nemotron-3.5-lightning`, `qwen/qwen3.8-max`, `qwen/qwen3.8-27b`), each the
served OpenRouter slug (§6.5), so no `openRouterWireName` case and no `WireName`
override is added.

Then regenerate the golden reference digest (`recordedReference`) in
`catalog/data_test.go` so the shipped Go data and the recorded reference table agree
(R-DQ16-AQWE) — the only test-file change, mirroring how prior tracked-model rows
were added.

**Done when:**
- `catalog/data.go` contains the three entries and two vendors:
  `grep -c '"nemotron-3.5-lightning"\|"qwen3.8-max"\|"qwen3.8-27b"' catalog/data.go` ≥ 3,
  `grep -c 'VendorNVIDIA\|VendorQwen' catalog/data.go` ≥ 2 (declaration + uses), and
  `grep -c 'openRouterWireName' catalog/data.go` is unchanged from before this phase
  (no override case added).
- The catalog data invariants hold with the three rows present and are what green
  covers: the golden reference test (R-DQ16-AQWE); shipped-offering completeness —
  non-nil pricing/reasoning, positive context (R-E5FU-SAHM); router coverage — each
  carries a `ProviderOpenRouter` offering (R-EBJC-P573); vendor/provider agreement —
  `nvidia`/`qwen` asserted to have no matching `ProviderID` (R-LXFD-GS3B); no
  `DefaultUnaudited` (R-E6NR-628B); `CanEnable ⇒ Toggle` (R-DK0D-JIP0); and
  `spec.Accepts(Default.Value)` for each default (R-DISH-5QYB). These ids are already
  realized by tagged tests in `catalog/` that range over every shipped model; this
  phase adds no new id, so its acceptance is those existing tests passing over the
  three new rows.
- The unit suite is green per design Conventions (`go test ./...`). The
  integration-gated live checks enumerate their subjects from the catalog, so they
  exercise the three new routes when run under `-tags integration`: the wire-name
  round trip (R-4NJ4-SJ41) completes for each, and the CanDisable check (R-DOVZ-2LNS)
  sees a completed off-form for `nemotron-3.5-lightning` and `qwen3.8-27b` and a
  provider-rejected off-form (400 → `ErrInvalidRequest`) for `qwen3.8-max`.
