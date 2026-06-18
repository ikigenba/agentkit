# AgentKit — Plan

**Authority: construction order and history.** This document owns the order AgentKit is built in and the record of what has been built. Unlike product (`docs/product.md`) and design (`docs/design.md`), which are rewritten in place to stay authoritative for the current state, the plan is **append-only**: phases are added at the bottom and marked done as they land; completed phases are never rewritten or deleted, so the plan doubles as the construction history. To extend the project later, update product and design in place, then append a new phase here.

**One phase = one package = one accumulating context.** Each phase is a single coherent unit — almost always one package — built in one accumulating context against product and design, reading only that package's design Decisions and the *interfaces* (not internals) of the packages it depends on. This is what keeps every phase the size of a small standalone tool no matter how large the project grows. Because the architecture is one large root `agentkit` package plus leaf sub-packages, the root work is split across several phases (it exceeds one context); each sub-package is its own phase. Some verification ids are table-driven or cross-provider (the error matrix, usage mapping, model registries, reasoning-`Opaque` capture, generation-settings mapping, and `R-C8UE`): each contributing phase covers its own provider's slice, and the id is fully discharged when its last contributing phase lands.

**Done bar.** A phase is **done** when every Verification id in the design Decisions it realizes (its slice of any shared id) is covered by a clearly-named test and the suite is green — measured against the per-Decision **Verification** lists in `docs/design.md` (minted `R-XXXX-XXXX` ids, one behavior each).

## Status

Not started; the workspace holds product, design, and this plan — no code yet.

## Phases

### Phase 1 — Neutral data model and tool-call ID minter · ✅ done
*Realizes design Decision 3 (canonical message & block data model). Depends on nothing (first phase).*

The module `github.com/ikigenba/agentkit` exists (package `agentkit` at the module root, `go.mod` declaring Go 1.26) and defines the provider-agnostic data model: `Role`, `Message`, the sealed `Block` interface and its four concrete types (`TextBlock`, `ToolUseBlock`, `ToolResultBlock`, `ReasoningBlock`), plus a tool-call ID minter producing IDs in Anthropic's strict charset `^[a-zA-Z0-9_-]+$`. The value types other phases build on — `GenSettings`/`ReasoningEffort`/`Warning` (D6) and `Usage` (D8) — are defined here as type declarations; their behavioral proofs live in the orchestration and adapter phases.

**Done when:** R-IKKQ-Z3B4 is covered (a minted `ToolUseBlock.ID` matches the charset and the paired `ToolResultBlock.ToolUseID` equals it); the package compiles and the sealed unions are enforced; suite green.

### Phase 2 — Error model · ✅ done
*Realizes design Decision 7 (the error model). Depends on Phase 1.*

The thirteen sentinel category vars, the rich `*Error` struct (all fields), and its `Error()`/`Is`/`Unwrap` methods exist, so `errors.Is(err, ErrX)` is the single branching idiom over provider failures. The per-provider classification matrix, the verbatim-`Raw` capture, MCP attribution, and the `*Error`-versus-bare-sentinel distinction are proven later, where the producing code and the other sentinel families exist.

**Done when:** R-BVYY-B2AX is covered (`errors.Is` returns true for a matching sentinel and false for a non-matching one); suite green.

### Phase 3 — Pricing and cost engine · ✅ done
*Realizes design Decision 16 (baked-in pricing & cost), struct and math only. Depends on Phase 1.*

`Pricing`, `RateTier`, `Cost`, `Cost.USD()`, and `Pricing.Cost(Usage)` exist — tier selection by a turn's total input tokens and exact nano-USD integer rating of every bucket (reasoning billed at the output rate). The per-provider rate tables and registry-completeness ship with the adapter phases; cumulative `TotalCost` is wired in orchestration/logging.

**Done when:** R-PTEW-5KVY, R-V2SM-WC8V, and R-PX2L-AW41 are covered (rating math against synthetic `Pricing` literals, tier selection by input size, USD conversion); suite green.

### Phase 4 — Tool definition and registration surface · ⬜ not started
*Realizes design Decision 4 (tool definition & registration). Depends on Phase 1.*

