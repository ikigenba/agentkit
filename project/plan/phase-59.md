# Phase 59 — openai subscription auth: the second credential + openai/subscription

*Realizes design Decision 25 (offline ids). Depends on Phase 56.*

The `openai.Subscription(ts TokenSource)` credential activates: a handle built with it targets `chatgpt.com/backend-api/codex/responses` with the per-request bearer + `chatgpt-account-id` + `originator` + beta headers over the existing Responses adapter (whose `store:false`/`instructions`/encrypted-include behavior already satisfies the backend). New opt-in subpackage `openai/subscription`: `Load(path)`, `(*Store).Token` (proactive refresh at auth.openai.com, atomic 0600 rewrite, single-flight, no token logging), `Login` (headless manual-paste PKCE, S256 + state check). `Name()` labels become `openai.apikey`/`openai.subscription`.

**Done when:** the suite is green and each id is covered by a clearly-named test — R-DG9Z-8KYU (subscription transport + headers + body constraints; apikey path unchanged), R-DHHV-MCPJ (refresh, atomicity, mode 0600, single-flight), R-DIPS-04G8 (PKCE login writes a research-§15.1-shaped file; bad state rejected), R-DL5K-RNXM (mode labels on Name() and Error.Provider).
