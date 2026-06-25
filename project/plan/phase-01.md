# Phase 1 — Neutral data model and tool-call ID minter
*Realizes design Decision 3 (canonical message & block data model). Depends on nothing (first phase).*

The module `github.com/ikigenba/agentkit` exists (package `agentkit` at the module root, `go.mod` declaring Go 1.26) and defines the provider-agnostic data model: `Role`, `Message`, the sealed `Block` interface and its four concrete types (`TextBlock`, `ToolUseBlock`, `ToolResultBlock`, `ReasoningBlock`), plus a tool-call ID minter producing IDs in Anthropic's strict charset `^[a-zA-Z0-9_-]+$`. The value types other phases build on — `GenSettings`/`ReasoningEffort`/`Warning` (D6) and `Usage` (D8) — are defined here as type declarations; their behavioral proofs live in the orchestration and adapter phases.

**Done when:** R-IKKQ-Z3B4 is covered (a minted `ToolUseBlock.ID` matches the charset and the paired `ToolResultBlock.ToolUseID` equals it); the package compiles and the sealed unions are enforced; suite green.

