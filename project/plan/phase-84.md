# Phase 84 — the catalog's reasoning descriptor: measured defaults, both toggle permissions, per offering

*Realizes design Decision 26 (the advisory model catalog). Depends on Phase 81 (`EnableReasoning` and `ReasoningSpec.CanEnable` must exist) and Phase 83 (the specs hang off offerings, not entries).*

Reshapes `catalog.ReasoningSpec` so it answers the three reasoning questions in three separate fields, refills every offering's cells from the measurements recorded in research §7.1, and enforces the tiered audit rule. The observable end state:

- `catalog.ReasoningDefault` and `catalog.ReasoningDefaultMode` exist, with `DefaultUnaudited` as the zero value, plus `DefaultOff`, `DefaultFixed`, and `DefaultDynamic`.
- `ReasoningSpec.Default` changes type from `agentkit.ReasoningValue` to `ReasoningDefault`, and `CanEnable` joins `CanDisable`.
- Every chat entry's `Offerings[0]` carries a non-nil `Pricing` and a non-nil `ReasoningSpec` with an audited default, and every toggle-shaped offering carries both permissions. Corrections the measurements force, at minimum: `kimi-k3` gains `CanDisable: true` (its off-form returns 200 with reasoning suppressed), and `grok-4.20` is recorded as `DefaultOff` with `CanEnable: true` and `CanDisable: true`.
- The four `glm` entries' secondary `openrouter` offerings carry `nil` `Reasoning` and `nil` `Pricing` — the audited way to say "this route is unmeasured". Copying z-ai's native vocabulary across would state a measured-looking falsehood, since OpenRouter normalizes reasoning into its own parameter shape; and D16 resolves OpenRouter cost from the reported figure, so a rate table there would never be read.
- The live checks are rewritten to enumerate from the catalog rather than from literals, so adding an offering extends them automatically.
- The golden reference table is regenerated to match, so `R-DQ16-AQWE` covers the new cells rather than the old ones.

The invariant tests are the point of the reshape: they make the ambiguities that motivated it mechanically impossible to reintroduce. `R-DMG6-B26E` in particular means adding a model without establishing its default fails the suite instead of shipping a blank that reads as "off".

**Done when** all of the following hold:

- Each id below is covered by a clearly-named test carrying the id verbatim as a tag:
  - `R-DHKK-RZ7M` — across every shipped offering with a `ReasoningSpec`, `Default.Value` is non-zero exactly when `Default.Mode == DefaultFixed`.
  - `R-DISH-5QYB` — every `DefaultFixed` offering satisfies `spec.Accepts(spec.Default.Value)`.
  - `R-DK0D-JIP0` — `CanEnable == true` implies `Kind == ReasoningToggle`, across every shipped offering.
  - `R-DL89-XAFP` — every `Kind == ReasoningRange` offering satisfies `CanDisable == (Min == 0 || an "off" sentinel exists)`; `gemini-2.5-flash` is true and `gemini-2.5-pro` is false.
  - `R-DMG6-B26E` — no shipped chat entry's `Offerings[0]` has `Default.Mode == DefaultUnaudited`.
  - `R-LSJR-XP4J` — every shipped chat entry's `Offerings[0]` carries non-nil `Pricing` and non-nil `Reasoning`, and at least one shipped secondary offering carries `nil` for one of them.
  - `R-DNO2-OTX3` — `spec.Accepts(agentkit.EnableReasoning())` is true exactly when `CanEnable` is set, and false on enum/range specs regardless of `CanDisable`.
  - `R-DOVZ-2LNS` — live, integration-gated: for each OpenRouter offering the catalog's `CanDisable` claim matches the real API — the off-form completes for every `CanDisable: true` offering and is rejected for every `CanDisable: false` offering.
  - `R-4NJ4-SJ41` — live, integration-gated: each derived OpenRouter wire name is accepted by the real API on a minimal round trip, with the subject list enumerated from `ListCurated(agentkit.ProviderOpenRouter)` rather than a literal, skipping cleanly without credentials.
  - `R-DQ16-AQWE` — the golden reference table matches the shipped data including each offering's provider, vendor, order, `Kind`, `Levels`, `Min`/`Max`, `Sentinels`, `CanEnable`, `CanDisable`, and `Default`.
- The `kimi-k3` correction is present: `sed -n '/"kimi-k3"/,/^	},/p' catalog/data.go | grep -qE 'CanDisable: *true'` exits 0.
- Neither live test carries a hardcoded slug list: `grep -nE '"(x-ai|z-ai|deepseek|moonshotai)/' --include='*_test.go' -r .` returns no matches.
- `go build ./...` and `go test ./...` both exit 0 (design Conventions).
- The integration suite compiles: `go vet -tags integration ./...` exits 0.
