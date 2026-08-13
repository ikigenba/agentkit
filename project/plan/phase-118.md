# Phase 118 — ProviderXAI and native-first grok catalog offerings

*Realizes design Decision 26 (catalog) — slice R-DZZQ-6RVO, R-E17M-KJMD, R-E2FI-YBD2. Depends on nothing pending; `ProviderXAI` is a root constant.*

Add `agentkit.ProviderXAI = "x-ai"` next to the other `ProviderID`s (D9/D40). Re-home the five shipped grok chat entries so `Offerings[0]` is the native xAI route with the measured native terms (research §20.5 / D26 R-E17M-KJMD) and OpenRouter remains a later offering with its currently shipped terms. Update the R-LXFD-GS3B assertion: `x-ai` now matches `ProviderXAI`; only `deepseek` and `moonshotai` stay exempt. Update the grok-only catalog tests that currently require a single OpenRouter offering. Refresh the catalog golden (R-DQ16-AQWE) to the new shipped table. Do not add an `xai` package in this phase.

**Done when:**
- R-DZZQ-6RVO — `grok-4.5`, `grok-4.6`, `grok-4.3`, `grok-4.20`, and `grok-4.20-multi-agent` each have `Offerings[0].Provider == ProviderXAI` and a later `ProviderOpenRouter` offering.
- R-E17M-KJMD — those native offerings carry the D26-stated context, nano-USD tiers, and reasoning specs; the OpenRouter offerings of `grok-4.3` and `grok-4.20` still carry their previously shipped divergent terms.
- R-E2FI-YBD2 — `Resolve("", "grok-4.6")` (and each other shipped grok chat model) is `Curated` + `ProviderXAI`; `Resolve(ProviderOpenRouter, "grok-4.6")` is the OpenRouter offering.
- R-LXFD-GS3B's shipped-data check treats `x-ai` as a provider-package vendor (identical to `ProviderXAI`) and no longer expects it to lack a `ProviderID`.
- `go build ./...` and `go test ./...` exit 0; `gofmt -l .` prints nothing.
