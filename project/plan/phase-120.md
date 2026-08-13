# Phase 120 — `xai/subscription` store

*Realizes design Decision 41. Depends on Phase 119.*

Add opt-in `xai/subscription` with `Load(path)` and `Store.Token` implementing `xai.TokenSource`. Raw token-endpoint file only; baked client id and `https://auth.x.ai/oauth2/token`; 5-minute skew; atomic 0600 rewrite; carry forward omitted `refresh_token` / `id_token`; single-flight; no login; no path discovery; no reactive 401 refresh. Extend the catalog import-graph roots with `xai/subscription`. Live test gated on `XAI_SUBSCRIPTION_AUTH_FILE`.

**Done when:**
- R-DTW8-9X67 — `Load` of a raw token-response file succeeds; `Token` returns the unexpired `access_token`.
- R-DV44-NOWW — `Load` of a file with no top-level `access_token` (including a grok-CLI wrapper shape) fails naming `access_token`.
- R-DWC1-1GNL — expired / skew-near token refreshes once under concurrency; rewrite is mode `0600`; subsequent `Token` uses the new bearer.
- R-DXJX-F8EA — omitted `refresh_token` / `id_token` are carried forward; rotated values replace when present; a fresh `Load` of the rewritten file succeeds.
- R-DYRT-T04Z — live (gated on `XAI_SUBSCRIPTION_AUTH_FILE`): one short real `Send` returns a non-error assembled message; read/use only, no forced refresh.
- `go build ./...` and `go test ./...` exit 0; `gofmt -l .` prints nothing.
