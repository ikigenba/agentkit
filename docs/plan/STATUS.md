# AgentKit — Plan Status

The manifest. One line per phase, in build order — the **only** place a phase's status marker lives. Each phase line begins with the literal word `Phase` and carries a done-marker (U+2705) or not-started-marker (U+2B1C). The build loop finds the next work with `grep -nE '^Phase .* ⬜' docs/plan/STATUS.md | head -1`, reads only that phase's `docs/plan/phase-NN.md`, and on completion flips that phase's one marker here to done. Nothing else in this file or any phase file is edited at build time. Append a new line (and a new phase file) to extend. (This paragraph deliberately carries no bare status glyph, so the anchored grep matches only phase lines.)

Phase 01  ✅  realizes D3                              — Neutral data model and tool-call ID minter
Phase 02  ✅  realizes D7                              — Error model
Phase 03  ✅  realizes D16                             — Pricing and cost engine
Phase 04  ✅  realizes D4                              — Tool definition and registration surface
Phase 05  ✅  realizes D9,D1,D2,D10,D16                — Orchestration core: SPI, Stream, Send, and the tool loop
Phase 06  ✅  realizes D11                             — Retry and backoff
Phase 07  ✅  realizes D15                             — Structured JSONL event log and conversation lifecycle
Phase 08  ✅  realizes D9,D7,D8,D16,D3,D2,D13,D12      — Anthropic adapter (plus shared HTTP/SSE internals and the fake-server harness)
Phase 09  ✅  realizes D9,D7,D8,D16,D3,D6,D13          — OpenAI adapter (Responses API)
Phase 10  ✅  realizes D5,D9,D7,D8,D16,D6,D13,D12      — Shared OpenAI-compatible internals and the Z.ai adapter
Phase 11  ✅  realizes D9,D7,D8,D16,D6,D4,D3,D13,D1,D5 — Google adapter and the cross-provider portability matrix
Phase 12  ✅  realizes D17,D12,D13                     — Internal MCP Streamable-HTTP client
Phase 13  ✅  realizes D17,D7,D10,D11,D4               — MCP integration into the orchestrator
Phase 14  ✅  realizes —                               — Example REPL · extracted
Phase 15  ✅  realizes D9                              — Fix: Anthropic streamed tool-call input assembly (placeholder concatenation)
Phase 16  ✅  realizes D9                              — Fix: OpenAI replayed reasoning item missing the required `summary` field
Phase 17  ✅  realizes D9                              — Fix: OpenAI replayed tool-call `arguments` serialized as object, not string
Phase 18  ✅  realizes D9,D12                          — Fix: shared `internal/openaicompat` replayed tool-call `arguments` serialized as object, not string
Phase 19  ✅  realizes D9                              — Fix: Google adapter drops co-resident `functionCall`/text when a part carries a `thoughtSignature`
Phase 20  ✅  realizes D9                              — Fix: Google adapter nests the replayed `thoughtSignature` inside `functionCall` instead of on the part
Phase 21  ✅  realizes D16,D6                          — Native reasoning introspection surface: descriptor types and per-package specs
Phase 22  ✅  realizes D6,D9                           — Native reasoning value and warn-and-default lowering: atomic retype across root and all adapters
Phase 23  ✅  realizes D9                              — Fix: Anthropic replayed thinking block serializes its reasoning text in `text` instead of `thinking`
Phase 24  ✅  realizes D2,D9,D10                       — Migrate to message-granular delivery: remove the consumer delta surface
Phase 25  ✅  realizes D7                               — Cover the Anthropic in-stream SSE `error` event (200-then-`event: error`)
Phase 26  ✅  realizes D9                               — Fail-loud `default` in adapter block translation; retire the orphan verification id
Phase 27  ✅  realizes D18,D19,D20                       — Embeddings root surface: Embedder, Embed, the EmbeddingProvider SPI, and the registry value types
Phase 28  ✅  realizes D19,D20,D18                       — OpenAI embeddings adapter (/v1/embeddings), the internal/openaicompat embeddings variant, and the OpenAI embedding registry
Phase 29  ✅  realizes D19,D20,D18                       — Google embeddings adapter (:batchEmbedContents) and registry; discharge the cross-provider embeddings ids
Phase 30  ✅  realizes D9                               — Fix: Anthropic replayed thinking block elides empty reasoning text (`thinking,omitempty`)
