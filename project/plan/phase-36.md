# Phase 36 — Exercise a genuine carried-over reasoning value (de-proxy R-B96T-WUUR)

*Realizes design Decision 6 (generation settings and the native reasoning value — the carried-over case R-B96T-WUUR). Depends on Phase 09 (the OpenAI adapter) and Phase 22 (native reasoning value and warn-and-default lowering).*

Close audit Finding A/`R-B96T-WUUR` (`project/audit/findings.md`, section A): the test (`openai/openai_test.go:299`) sets `Level("max")` on `ModelGPT55` in a **fresh** conversation. `"max"` is not in any model's `Levels`, so the case is mechanically identical to the wrong-value case (`R-B7YX-J342`) — it never carries a value over from a previously-selected model. The claim is specifically that validation runs at request-build time against `req.Model`, catching a value that was native to a *previously*-selected model but is invalid for the now-selected one; that carried-over scenario is untested.

The OpenAI registry gives a clean intra-provider carry-over: `gpt-5.5` (`ModelGPT55`) accepts `Levels: [none, low, medium, high, xhigh]`, while `gpt-5.5-pro` (`ModelGPT55Pro`) accepts only `[high, xhigh]`. A `Level("low")` is native to `gpt-5.5` but invalid for `gpt-5.5-pro`.

End state:

- A clearly-named test uses **one** `Conversation` and sets `conv.Gen.Reasoning = agentkit.Level("low")` while `conv.Model` is `ModelGPT55`. A first `Send` against `gpt-5.5` issues the request with the value honored (effort `"low"`) and **no** `Warning` — establishing the value is native to the originally-selected model.
- The conversation then **switches model** to `ModelGPT55Pro` (the value is left carried over, not re-set) and `Send`s again. The request built against `req.Model == gpt-5.5-pro` replaces the now-invalid `Level("low")` with that model's default (effort `"high"`) and records exactly one `Warning{Setting:"reasoning", Code:WarnReasoningUnsupported}`; the turn still succeeds.
- The test thereby proves the carried-over value is caught at the build-time choke point against `req.Model` — not at set time (where it was accepted under `gpt-5.5`) — and is mechanically distinct from `R-B7YX-J342` (which carries a wrong-*kind* `Budget`, never switching models).

**Done when:** R-B96T-WUUR is covered by a clearly-named test that selects a model, sets a reasoning value native to it (no warning), switches to a model for which it is invalid, and asserts the carried-over value is replaced by the new model's default with a single `WarnReasoningUnsupported` warning and a successful turn; the other D6 ids (notably `R-B7YX-J342`) stay green; the suite is green per design Conventions. No production change is expected; if the genuine scenario exposes a real build-time-validation defect, fix the production code.