The sealed `Tool` interface, the generic `NewTool[In]` constructor (JSON Schema derived once from `In` via `invopop/jsonschema` and cached), and the `RawTool` escape hatch exist; a `Tool`'s `Call` decodes input and invokes the consumer's `fn`, returning the string result. Send-boundary validation (bad `RawTool` schema, duplicate names), the Gemini lossy-schema conversion, and the MCP-schema warning are proven in later phases.

**Done when:** R-WYZP-N2VB, R-X07M-0UM0, and R-X2NE-SE3E are covered (schema reflects the struct and is byte-stable/cached; typed and raw `Call` decode-and-invoke paths); suite green.

### Phase 5 — Orchestration core: SPI, Stream, Send, and the tool loop · ⬜ not started
*Realizes design Decision 9 (provider adapter SPI), Decision 1 (consumer surface), Decision 2 (streaming surface), and Decision 10 (orchestration layer); wires Decision 16 cost into `Stream`/`Conversation`. Depends on Phase 1, Phase 2, Phase 3, and Phase 4.*

This is the heart of the library (the largest root phase). The exported SPI types (`Provider`, `Request`, `RoundTrip` and its accessors, `FinishReason`), the `Conversation` struct (incl. `MaxToolIterations`) and `Send`, and the `Stream` (`Events`/`Err`/`Usage`/`Warnings`/`Cost`) all exist. `Send` runs the full turn algorithm above the SPI: append the user message at a recorded rollback point, build a `Request` with tools in name-sorted deterministic order, loop `RoundTrip` calls forwarding `TextDelta`/`ReasoningDelta` and emitting `ToolUse`/`ToolResult`/`MessageDone`, run matching tools sequentially (unknown tool → `IsError` result fed back), feed results and loop to a tool-free assistant message, cap at `MaxToolIterations` (default 1000), accumulate usage and cost. Boundary validation (missing provider/model, empty `userText`, duplicate tool names, invalid `RawTool` schema, unknown model via the registry validity gate), atomic `History` rollback, and the non-re-entrant stream-live guard all hold. The boundary sentinels `ErrInvalidConfig`/`ErrInvalidInput` and the orchestration sentinels `ErrToolLoopLimit`/`ErrStreamPending` are declared here. Verified against in-package fake `Provider` doubles and fake `Tool`s — no real adapter required; the provider-switch behavior is exercised with two fake doubles and re-asserted against real backends in Phase 11.

**Done when:** R-ZWV0-CY54, R-ZELD-OQNG, R-ZZAT-4HMI, R-00IP-I9D7 (D1); R-C7MI-HRFI, R-C8UE-VJ67 (orchestration ordering/completeness slice), R-CBA7-N2NL, R-CCI4-0UEA, R-CDQ0-EM4Z (D2); R-SX1B-XRK2, R-SZH4-PB1G, R-X1FI-EMCP (D4 boundary/loop); R-7GGH-BPYN (D5 model validity); R-02PH-VYKB, R-03XE-9QB0 (D9); R-VV9Y-GMKH, R-VWHU-UEB6, R-VXPR-861V, R-VYXN-LXSK, R-W05J-ZPJ9, R-W1DG-DH9Y, R-XZNX-IG6O, R-Y4JJ-1J5G (D10) are covered; suite green.

### Phase 6 — Retry and backoff · ⬜ not started
*Realizes design Decision 11 (retry & backoff policy). Depends on Phase 5.*

`RetryPolicy` and `Conversation.Retry` exist, and the orchestrator wraps each `RoundTrip` with the single cross-provider retry policy: full-jitter exponential backoff over the fixed retryable category set, per-round-trip budget, the no-retry-after-first-byte streaming-idempotency rule, server `Retry-After` honored (toggleable), context-aware waits, and an injectable unexported clock for deterministic tests. Verified against fake `Provider` doubles that fail N times then succeed, with the injected clock asserting attempt counts and delays.

