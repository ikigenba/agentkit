# Phase 58 — the openrouter provider package

*Realizes design Decision 24 (offline ids) and the openrouter slices of Decisions 5, 6, and 9. Depends on Phase 57.*

New public package `openrouter`: `New(cred openrouter.Credential, opts...)` over `internal/openaicompat`, base `https://openrouter.ai/api/v1` baked in, label `"openrouter"`. Response cost (`usage.cost` + `cost_details.upstream_inference_cost`, final SSE frame) is converted credits→nano-USD into `RoundTrip.ReportedCost`; a costless response yields `(0,false)`. Reasoning lowers to the normalized `reasoning` object. Routing rides `ProviderOptions`.

**Done when:** the suite is green and each id is covered by a clearly-named test — R-DA6H-BQ9D (authenticated request, verbatim slug incl. `:floor`/`:nitro`), R-DBED-PI02 (reported-cost extraction and absence), R-DCMA-39QR (reasoning encoding), plus the openrouter slices of R-CQO3-7EE9, R-CRVZ-L64Y, R-CT3V-YXVN, R-CUBS-CPMC, R-CVJO-QHD1, R-CXZH-I0UF.
