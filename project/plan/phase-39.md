# Phase 39 — Exercise parallel reasoning→tool positional binding (de-trivialize R-IPGC-I69W)

*Realizes design Decision 3 (the canonical message & block data model — the Gemini positional reasoning-binding clause of R-IPGC-I69W). Depends on Phase 11 (the Google adapter that parses `thoughtSignature`/`functionCall` parts and mints tool-use IDs).*

Close audit Finding B/`R-IPGC-I69W` (`project/audit/findings.md`, section B; sketch in `project/audit/b-section-fixes.md`): the claim is that a `ReasoningBlock` produced **alongside parallel tool calls** carries the `BoundToID` of the *specific* `ToolUseBlock` it must bind to (Gemini positional rule). The existing test (`google/google_test.go:165`) uses a fixture with a **single** function call, so `reasoning.BoundToID == use.ID` holds trivially — any binding (even "bind to the only call") passes, and the positional disambiguation that is the heart of the claim is untested.

Production binds reasoning positionally: `parseParts` (google.go:658-712) accumulates `thoughtSignature` parts into a `pending` slice and `flushPending(id)` binds them to the **next** `functionCall`'s freshly minted id. Only a multi-call fixture exercises that the right reasoning binds to the right call.

End state:

- A clearly-named Google-adapter test feeds a response whose parts stream is two parallel reasoning+call pairs — `[thoughtSignature_A, functionCall_A, thoughtSignature_B, functionCall_B]` — assembled through the real adapter (SSE or JSON fixture, consistent with the existing reasoning+tool test harness).
- The test asserts: two `ReasoningBlock`s and two `ToolUseBlock`s are produced; the two minted tool-use IDs are distinct; reasoning A binds to call A and reasoning B binds to call B; **and** an explicit negative — reasoning A's `BoundToID` does *not* equal call B's id. The negative is what makes the binding falsifiable: a "bind all reasoning to the last/first call" regression fails it.
- The single-call assertion's intent is preserved (each reasoning block is bound to a real tool-use id), now under genuine disambiguation.

**Done when:** R-IPGC-I69W is covered by a clearly-named test that exercises parallel tool calls and asserts each `ReasoningBlock.BoundToID` matches its own `ToolUseBlock.ID` (with a cross-binding negative), the other D3 ids stay green, and the suite is green per design Conventions. No production change is expected; if the parallel fixture exposes a real mis-binding, fix the production code.
