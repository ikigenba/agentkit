# Phase 82 — the catalog's reasoning descriptor: measured defaults and both toggle permissions

*Realizes design Decision 26 (the advisory model catalog). Depends on Phase 81 (`EnableReasoning` and `ReasoningSpec.CanEnable` must exist).*

Reshapes `catalog.ReasoningSpec` so it answers the three reasoning questions in three separate fields, and refills the shipped data from the measurements recorded in research §7.1. The observable end state:

- `catalog.ReasoningDefault` and `catalog.ReasoningDefaultMode` exist, with `DefaultUnaudited` as the zero value, plus `DefaultOff`, `DefaultFixed`, and `DefaultDynamic`.
- `ReasoningSpec.Default` changes type from `agentkit.ReasoningValue` to `ReasoningDefault`, and `CanEnable` joins `CanDisable`. This is a breaking change to a consumer-facing type; Phase 83 records it.
- Every chat entry in `catalog/data.go` carries an audited default in the new shape, and every toggle-shaped entry carries both permissions. Corrections the measurements force, at minimum: `kimi-k3` gains `CanDisable: true` (its off-form returns 200 with reasoning suppressed), and `grok-4.20` is recorded as `DefaultOff` with `CanEnable: true` and `CanDisable: true`.
- The recorded reference table backing the golden test is regenerated to match, so `R-DQ16-AQWE` covers the new cells rather than the old ones.

The invariant tests are the point of the reshape: they make the ambiguities that motivated it mechanically impossible to reintroduce. `R-DMG6-B26E` in particular means adding a model without establishing its default fails the suite instead of shipping a blank that reads as "off".

**Done when** all of the following hold:

- Each id below is covered by a clearly-named test carrying the id verbatim as a tag:
  - `R-DHKK-RZ7M` — across every shipped entry with a `ReasoningSpec`, `Default.Value` is non-zero exactly when `Default.Mode == DefaultFixed`.
  - `R-DISH-5QYB` — every `DefaultFixed` entry satisfies `spec.Accepts(spec.Default.Value)`.
  - `R-DK0D-JIP0` — `CanEnable == true` implies `Kind == ReasoningToggle`, across every shipped entry.
  - `R-DL89-XAFP` — every `Kind == ReasoningRange` entry satisfies `CanDisable == (Min == 0 || an "off" sentinel exists)`; `gemini-2.5-flash` is true and `gemini-2.5-pro` is false.
  - `R-DMG6-B26E` — no shipped chat entry with a `ReasoningSpec` has `Default.Mode == DefaultUnaudited`.
  - `R-DNO2-OTX3` — `spec.Accepts(agentkit.EnableReasoning())` is true exactly when `CanEnable` is set, and `catalog.Check` agrees for a cataloged model.
  - `R-DOVZ-2LNS` — live, integration-gated: for each aggregator-routed entry the catalog's `CanDisable` claim matches the real OpenRouter API — the off-form completes for every `CanDisable: true` entry and is rejected for every `CanDisable: false` entry.
  - `R-DQ16-AQWE` — the golden reference table matches the shipped data including `Kind`, `Levels`, `Min`/`Max`, `Sentinels`, `CanEnable`, `CanDisable`, and `Default`.
- `grep -c 'CanDisable: *true' catalog/data.go` includes the `kimi-k3` entry: `sed -n '/"kimi-k3"/,/^	},/p' catalog/data.go | grep -qE 'CanDisable: *true'` exits 0.
- No provider or root package imports the catalog (`R-DR92-OIN3` still green).
- `go build ./...` and `go test ./...` both exit 0 (design Conventions).
- The integration suite compiles: `go vet -tags integration ./...` exits 0.
