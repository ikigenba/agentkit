# Phase 119 — The `xai` chat provider (API key and Subscription credential)

*Realizes design Decision 40 (remaining ids) and Decision 38 (R-E3NF-C33R). Depends on Phase 117 and Phase 118.*

Add public package `xai/` with `New(cred, opts...)`, `APIKey`, `Subscription(TokenSource)`, `WithBaseURL`, `WithHTTPClient`. Both credentials post to `{base}/v1/responses` via `internal/responses` with only `Authorization: Bearer`. Default base URL `https://api.x.ai`. Identity `{ProviderXAI, AuthAPIKey|AuthSubscription}`. Classify xAI `{code,error}` bodies per D40. Extract `cost_in_usd_ticks/10` as `ReportedCost`. Encode `Level` as `reasoning.effort`; omit `reasoning` when unset. Assemble `function_call` / emit `function_call_output`. No `NewEmbedder`. Extend D9/D7/D15 fleet assertions that list shipped providers to include `x-ai`. Extend `catalog` import-graph roots with `xai` (R-DR92-OIN3). Live tests gated on `XAI_API_KEY`.

**Done when:**
- R-DGHC-2G0K — `xai.New(xai.APIKey(k))` httptest posts to `{base}/v1/responses` with the D40 headers and body constraints.
- R-DHP8-G7R9 — `xai.New(xai.Subscription(ts))` httptest posts to the same path with the token-source bearer and no OpenAI-subscription headers.
- R-DIX4-TZHY — Identity and `Identity.String()` match D40; provider errors carry `ProviderXAI` and the matching `Auth`.
- R-DK51-7R8N — fixture `cost_in_usd_ticks` 6752000 → `ReportedCost` 675200 nano-USD, wins over `Conversation.Pricing`; omitted field falls through.
- R-DLCX-LIZC — table-driven classify of the three research §20.6 fixtures.
- R-DQ8J-4LY4 — `Level("low")` emits `reasoning.effort` `"low"`; unset emits no `reasoning` object.
- R-DSOB-W5FI — `function_call` assembles a `ToolUseBlock`; follow-up emits `function_call_output` with the same `call_id`.
- R-E3NF-C33R — empty API key and nil token source construct; `Send` wraps `ErrMissingCredential` with two distinct messages and issues no HTTP request.
- R-DMKT-ZAQ1 — live (gated on `XAI_API_KEY`): plain round trip, non-error assembled non-empty text.
- R-DNSQ-D2GQ — live (gated on `XAI_API_KEY`): tool-call round trip to a non-error assembled final message.
- `go build ./...` and `go test ./...` exit 0; `gofmt -l .` prints nothing.
