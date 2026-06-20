# AgentKit Audit

**Date:** 2026-06-19 · **Module:** `github.com/ikigenba/agentkit`
**Scope:** correctness/completeness vs. the contract docs (a) + forward-looking improvements (b)
**Method:** multi-agent research — 10 audit areas, every finding adversarially verified before inclusion (59 findings, 52 confirmed, 7 refuted).

This document is a backlog. Each finding below is to be investigated in a future
session; check it off when resolved (fixed, or consciously declined with a note).

---

## Verdict

AgentKit is **substantially correct and complete against its own contract**. All
17 Decisions and **116** genuine R-ids (the "117" figure double-counted the
literal `R-XXXX-XXXX` template on design.md:8) are implemented and cited by tests
that genuinely assert their behavior — not name-tagged stubs. The eight Phase
15–23 replay-shape fixes are each **mutation-verified** (revert the fix → its
pinned test fails → restore). `internal/*` indirect coverage is real and
meaningful (e.g. `sse.ReadAll` 94.1%, `openaicompat.assemble` 94.4% via adapter
tests). The REPL is fully gone (this is a library; no CLI/app surface).
`go build ./...` and `go vet ./...` are clean.

### Answers to the audit's required questions
- **Does the code fulfill every Decision and R-id?** Yes — all 17 Decisions and
  all 116 genuine R-ids are implemented and cited. One id cited by tests
  (`R-P71Z-J46O`) has no contract definition (Finding 2).
- **Are R-ids backed by genuine green tests?** Yes in substance — every sampled
  citing test truly asserts its id's behavior (cost/reasoning-spec equality are
  real `reflect.DeepEqual`/byte assertions).
- **Are the Phase 15–23 replay fixes truly pinned?** Yes — all eight verified by
  mutation testing.
- **Is `internal/*` indirect coverage real?** Yes — confirmed via a
  `-coverpkg=./...` merged profile; the 0.0% per-package figures are an artifact
  of having no in-package test files, not absence of exercise.
- **Is the REPL fully gone?** Yes — nothing in the repo realizes it; extracted to
  the standalone `agentrepl` project.

---

## Do now — blocks a clean contract gate

- [x] **Finding 1 — `go test ./...` was RED: stray cancel test hung to timeout.**
  *(RESOLVED 2026-06-19)*
  `anthropic/zz_cancel_test.go` (untracked, created after the last green run)
  blocked forever on `<-r.Context().Done()` under an unconditional
  `defer srv.Close()`, hanging the anthropic package to the 600s default timeout.
  Zero assertions (only `t.Logf`), no R-id, and the sole `gofmt -l` violation.
  Removed the untracked file; `go test ./...` is fully green again.
  *Note for any future cancellation test:* drive it through `Conversation.Send`
  with a handler that returns promptly — do **not** block the handler on
  `r.Context().Done()` under an unconditional `defer srv.Close()`. The anthropic
  `RoundTrip` is eager (`io.ReadAll` at anthropic.go:176), so "drain events
  first" would not have fixed the hang; the server handler was the problem.

---

## Before release

- [ ] **Finding 2 — MEDIUM — Orphan verification id `R-P71Z-J46O`.**
  Cited in `openai/openai_test.go:77`, `zai/zai_test.go:81`,
  `anthropic/anthropic_test.go:328`, `google/google_test.go:50`, and
  `plan.md:69,76,83,90`, but **undefined in design.md** — a "verification" with
  no contract to verify against. The behavior these tests assert (generation/
  reasoning settings reaching the request) is already covered by co-cited defined
  ids `R-P5U3-5CFZ` (sampling), `R-T40A-VZQ7` (native reasoning passthrough),
  `R-ELUQ-VJIQ` (lowering table). **Action:** mint a definition in design.md, or
  drop the orphan citation — removing it loses no coverage.