**Done when:** R-P3LQ-QY2X, R-P4TN-4PTM, R-P61J-IHKB, R-Y878-6UDJ, R-P79F-W9B0, and R-P8HC-A11P are covered; suite green.

### Phase 7 — Structured JSONL event log and conversation lifecycle · ⬜ not started
*Realizes design Decision 15 (JSONL event log & lifecycle); completes the one-idiom error proof and cumulative cost. Depends on Phase 5 and Phase 6.*

`Conversation.Log io.Writer`, the `LogRecord` schema, `Close()`, `TotalUsage()`, `TotalCost()`, and the `ErrClosed` sentinel exist. A turn writes one JSONL record per protocol event in stream order (`Time` from the injected clock, `Seq` monotonic), writes are best-effort, `Close()` emits exactly one cumulative `summary` (idempotent) and `Send`-after-`Close` returns `ErrClosed`. With every sentinel family now present (provider, orchestration, boundary, and `ErrClosed`), the `*Error`-versus-bare-sentinel distinction is fully provable.

**Done when:** R-PH7W-BVH0, R-PIFS-PN7P, R-PJNP-3EYE, R-PKVL-H6P3, R-PM3H-UYFS, R-PNBE-8Q6H, R-POJA-MHX6, R-PPR7-09NV, R-PVUO-X4DC (TotalCost + summary), R-I5VJ-CTXE, and R-7CYE-KS40 are covered; suite green.

### Phase 8 — Anthropic adapter (plus shared HTTP/SSE internals and the fake-server harness) · ⬜ not started
*Realizes design Decision 9, Decision 7, Decision 8, Decision 16, Decision 3, Decision 2, and Decision 13 (the Anthropic slice), and Decision 12 (`internal/sse`, `internal/httpx`). Depends on Phases 1 through 7.*

