# Phase 49 — `load_tools` accepts group names

*Realizes design Decision 23 (amended settled choice: exact tool **or group** names). Depends on Phase 46 (deferred tools & the built-in `load_tools`).*

Live use (the first deferred-tools run from the ikigenba prompts service) showed a model's natural first `load_tools` call passes the **group** name ("crm") rather than an exact tool name, burning a corrective round-trip for no safety. Per the amended D23, `load_tools` name resolution widens: each requested name resolves first as a deferred tool name; failing that, a name matching a `DeferredToolGroup.Name` loads every tool in that group; a name matching neither remains a per-name unknown. A name that is both a tool and a group loads only the tool. The generated `load_tools` description states both accepted forms. Everything else is untouched: monotonic conversation-scoped loading, frozen-base/append ordering, the deferred-miss corrective path, and the `Send` validation gate (group names stay outside the uniqueness universe).

End state: `deferred_tools.go` resolves group names in `load_tools` calls and its generated description says so; the fake-provider test suite covers the three new behaviors.

## Done when

The suite is green and every id below is covered by a clearly-named test:

- **R-B5BR-U5M1** — a `load_tools` call naming a group loads every tool in that group (result text carries each tool's description + input schema; the group's tools appear in the next round-trip's request tools in group order; a native call to one dispatches to its real `Call`), and the generated `load_tools` description states that a group name loads the whole group.
- **R-B6JO-7XCQ** — one call mixing a tool name, a group name, and an unknown name loads the tool and the group's tools, reports only the unknown name, and the turn continues.
- **R-B7RK-LP3F** — a name that is both a deferred tool's name and another group's `Name` loads only the tool, never the group.
