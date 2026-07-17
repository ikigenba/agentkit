# Phase 56 — openai adapter to the dumb layer (API-key path)

*Realizes design Decisions 5, 6, and 9 (openai slices). Depends on Phase 53.*

The `openai` package moves to the credential constructor: `New(cred openai.Credential, opts...)` with `openai.APIKey` (the `Subscription` member and its transport arrive in Phase 59; the closed `Credential` interface and `TokenSource` seam are declared now). Chat registry/pricing/reasoning tables, model constants, and the validity gate deleted; free-flow model strings. Reasoning lowers by shape (`Level` → `reasoning.effort`; a `Budget` has no encoding on this wire → `ErrInvalidConfig` per the D9 structural rule). `ProviderOptions` merges into the wire body. The Responses replay traps (summary/arguments, D9) remain green.

**Done when:** the suite is green and the openai slice of R-CQO3-7EE9, R-CT3V-YXVN, R-CUBS-CPMC, R-CVJO-QHD1, and R-CXZH-I0UF is covered by clearly-named tests.