- [ ] **Finding 3 — Pricing / reasoning-spec staleness (RELEASE OBLIGATION,
  design.md:1020,1046).** Base rates verify clean against live data and the
  registries copy the design tables verbatim (no transcription drift). Still
  outstanding as of 2026-06-19:
  - `gpt-5.5-pro` levels `[high, xhigh] (est.)` + default `high (est.)`
    (design.md:1027) — **unverified**; no official page enumerates pro levels and
    the predecessor `gpt-5-pro` was high-only. *Risk:* if pro is high-only, the
    over-broad set lets `xhigh` reach the API → live **400**, not the soft D6
    warn-and-default. Verify live; if high-only set `Levels=[high]`.
  - `gpt-5.4-mini` default `none (est.)` (design.md:1030) — verify live.
  - `gpt-5.4-nano` default `none (est.)` (design.md:1031) — verify live.
  - `gemini-3.1-flash-lite` default `medium (est.)` (design.md:1035) — verify live.
  - `gemini-3.1-pro-preview` model id (design.md) — still the live id per
    ai.google.dev; confirm with one live `Send` before release.
  - Anthropic cache-write multipliers (design.md:994 vs 1048) — doc is internally
    inconsistent (994 "high confidence" vs 1048 "lower"); cache-read verified at
    exactly 0.1x. Downgrade the 1048 flag; spot-check one invoice line.
  - **Refuted (no action):** `gpt-5.4-mini` $0.75/$4.50 is correct (the ~$0.40
    figure was GPT-4.1-mini confusion); `glm-5.1` $1.40/$0.26/$4.40 is correct vs
    Z.ai-direct (the $0.98 figure was an OpenRouter reseller discount).

