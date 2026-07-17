# Phase 57 — internal/openaicompat + zai to the dumb layer

*Realizes design Decisions 5, 6, and 9 (zai/compat slices). Depends on Phase 53.*

The shared `internal/openaicompat` chat adapter drops its `Pricing` map and pre-call gate (`cfg.Pricing`/`cfg.Reasoning` model tables deleted); it gains the per-provider configuration seams the compat family needs: reasoning-encoding selection (zai's `thinking`/`reasoning_effort` vs openrouter's `reasoning` object, Phase 58), optional response-cost extraction plumbing into the `ReportedCost` slot, and the `ProviderOptions` merge. The `zai` package moves to `New(cred zai.Credential, opts...)` with `zai.APIKey`, keeps its baked-in base URL, loses its model knowledge, and lowers reasoning by shape (`Budget` → `ErrInvalidConfig`, no budget field on this wire).

**Done when:** the suite is green and the zai/compat slice of R-CQO3-7EE9, R-CRVZ-L64Y (zai half), R-CT3V-YXVN, R-CUBS-CPMC, R-CVJO-QHD1, and R-CXZH-I0UF is covered by clearly-named tests.
