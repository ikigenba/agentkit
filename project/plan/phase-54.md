# Phase 54 — anthropic adapter to the dumb layer

*Realizes design Decisions 5, 6, and 9 (anthropic slices). Depends on Phase 53.*

The `anthropic` package becomes a model-blind encoder: constructor `New(cred anthropic.Credential, opts...)` with the closed one-member credential set (`anthropic.APIKey`); the model registry, pricing table, reasoning-spec table, exported model constants, and the `Send`-time validity gate are deleted; any model string flows to the wire verbatim. Reasoning lowers by value shape alone per the D9 table (a `Level` emits `output_config.effort` + adaptive thinking; a `Budget` emits `budget_tokens`; no warn-and-default path remains, and the reasoning `Warning` codes' anthropic uses are removed with it). `Request.ProviderOptions` shallow-merges into the wire body. All replay-trap behaviors (D9) remain green.

**Done when:** the suite is green and the anthropic slice of each cross-provider id is covered by a clearly-named test — R-CQO3-7EE9 (credential construction + authenticated request), R-CT3V-YXVN (uncataloged model passes through verbatim; vendor rejection surfaces typed), R-CUBS-CPMC (no model knowledge consulted, no reasoning warning, value sent as constructed), R-CVJO-QHD1 (lowering by shape per table), R-CXZH-I0UF (ProviderOptions merge on the recorded body).
