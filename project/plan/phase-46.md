# Phase 46 — Deferred tools & the built-in `load_tools` meta-tool (offline)

*Realizes design Decision 23 (deferred tools — the offline Verification ids) and the rewritten Decision 10 cache-prefix/ordering prose (R-VXPR-861V's base-set rescoping). Depends on Phase 05 (the orchestration core this extends) and Phase 44 (the generalized `toolSchemaWarnings` gate the deferred universe joins).*

Add the deferred-tools surface to the root package: `DeferredToolGroup`, `Conversation.DeferredTools`, and the synthesized `load_tools` meta-tool whose description carries the generated catalog (per-group name + blurb + bare tool names, no descriptions/schemas). The orchestrator gains a live toolset: the frozen name-sorted base (eager `Tools` + MCP tools + `load_tools`), plus loaded tools appended in load order; the tool loop re-collects it per iteration so a load made mid-turn is visible on the next round-trip. Dispatch distinguishes loaded/eager (run), deferred-but-unloaded (in-band corrective error naming `load_tools`, guessed input never executed, tool loaded as a side effect), and truly unknown (the existing Decision 10 error result, unchanged). Loading is monotonic and conversation-scoped. The `Send` gate extends to the whole universe: name uniqueness across eager ∪ MCP ∪ deferred ∪ the reserved `load_tools` name, deferred schemas through the same well-formedness check as `RawTool`, schema-translation warnings computed over everything. When `DeferredTools` is empty the synthesized tool does not exist and behavior is byte-identical to today. No provider adapter changes: the append-order guarantee is orchestrator-owned (the Google adapter's dialect re-sort stays as is).

End state: root-package implementation plus co-located tests against the existing fake-provider harness; every id below covered by a clearly-named test.

**Done when:** the offline Verification ids are each covered by a clearly-named test and the suite is green per design Conventions —

- R-9RQ8-9G3W — non-empty `DeferredTools` synthesizes `load_tools` whose description carries every group's name, blurb, and bare tool names — and no deferred tool's own description or schema.
- R-9SY4-N7UL — empty/nil `DeferredTools` produces no `load_tools` and requests identical to a `Conversation` without the field.
- R-D5PT-82VU — a `load_tools` call naming deferred tools puts them (full description + schema) in the next round-trip's request tools within the same turn, and a subsequent native call dispatches to the real `Call`.
- R-D6XP-LUMJ — a successful `load_tools` result carries each named tool's description and input JSON Schema.
- R-D85L-ZMD8 — a mixed valid/unknown-names call loads the valid ones, reports each unknown per-name in the result, and continues the turn (not a terminal stream error).
- R-D9DI-DE3X — a tool loaded in one `Send` is present in a later `Send`'s request tools on the same `Conversation`; no unload path exists.
- R-DALE-R5UM — a direct call to a deferred-but-unloaded tool yields `ToolResultBlock{IsError: true}` naming `load_tools`, never invokes the tool's `Call` with the guessed input, and loads the tool as a side effect (present next round-trip).
- R-DE93-WH2P — a name neither eager, loaded, nor deferred still yields the in-band `unknown tool` error result and the turn continues.
- R-DBTB-4XLB — across a turn's round-trips with loads, request tool order is the frozen name-sorted base then loaded tools in load order; the pre-existing prefix's byte serialization is identical before and after each load.
- R-DD17-IPC0 — a duplicate name anywhere in the combined universe (including the reserved `load_tools`), or an invalid deferred schema, surfaces `ErrInvalidConfig` via the `Stream` at the `Send` boundary, `History` unchanged, no provider call.

(The live real-substrate id R-DFH0-A8TE is Phase 47.)
