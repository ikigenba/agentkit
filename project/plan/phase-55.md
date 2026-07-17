# Phase 55 — google adapter to the dumb layer

*Realizes design Decisions 5, 6, and 9 (google slices). Depends on Phase 53.*

The `google` package becomes a model-blind encoder: `New(cred google.Credential, opts...)` with `google.APIKey`; registry/pricing/spec tables, model constants, and the validity gate deleted; free-flow model strings. Reasoning lowers by shape (`Level` → `thinkingLevel`, `Budget` → `thinkingBudget`, disable → `thinkingBudget:0`; never both fields). `ProviderOptions` merges into the wire body. The `thoughtSignature` parse/replay behaviors (D9) remain green.

**Done when:** the suite is green and the google slice of R-CQO3-7EE9, R-CT3V-YXVN, R-CUBS-CPMC, R-CVJO-QHD1, and R-CXZH-I0UF is covered by clearly-named tests.
