# AgentKit — Plan Status

The manifest. One line per phase, in build order — the **only** place a phase's status marker lives. Each phase line begins with the literal word `Phase` and carries a done-marker (U+2705) or not-started-marker (U+2B1C). The build loop finds the next work with `grep -nE '^Phase .* ⬜' project/plan/STATUS.md | head -1`, reads only that phase's `project/plan/phase-NN.md`, and on completion flips that phase's one marker here to done. Nothing else in this file or any phase file is edited at build time. Append a new line (and a new phase file) to extend. (This paragraph deliberately carries no bare status glyph, so the anchored grep matches only phase lines.)

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
Phase 31  ✅  realizes D21                              — Shared `internal/retry` executor: de-duplicate the four retry copies
Phase 32  ✅  realizes D22                              — `ToolSchemaLimiter` capability interface: kill the `"google"` name-dispatch
Phase 33  ✅  realizes —                                — Release: patch version bump to `v0.1.3`
Phase 34  ✅  realizes D3                               — Audit A: de-tautologize the tool-use/result pairing test (R-IKKQ-Z3B4)
Phase 35  ✅  realizes D2                               — Audit A: observe early-break body close + no goroutine leak on real HTTP (R-CCI4-0UEA)
Phase 36  ✅  realizes D6                               — Audit A: exercise a genuine carried-over reasoning value (R-B96T-WUUR)
Phase 37  ✅  realizes D15                              — Audit A: falsifiably pin the nil-Log no-write guarantee (R-PM3H-UYFS)
Phase 38  ✅  realizes D2                               — Audit B: assert MessageDone mirrors History for reasoning+tool_use shapes (R-CBA7-N2NL)
Phase 39  ✅  realizes D3                               — Audit B: exercise parallel reasoning→tool positional binding (R-IPGC-I69W)
Phase 40  ⬜  realizes D10                              — Audit B: deterministic re-ordering on MCP attach/detach (R-6W63-I4FW)
Phase 41  ⬜  realizes D11                              — Audit B: MCP discovery fails fast on 400/401/403, retries 5xx (R-6XDZ-VW6L)
Phase 42  ⬜  realizes D17                              — Audit B: unreachable MCP server and cross-server isolation (R-6L70-26RN)
Phase 43  ⬜  realizes D11                              — Audit B: reconcile streaming-idempotency claims to the message-granular seam (R-P61J-IHKB, R-Y878-6UDJ, R-6YLW-9NXA)