- [x] **Finding 4 — Incremental streaming is not actually delivered
  (contract-vs-product).** *(RESOLVED 2026-06-20 — Phase 24.)* Adopted **Option B**
  (message-granular delivery): dropped the consumer-facing `*Delta` surface so the
  stream emits only completed `MessageDone`/`ToolUse`/`ToolResult` events, and
  softened product.md / design.md to match ("messages delivered as completed units,"
  no token-by-token promise). Backend SSE decoding is retained (D12 unchanged);
  `R-C7MI-HRFI` deleted, `R-HUZX-7N2W` minted; `R-CCI4-0UEA` still holds by
  construction (no live consumer streaming was introduced).
  Every adapter `io.ReadAll`s the full response body and
  then replays a precomputed in-memory slice (anthropic.go:176/181,
  openaicompat.go:94/110, google.go:188/200). This contradicts product.md:73
  ("Replies are delivered incrementally") and design.md:136 ("as the model
  generates"). No verification id is violated (R-C7MI-HRFI is timing-agnostic),
  so it is a product-promise vs. implementation gap, not a test failure.
  **Decision needed:** implement live SSE decoding (and then actively close the
  body on early break — re-arming R-CCI4-0UEA), **or** soften product.md:73 /
  design.md:136 to "deltas emitted post-buffer."

- [ ] **Finding 5 — Design-prose contradiction at the pricing tier boundary.**
  design.md:1069 (R-V2SM-WC8V) says "at or below [MinInputTokens] → base tier,"
  but pricing.go:39 uses `>=`, so `total == MinInputTokens` lands in the HIGH
  tier. Tests already pin the impl behavior. **Action:** tighten the prose to
  "strictly below → base; at-or-above → high." (Design-vs-impl contradiction,
  originally mislabeled as a coverage gap.)

- [ ] **Finding 6 — `RetryPolicy.MaxElapsed` enforcement has zero coverage.**
  A documented public knob whose entire enforcement path is dead in tests.
  `boundedRetryDelay` at 37.5%; the budget-exhausted branch (orchestration.go:
  469–472, `return -1`) and its consumer abort (orchestration.go:435–436,
  `if delay < 0`) never execute — every test takes the `MaxElapsed==0`
  short-circuit. **Action:** add a test setting `RetryPolicy.MaxElapsed` with a
  fake clock advancing past the budget, asserting the loop aborts rather than
  sleeping. The `retryClock` interface already abstracts `Now/Sleep/Jitter`.
  (Severity arguably medium — opt-in; the zero-value default path is covered.)

- [x] **Finding 7 — Anthropic in-stream SSE `error` event never tested.**
  *(RESOLVED 2026-06-20 — Phase 25.)* Added a clearly-named test driving a fake
  server that returns **200** + `text/event-stream` and emits an `event: error`
  frame (`overloaded_error` / `rate_limit_error`), asserting the mapped `Category`
  via `errors.Is`, the populated `Type`, and byte-for-byte `Raw`. Test-only —
  `classifyStreamError` already implemented the behavior; minted `R-FR35-46U7`.
  `classifyStreamError` (anthropic.go:658), reached only from the
  200-then-`event: error` SSE branch (anthropic.go:487–489), is at 0.0%. The
  existing error-mapping test (anthropic_test.go:541–583) uses `w.WriteHeader`
  (non-2xx), routing through `classifyHTTP` instead. This is the only path for
  provider overload/rate-limit signaled **mid-stream on a 200**. **Action:** add
  a 200 + `text/event-stream` test emitting `event: error\ndata: {…}` and assert
  the mapped category / Type / Raw.

- [ ] **Finding 8 — Durable-persistence escape hatch does not work.**
  product.md:36 says "a consumer may serialize the object itself," but
  `Conversation.History` → `[]Block` is a sealed interface with no custom
  `MarshalJSON/UnmarshalJSON`: `json.Marshal` drops the type discriminator and
  `json.Unmarshal` into `[]Block` **errors** (reproduced). It is **not** a
  one-way door. **Action:** soften product.md:36, or add additive discriminated-
  union marshalers — best done now, while only four block types exist (each
  future type widens the gap).

---

## Nice-to-have

- [ ] **Finding 9 — Transport/network error classification untested in
  openai/google/openaicompat.** `transportError`/`providerTransportError` are 0%
  (openaicompat.go:559, openai.go:723, google.go:797); no test triggers a
  connection failure, body-read failure, or `DeadlineExceeded→ErrTimeout`.
  *(Correction: anthropic's `classifyTransport` was exercised only by the broken
  stray test — once Finding 1's file is gone it is uncovered too, but the colocated
  issue there was the hang, now resolved.)* **Action:** per-adapter test with a
  custom `http.RoundTripper` (or connection-closing server) and an expired-deadline
  context; assert `ErrNetwork`/`ErrTimeout`.

- [ ] **Finding 10 — OpenAI `finishFromIncomplete` (truncation mapping)
  untested.** `finishFromIncomplete` (openai.go:693) maps `response.incomplete`
  reasons (`max_output_tokens→FinishMaxTokens`, `content_filter→
  FinishContentFilter`) at 0.0%. A broken mapping mis-reports truncation/filtering
  to callers, affecting downstream retry/continuation. **Action:** drive an
  incomplete response with each reason; assert the finish reason.

- [ ] **Finding 11 — Context cancellation misclassified.** `context.Canceled`
  (vs. `DeadlineExceeded`) falls through to retryable `ErrNetwork` in all transport
  classifiers, so a cancelled `Send` emits a spurious `"retry"` log record and
  computes a backoff before bailing inside `clock.Sleep`. No extra round-trip is
  issued and `errors.Is(context.Canceled)` still holds (design.md:153 honored), so
  harm is observability + a wasted backoff. **Action:** short-circuit `ctx.Err()`
  at the top of each `roundTripWithRetry` attempt and/or classify
  `context.Canceled`.

- [ ] **Finding 12 — >4MB single SSE line hard-fails the turn.** `internal/sse/
  sse.go:19` caps the scanner token at 4MB; an oversize frame returns "token too
  long." *(Correction: "retries pointlessly as ErrNetwork" holds for Anthropic/
  OpenAI only — Google passes `err=nil` to `providerError`, yielding non-retryable
  `ErrUnknown`, so Google fails immediately.)* Most plausible trigger is a large
  Google whole-frame tool-args/text part. **Action:** raise/remove the cap or
  document it.

- [ ] **Finding 13 — Direct unit tests for `internal/httpx` & `internal/sse`.**
  No direct test files. Untested branches: `sse.ReadAll` scanner-error / >4MB-frame
  (sse.go:62–63), `httpx.RetryAfter` HTTP-date branch (httpx.go:51–58, 45.5%),
  `httpx.JSONRequest` encode-error (httpx.go:27–29). All trivially offline per
  Decision 13. (Google never calls `RetryAfter`.)

- [ ] **Finding 14 — `internal/mcp` `DeleteSession` branches untested.** The
  405-benign and 5xx-error branches are 0%; only the 202 happy path runs, and even
  then via `_ =` so the return is discarded. Pure best-effort-contract lock-in.

