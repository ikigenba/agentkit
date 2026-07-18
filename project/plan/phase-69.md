# Phase 69 — Catalog the OpenRouter-routed vendors and correct the GLM-5.1 reasoning spec

*Realizes design Decision 26 (the advisory model catalog) — its new ids R-4MB8-ERDC and R-4NJ4-SJ41, plus the existing golden id R-DQ16-AQWE whose recorded table this phase re-pins.*

`agentkit/catalog` gains entries for the three vendors reachable only through OpenRouter — xAI (Grok), DeepSeek, and Moonshot (Kimi) — each with `Provider: "openrouter"` and a `Routes["openrouter"]` vendor-namespaced slug, per the rate data in research §6.5 and the reasoning vocabularies in §7.1. The same pass corrects `glm-5.1`, which currently carries GLM-5.2's `reasoning_effort` enum although Z.ai's API reference scopes that parameter to 5.2 alone; 5.1 has the `thinking` toggle only, like `glm-4.7`/`glm-4.6`.

Observable end state: `catalog.Resolve("", "grok-4.5")` yields provider `openrouter` and wire id `x-ai/grok-4.5`, and the equivalents for `grok-4.3`, `grok-4.20`, `grok-4.20-multi-agent`, `deepseek-v4-flash`, `deepseek-v4-pro`, `kimi-k3`, `kimi-k2.7-code`, and `kimi-k2.6`. `catalog.Check` accepts and rejects reasoning values per each model's spec — notably rejecting `DisableReasoning()` for `grok-4.5`, `kimi-k3`, `kimi-k2.7-code`, and `grok-4.20-multi-agent`, and accepting it for `grok-4.3`, `grok-4.20`, `kimi-k2.6`, and both DeepSeek entries. Context-tiered Grok entries price across the 200K boundary via a second `RateTier`. The golden reference table and the `ListByProvider("openrouter")` expectation are updated to match the new set; no root or provider package gains an import of `catalog` (R-DR92-OIN3 stays true).

The live slug check lands as an integration-gated test in the `openrouter` package's existing integration file, skipping when its credential env var is absent so the default suite stays hermetic.

Deliberately **not** cataloged, and each for a stated reason: `deepseek-v3.2` (no longer addressable on DeepSeek's first-party API), `kimi-k2.5` (EOL 2026-08-31), `kimi-k2.7-code-highspeed` (Moonshot-direct only, so unreachable without a native adapter), `glm-5.2[1m]` (a separate id, not a parameter), and every vendor's budget/flash tier (out of scope — the tracked axis is reasoning mode, not price tier).

**Done when:**
- `R-4MB8-ERDC` — a test asserts that every entry whose `Provider` is `"openrouter"` has a non-empty `Routes["openrouter"]` containing `/`, that `Resolve("", model)` returns that slug rather than the bare model name, and that each such entry appears in `ListByProvider("openrouter")`.
- `R-4NJ4-SJ41` — an integration-gated test drives the real OpenRouter API once per aggregator-routed slug and asserts a completed round trip rather than a model-not-found error; it skips cleanly with no credentials present.
- `R-DQ16-AQWE` — the package's golden reference test passes against the recorded table with the new entries and the corrected `glm-5.1` spec included, and the reasoning `Default` of every entry remains accepted by its own spec.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
