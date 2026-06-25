# Phase 40 — Exercise deterministic re-ordering on MCP attach/detach (complete R-6W63-I4FW)

*Realizes design Decision 10 (the orchestration layer — the attach/detach re-ordering clause of R-6W63-I4FW). Depends on Phase 13 (MCP integration into the orchestrator) and Phase 05 (the deterministic name-sorted tool serialization).*

Close audit Finding B/`R-6W63-I4FW` (`project/audit/findings.md`, section B; sketch in `project/audit/b-section-fixes.md`): the claim has three parts — MCP-discovered tools are merged with custom tools in one deterministic name-sorted order, **stable across turns while the attached set is unchanged**, **and re-ordered deterministically when a server is attached/detached**. The existing test (`mcp_integration_test.go:399`) asserts the stable-across-turns part (two turns, same order). The attach/detach **re-ordering** part is not exercised — the detach test under `R-6SIE-CT7T` only checks tool *removal* (the list goes empty), never a re-sort among multiple remaining tools.

Production re-discovers and re-sorts on any server-set change: `resolveMCPTools` keys the tool cache on the server set (`mcpServerSetKey`), and `validateAndSortTools` name-sorts the merged `custom + mcp` list every resolve. Observable, untested for the multi-tool re-order.

End state:

- A clearly-named test (or an extension of the deterministic-order test) starts with a known merged order across two same-set turns, then **attaches an additional list-only MCP server whose exposed tool sorts into the middle** of the existing order, runs another `Send`, and asserts the new provider call's tool order is the deterministically re-sorted merged list (a genuine re-order, not an append).
- A symmetric detach assertion confirms that removing one of several servers re-sorts the remaining merged set (distinct from the existing empty-list removal check).
- The existing stable-across-turns assertions are retained.

**Done when:** R-6W63-I4FW is covered by a clearly-named test asserting the merged tool order is deterministically re-sorted when a server is attached and when one of several is detached (beyond mere removal), the other D10 ids stay green, and the suite is green per design Conventions. No production change is expected; if the re-order assertion exposes a real ordering defect, fix the production code.