- [ ] **Finding 15 — Weak/redundant verification tests (tighten, low priority).**
  - `R-IN0J-QMSI` "byte-for-byte Opaque round-trip" is verified by a hardcoded
    signature literal (`requestContainsSignature(requests[1], "sig-anthropic-1")`,
    anthropic_test.go:227), not by comparing captured `Opaque` bytes. Strict
    byte-equality is unsatisfiable (Opaque is a `{"signature":…}` envelope); the
    real strengthening is to extract the signature from the captured Opaque and
    compare that.
  - `R-CCI4-0UEA` "no goroutine leak" is unasserted but holds by construction
    (synchronous range-over-func iterator; no per-stream goroutine in non-test
    source). *(Would need re-arming if Finding 4 introduces live streaming.)*
  - openai `R-V2SM` tier-boundary check is weak (272001 vs 272002, both already
    HIGH). The transition is correctly pinned by root `pricing_test.go` (100 vs
    101) and `google_test.go` (200000 vs 200001), so the contract is verified; the
    openai variant is redundant.
  - `R-V1KQ` pricing-completeness iterates a hand-coded model slice, not the
    registry — an exported constant absent from both would be invisible. Mitigated
    by root `reasoning_test.go`'s `reflect.DeepEqual(SupportedReasoning(), specs)`
    (single registry backs both pricing and reasoning).

- [ ] **Finding 16 — Ergonomics additions (all additive; v0.x allows breaking
  changes).**
  - **`Stream.Message()` / `Text()` accessor (MEDIUM):** consumers currently must
    re-concatenate `TextDelta` or reach into `Conversation.History`. design.md:69
    already names "a final-text convenience may layer on top later." Cache it like
    usage/cost already are; note multi-turn semantics (tool loops emit multiple
    `MessageDone`).
  - **`Ptr[T any](v T) *T` helper (LOW):** `GenSettings.Temperature/TopP` are
    `*float64`, forcing the take-address-of-a-local dance everywhere.
  - **`Provider.Models()` listing (LOW):** supported ids are discoverable only via
    scattered exported constants; registry keys already exist. (No contract requires
    runtime enumeration — product.md:27 = "fixed, curated, closed set.")
  - **Clarify stream-accessor doc comments (INFO):** `Usage()/Cost()/Warnings()`
    silently return zero/partial values pre-drain; document (optionally fail-loud).
  - **Do NOT add a multimodal `SendBlocks`** — text-only `Send` is a permanent
    product boundary (product.md:34–35). Only valid action: restate text-only in
    the `Send` doc comment.

- [ ] **Finding 17 — Missing `default` cases in adapter block-type switches
  (LOW).** Three adapter switches lack a `default`, so a future block type would be
  **silently skipped** rather than caught at compile time or runtime. Worth a
  fail-loud `default` per the project's "fail loudly" principle. (Images/audio
  remain a sound, no-action deferral otherwise; embeddings have zero scaffolding —
  correct; MCP resources/prompts and OAuth/ambient-creds are cleanly layerable;
  stdio MCP would need a transport seam below the JSON-RPC layer — acceptable since
  stdio is rejected, not deferred-soon.)

---

## Refuted findings (recorded so they are not re-investigated)

- `gpt-5.4-mini` price $0.75/$4.50 is **correct** (the ~$0.40/$1.60 "conflict" was
  GPT-4.1-mini confusion).
- `glm-5.1` $1.40/$0.26/$4.40 is **correct** vs Z.ai-direct (the $0.98/$3.08 figure
  was an OpenRouter reseller discount).
- The claim that all four adapters' transport classifiers are untested overstated:
  anthropic's was exercised (via the now-removed stray test) — see Finding 9.
- The ">4MB SSE line retries pointlessly as ErrNetwork" claim was too broad — true
  for Anthropic/OpenAI, but Google fails immediately as `ErrUnknown` — see
  Finding 12.

---

## Replay-fix confidence (all PINNED, mutation-verified)

Phase 15 `R-OUE3-L8VS` · 16 `R-OMKB-AY19` · 17 `R-UJNS-PFLL` · 18 `R-ZCMP-ARG8` ·
19 `R-DNS8-QC6Z`/`R-DRFX-VNF2`/`R-DTVQ-N6WG` · 20 `R-GSIG-PT07` · 23 `R-TQ77-6QLK`.
Each: reverted the fix → pinned test FAILED with the exact provider 400/assertion →
restored (git clean). `R-XW08-D4YL` correctly covers non-empty `Opaque`; the
Phase-23 thinking-field fix is correctly carried by the separate `R-TQ77-6QLK`
test — no mis-citation.