The shared internals `internal/httpx` (request execution, header/body helpers, `*http.Client` injection) and `internal/sse` (SSE framing) exist, and the `anthropic` sub-package implements the `Provider` SPI end-to-end over raw `net/http`: request build with system injection, the frozen stable prefix with a single default 5m `cache_control` breakpoint on the last prefix block, name-sorted tools, SSE parse with central partial-JSON tool assembly, the assembled `Message` (incl. `ReasoningBlock` whose `Opaque` is the Anthropic `signature`), `FinishReason`, usage mapping (derived `Total`, cache-write 5m/1h sub-split, `ReasoningOutput`=0), warnings, error classification (status then type), foreign-reasoning drop, `Name()`="anthropic", and the model registry + pricing table (verbatim from the design's rate table) with exported model constants. This phase also stands up the fake-`httptest`-server harness and golden-SSE replay (`-update`) reused by the later adapters.

**Done when:** (Anthropic slices unless noted): R-H3PK-QFG3, R-01HL-I6TM (dependency isolation), R-BUR1-XAK8, R-BX6U-OU1M, R-BYER-2LSB, R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7, R-VDY4-AP7H, R-V1KQ-IKI6, R-IN0J-QMSI, R-XW08-D4YL, R-055A-NI1P, R-W2LC-R90N, R-P5U3-5CFZ, R-P71Z-J46O, R-P89V-WVXD (Opus 4.8 half), R-PBXL-275G, R-C8UE-VJ67 (Anthropic fragmented-JSON assembly slice), R-WKTI-LIIE, R-WM1E-ZA93, R-WJLM-7QRP are covered; suite green.

### Phase 9 — OpenAI adapter (Responses API) · ⬜ not started
*Realizes design Decision 9, Decision 7, Decision 8, Decision 16, Decision 3, Decision 6, and Decision 13 (the OpenAI slice). Depends on Phases 5 through 8 (reuses `internal/sse`, `internal/httpx`, and the harness).*

The `openai` sub-package implements the SPI over the Responses API on raw `net/http`: every request carries `store:false` and `include:["reasoning.encrypted_content"]` (fixed, never a consumer knob), the returned `encrypted_content` populates `ReasoningBlock.Opaque` and replays verbatim, `call_id` is normalized into the neutral ID, SSE parse with central assembly, usage mapping (`InputUncached`=prompt−cached, `Output`=output−reasoning, `ReasoningOutput` populated, native total asserted == bucket sum), error classification, and the registry + pricing (gpt-5.5-pro with `CacheReadInput`==`InputUncached`, tiered gpt-5.5 and gpt-5.4, gpt-5.4-mini, gpt-5.4-nano) with exported constants.

**Done when:** (OpenAI slices): R-H3PK-QFG3, R-XR4M-U1ZT, R-BUR1-XAK8, R-BX6U-OU1M, R-BYER-2LSB, R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7, R-VDY4-AP7H, R-V1KQ-IKI6, R-XW08-D4YL, R-055A-NI1P, R-P5U3-5CFZ, R-P71Z-J46O, R-C8UE-VJ67 (OpenAI assembly slice), R-V2SM-WC8V (gpt tiered slice) are covered; suite green.

### Phase 10 — Shared OpenAI-compatible internals and the Z.ai adapter · ⬜ not started
*Realizes design Decision 5 (Z.ai first-classness), Decision 9, Decision 7, Decision 8, Decision 16, Decision 6, and Decision 13 (the Z.ai slice), and Decision 12 (`internal/openaicompat`). Depends on Phases 5 through 8.*

The shared, non-consumer-importable `internal/openaicompat` Chat-Completions adapter exists (request build, SSE parse, central assembly, usage mapping, pluggable error classifier), and the public `zai` sub-package constructs it with Z.ai's baked-in base URL (`https://api.z.ai/api/paas/v4/`) — the consumer supplies only an API key. `zai` classifies errors by Z.ai's numeric `code`, maps `reasoning_content` into `Opaque` with `ReasoningOutput`=0, degrades a forced `tool_choice` to `auto` with a `Warning`, labels `Error.Provider`="zai", and ships the registry + pricing (glm-5.2/5.1/4.7/4.6) with exported constants. `internal/openaicompat` carries no public surface and no ids of its own.

**Done when:** (Z.ai slices): R-H4XH-476S, R-BZMN-GDJ0, R-P9HS-ANO2, R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7, R-VDY4-AP7H, R-V1KQ-IKI6, R-XW08-D4YL, R-055A-NI1P, R-P5U3-5CFZ, R-P71Z-J46O, R-C8UE-VJ67 (Z.ai assembly slice) are covered; suite green.

### Phase 11 — Google adapter and the cross-provider portability matrix · ⬜ not started
*Realizes design Decision 9, Decision 7, Decision 8, Decision 16, Decision 6, Decision 4, Decision 3, and Decision 13 (the Google slice), plus the cross-provider switch ids of Decision 1, Decision 3, and Decision 5. Depends on Phases 5 through 10 (all other adapters exist).*

The `google` sub-package implements the SPI over the Gemini API on raw `net/http`, owning its own response parse: best-effort JSON Schema → `*genai.Schema` conversion (drops `$ref`/`additionalProperties`/`oneOf` without erroring), the `ReasoningEffort` ordinal mapped to `thinkingLevel`/`thinkingBudget`, `thoughtSignature` into `Opaque` with the positional `BoundToID` binding for parallel tool calls, usage mapping (thoughts → `ReasoningOutput`, cached subtracted from prompt, cache fields 0), error classification with `RetryInfo.retryDelay` → `RetryAfter`, and the registry + pricing (gemini-2.5-flash, tiered gemini-2.5-pro, gemini-3.5-flash, gemini-3.1-flash-lite, tiered gemini-3.1-pro-preview with its preview-caveat doc-comment) with exported constants. With all four adapters now present, this phase also proves the cross-provider portability matrix.

**Done when:** (Google slices and cross-provider): R-H3PK-QFG3, R-X3VB-65U3, R-BUR1-XAK8, R-BX6U-OU1M, R-BYER-2LSB, R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7, R-VDY4-AP7H, R-V1KQ-IKI6, R-XW08-D4YL, R-IPGC-I69W, R-055A-NI1P, R-P5U3-5CFZ, R-P71Z-J46O, R-P89V-WVXD (Gemini 2.5 Pro half), R-V2SM-WC8V (gemini tiered slice), R-C8UE-VJ67 (Gemini whole-JSON assembly slice — completing it across all four providers); and the cross-provider ids R-ILSN-CV1T, R-IO8G-4EJ7, R-H65D-HYXH, R-00IP-I9D7 (re-asserted against two real backends) are covered; suite green.

### Phase 12 — Internal MCP Streamable-HTTP client · ⬜ not started
*Realizes design Decision 17 (the MCP client portion), Decision 12 (`internal/mcp`), and Decision 13 (fake MCP server). Depends on Phase 1, Phase 2, and the `internal/sse` helper from Phase 8.*

The non-consumer-importable `internal/mcp` raw-HTTP Streamable-HTTP JSON-RPC client exists, targeting MCP revision `2025-11-25` and implementing exactly the four calls (`initialize`, `notifications/initialized`, `tools/list` paginated to exhaustion, `tools/call`). It reads the JSON-RPC response from whichever of `application/json` or `text/event-stream` arrives (reusing `internal/sse`), echoes any `Mcp-Session-Id` and always sends `MCP-Protocol-Version: <negotiated>`, transparently re-initializes on a `404` for idempotent discovery, and re-establishes (but does not replay) on a `404` mid-`tools/call`. Verified against a fake `httptest` MCP server.

**Done when:** R-711P-17EO, R-6MEW-FYIC, R-6OUP-7HZQ, and R-6RAH-Z1H4 are covered; suite green.

### Phase 13 — MCP integration into the orchestrator · ⬜ not started
*Realizes design Decision 17 (the MCP consumer surface and merge) plus its slices of Decision 7, Decision 10, Decision 11, and Decision 4. Depends on Phase 5, Phase 6, Phase 11, and Phase 12.*

`Conversation.MCPServers []MCPServer` and the `MCPServer` type exist. At the `Send` boundary, AgentKit connects, handshakes, and runs `tools/list` whenever the attached set changes, caching the live session and tool snapshot (unexported) and re-discovering on change; each discovered tool is wrapped as an ordinary `Tool` (its `JSONSchema()` the server's third-party `inputSchema`), exposed as `<serverName>_<originalName>` sanitized to `^[a-zA-Z_][a-zA-Z0-9_]{0,63}$` (hash suffix on overflow) and routed back by a stored `(server, originalName)` binding, then merged into the one name-sorted `[]Tool` the loop drives. A name collision surfaces `ErrInvalidConfig` at the boundary; a `result` with `isError:true` feeds back a `ToolResultBlock{IsError:true}` while a JSON-RPC `error` or transport failure surfaces via `Stream.Err()`; MCP failures carry the `MCPServer` attribution with no new sentinel; a Gemini-unsupported MCP schema converts best-effort with a `Warning`; MCP discovery is retried but `tools/call` is not; `Close()` best-effort `DELETE`s each live session. One unreachable server is isolated to its own boundary.

**Done when:** R-6GBE-J3SV, R-6HJA-WVJK, R-6IR7-ANA9, R-6L70-26RN, R-6NMS-TQ91, R-6Q2L-L9QF, R-6SIE-CT7T, R-6W63-I4FW, R-6ZTS-NFNZ, R-6TQA-QKYI, R-6UY7-4CP7, R-6XDZ-VW6L, and R-6YLW-9NXA are covered; suite green.

### Phase 14 — Example REPL · ⬜ not started
*Realizes design Decision 14 (the example REPL). Depends on Phase 5, Phase 8, and at least one further adapter (Phase 9, 10, or 11).*

A runnable `examples/repl/` program exists as a thin consumer of the public API only: it reads stdin, calls `conv.Send`, prints `TextDelta`s as they stream, registers a `bash` tool via `NewTool`, and supports `/model <provider>:<name>` to swap `conv.Provider`+`conv.Model` mid-session with `History` retained. It uses nothing internal — if it cannot build cleanly against the public surface, the surface is wrong.

**Done when:** R-WCNR-SQFT and R-WDVO-6I6I are covered; suite green.
