# Phase 63 — live integration: OpenRouter reported cost & ChatGPT-subscription round trip

*Realizes design Decisions 24 and 25 (live ids). Depends on Phases 58 and 59.*

The two claims that hinge on real external contracts are proven against the real services, as presence-gated live tests that skip cleanly when their credential is absent: a real OpenRouter round trip returning a provider-reported cost, and a real ChatGPT-backend round trip authenticated from a locally present codex-compatible auth file (read/use only — no refresh forced against the shared-lineage file).

**Done when:** the suite is green; with `OPENROUTER_API_KEY` present, the R-DF22-UT85 test completes a real round trip with `ReportedCost()` present and > 0; with the local auth file present, the R-DJXO-DW6X test completes a real subscription round trip with a non-error assembled message; both tests skip (not fail) when their gate is absent.
