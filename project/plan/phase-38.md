# Phase 38 — Assert MessageDone mirrors History for richer message shapes (de-partial R-CBA7-N2NL)

*Realizes design Decision 2 (the consumption surface — the emitted-equals-History clause of R-CBA7-N2NL). Depends on Phase 05 (the orchestration tool loop and `MessageDone` emission) and Phase 01 (the D3 block model, incl. `ReasoningBlock`/`ToolUseBlock`).*

Close audit Finding B/`R-CBA7-N2NL` (`project/audit/findings.md`, section B; sketch in `project/audit/b-section-fixes.md`): the claim is that each completed assistant message is emitted as a `MessageDone` carrying the fully assembled `Message` — **visible text, tool_use blocks, and any reasoning summary** — and that same message is what landed in `History`. The existing assertion (`orchestration_test.go:226`) proves `reflect.DeepEqual(done.Message, conv.History[1])` only for a single-`TextBlock` message; the enumerated tool_use-block and reasoning-summary shapes are never asserted emitted-and-equal-to-History.

Production already builds the event and the History entry from the same `message` via `cloneMessage` (orchestration.go:333-335), so equality holds for every block type — it is simply unasserted for the richer shapes. This is a test gap, not a design or code defect.

End state:

- A clearly-named test drives a fake-provider tool-loop turn whose first assistant round-trip returns a `Message` containing a `ReasoningBlock` (with a non-empty `Summary`), a `TextBlock`, and a `ToolUseBlock`, followed by a tool run and a tool-free final round-trip.
- The test collects the emitted events and asserts the first `MessageDone.Message` `reflect.DeepEqual`s `conv.History[1]` (the assembled assistant message), and that that History entry actually carries the three block types — so a regression that drops, reorders, or diverges any block between the emitted event and History fails the test.
- The single-`TextBlock` assertion already at `orchestration_test.go:226` is retained.

**Done when:** R-CBA7-N2NL is covered by a clearly-named test asserting the emitted `MessageDone` equals the committed `History` message for a reasoning-summary + text + tool_use shape, the other D2 ids stay green, and the suite is green per design Conventions. No production change is expected; if the assertion exposes a real divergence, fix the production code.
