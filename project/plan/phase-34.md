# Phase 34 — Make the tool-use/result pairing assertion genuine (de-tautologize R-IKKQ-Z3B4)

*Realizes design Decision 3 (the canonical message & block data model — the pairing half of R-IKKQ-Z3B4). Depends on Phase 01 (the D3 data model and `NewToolUseID`) and Phase 05 (the orchestration tool loop that produces the pairing).*

Close audit Finding A/`R-IKKQ-Z3B4` (`project/audit/findings.md`, section A): the pairing half of the test (`block_test.go:14`, `TestToolUseIDMintsStrictCharsetAndPairsWithResult`) is a tautology — it sets `result.ToolUseID = use.ID` by hand and then asserts they are equal, so it can never fail regardless of any pairing logic. The strict-charset mint half (`NewToolUseID()` against `^[a-zA-Z0-9_-]+$`) is genuine and must be kept; only the pairing assertion is hollow.

The claim has two clauses: (1) a `ToolUseBlock.ID` is AgentKit-minted in the strict charset, and (2) the paired `ToolResultBlock.ToolUseID` equals it. Clause 2 is only meaningful if the pairing is **produced by production code** (the orchestrator's tool loop feeding a result back for a requested tool call), not assigned by the test in the line above the assertion.

End state:

- The strict-charset assertion on `NewToolUseID()` is retained unchanged (genuine mint coverage).
- A clearly-named test drives a real fake-provider tool-loop turn: the assistant round-trip requests a tool via a `ToolUseBlock` carrying a minted `ID`, a registered tool runs, and a final round-trip ends the turn. The test reads the `ToolUse` event's `ID` and the orchestrator-fed `ToolResult` event's `ID` (and/or the `ToolResultBlock.ToolUseID` the loop appended to `History`) **from what production emitted**, and asserts they are equal. The test never assigns `ToolResultBlock.ToolUseID` itself, so a regression that mis-pairs (wrong id, empty id, dropped result) fails the test.
- The asserted `ToolUse` id still matches the strict charset, so both clauses are exercised against production-produced values.

**Done when:** R-IKKQ-Z3B4 is covered by a clearly-named test in which the tool-use/result pairing is produced by the orchestrator's tool loop and read back from the emitted events/`History` (not set by the test), the strict-charset mint of `NewToolUseID()` remains asserted, the other D3 ids stay green, and the suite is green per design Conventions. No production change is expected; if the genuine assertion exposes a real pairing defect, fix the production code.
