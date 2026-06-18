# AgentKit — Research

**Status: non-contractual.** This document informs the *author* of `docs/design.md`; nothing downstream (the autonomous build) reads it. It records options, prior art, constraints, and recommendations as of **2026-06-17**. Design remains the single authority for *how*; where this doc recommends a mechanism, design may adopt, refine, or reject it. Edit this doc in place as the product evolves — never append a log.

**Model-list re-verification (2026-06-17).** The supported-model set was re-verified against each provider's official live model/pricing/deprecation pages. Net result: the design registry (D16) needed reconciling — see the reconciliation block in §6.5 (now applied). Key facts baked in below: OpenAI dropped `o3`/`o4-mini` (deprecated) and `gpt-5.4-nano` *does* exist; Google's 3.x Pro is **preview-only** (`gemini-3.1-pro-preview` — no GA `gemini-3.1-pro`); Anthropic `claude-fable-5` was **dropped from the curated set** — it is globally disabled since 2026-06-12 (export control) and so cannot be served; and Opus 4.8 reasoning **cannot be disabled**.

**MCP support added (2026-06-17, product change).** The product now promises **remote MCP (Model Context Protocol) tool servers**: AgentKit acts as an MCP **client** — the consumer attaches one or more remote MCP servers (network transport only; AgentKit spawns **no** subprocesses, so local stdio servers are out of scope), AgentKit connects, discovers each server's tools, and feeds them into the *same* automatic tool loop as custom tools, uniformly across all four providers. Only **tools** are surfaced (MCP resources/prompts deferred); the consumer names each server and that name **prefixes** its tools; credentials are supplied **explicitly**; **no interactive OAuth**. Servers attach/detach **between turns**, mirroring provider/model switching. This adds a new research dimension — see the new **§9** (protocol, transport, integration, auth, failure-mapping) — and new recommendation items in §10.

**Hard constraint (user directive, 2026-06-17): no third-party libraries.** Using a library is **not an option to consider** — AgentKit is built on the **Go standard library only** (`net/http`, `encoding/json`, `iter`, …). This is no longer a tradeoff to weigh: it **decides** every wrap-vs-raw question in this doc. All four provider adapters (Anthropic, Google, OpenAI, Z.ai) and the MCP client are **raw HTTP**; SSE parsing, partial-JSON tool-call accumulation, retry/backoff, error/usage extraction, and struct→JSON-Schema generation are all **hand-rolled**. The official provider SDKs, the MCP `go-sdk`, `invopop/jsonschema`, and `cenkalti/backoff` are all **excluded** — they appear below only as reference for *what* behavior AgentKit must re-implement. The former "open question" §11 is consequently **closed** (raw HTTP), and §9.2 / §4.3 are settled the same way.

The product (`docs/product.md`) fixes the target: a Go 1.26 library, module `github.com/ikigenba/agentkit`, starting `v0.1.0`, giving **one uniform surface** for a tool-using, multi-turn, **text-only**, streaming chat across multiple providers — provider+model is configuration, switchable mid-conversation. **Dollar-cost accounting is in scope** (product change): AgentKit ships baked-in per-model pricing and reports per-turn and cumulative cost; because the supported-model set is closed and curated, every supported model has a pricing entry by construction and cost is always available (no "unavailable" state). Out of scope: images/audio, persistence, ambient credentials. Embeddings are a committed *later* phase, not v1.

**Providers researched: Anthropic, Google, OpenAI, and Z.ai (Zhipu/BigModel, GLM family) — treated as four equal options.** ✅ **Scope note (resolved):** Z.ai is now a promised, first-class v1 provider — `docs/product.md` names all four, and the design exposes it as the `zai` sub-package (`zai.New(apiKey)`, base URL internal), a first-class peer rather than a generic `openaicompat` endpoint. The first-classness principle: a provider reached via API-compatibility is still first-class on the public surface; the OpenAI-compatible reuse lives in `internal/openaicompat`. Practically, Z.ai remains the cheapest provider to add: it is an **OpenAI Chat-Completions-compatible** endpoint, so the internal adapter largely reuses the OpenAI Chat-Completions path (see §2.4, §2.3).

This is a **greenfield** repo — only `docs/product.md` exists (no Go code, no `go.mod`, not yet a git repo). So nearly all research is external: current provider APIs, prior art, and the core abstraction.

---

## 1. The central finding

Structural unification across the providers is **genuinely achievable and clean for text chat**. Every serious prior-art abstraction confirms it. The irreducible leaks cluster in exactly four places — **streaming tool-call deltas, tool-call identity, reasoning/thinking state, and token/usage accounting**. AgentKit's *text-only* scope drops images and persistence — but it does **not** get to drop cost (now in scope, computed from baked-in per-model rates against the usage buckets) and does **not** get to drop reasoning, because the v1 target models are all newest-generation **reasoning** models and three of four providers *require* reasoning state to be echoed back across tool-use turns (see §7). So **three** of the four leak zones are squarely in play and are where the design must concentrate: **tool-call identity (§5), reasoning-state preservation (§7), and token/usage + caching accounting (§6.3, §8)**. Get those three right and the rest of the uniform surface falls out naturally.

The recommended canonical model is **Anthropic-shaped**: a conversation is `[]Message`; each `Message` is a `Role` plus an ordered `[]Block`; blocks are `text` / `tool_use` / `tool_result`. Anthropic's content-block shape is the richest of the providers and the cleanest to down-convert from. OpenAI's Responses API, Google's `Part` struct, and Z.ai's OpenAI-compatible Chat Completions shape all map onto it; the provider adapter owns the translation.

**The four providers split into two implementation families.** Three are *native* protocols requiring bespoke adapters: Anthropic (Messages API), Google (Gemini `genai`), OpenAI (Responses API). The fourth — **Z.ai/GLM — is OpenAI-Chat-Completions-compatible**, so it is not a fourth bespoke adapter but a **near-clone of an OpenAI Chat-Completions adapter** parameterized by base URL + key + model, with three small deltas (Zhipu-shaped error envelope, GLM `thinking`/`reasoning_content` fields, `tool_choice=auto`-only). This is the strongest single argument for building an OpenAI **Chat-Completions** adapter (not only Responses) and for designing the OpenAI-compatible path around a **configurable base URL** — it makes Z.ai (and any other OpenAI-compatible endpoint) nearly free.

**MCP rides on the existing tool abstraction — it is not a fifth provider.** The MCP addition (§9) does **not** introduce a new leak zone; it introduces a new *capability source*. MCP tools are discovered over the wire and then become ordinary entries in the same `Tool` registry and the same auto-loop as custom tools — the model and the providers never know the difference. So MCP's work concentrates in three already-familiar places plus one new transport concern: (1) **name prefixing + collision detection** (reuses the strict tool-name charset from §5), (2) **JSON-Schema translation** — MCP `inputSchema` is arbitrary third-party JSON Schema, so it hits the *same* lossy Gemini converter as custom tools (§4.3), only now with schemas AgentKit does not control, (3) **failure-channel mapping** into the existing error taxonomy (§6.1) — the MCP `isError` result-vs-protocol-error split maps exactly onto AgentKit's "tool returns an error result (fed back to model)" vs "transport failed (uniform error)" distinction — and the one genuinely new piece, (4) a **remote Streamable-HTTP MCP client** (§9.1–9.2). No new error sentinel and no change to the canonical message model are needed.

---

## 2. Provider API surfaces

### 2.1 Anthropic — Messages API

- **Endpoint/auth.** `POST https://api.anthropic.com/v1/messages`; headers `x-api-key`, `anthropic-version: 2023-06-01`, `content-type: application/json`. Request: `model`, `max_tokens` (**required**), `messages[]`, optional **top-level `system`** (string or text-block array — NOT a message role), `temperature`, `stream`, `tools`, `tool_choice`.
- **Messages.** `{role: "user"|"assistant", content: string | ContentBlock[]}`. Blocks: `text {type,text}`, `tool_use {type,id,name,input}`, `tool_result {type,tool_use_id,content,is_error}`. `stop_reason ∈ end_turn | max_tokens | stop_sequence | tool_use | refusal`.
- **Tools.** `{name, description, input_schema}` where `input_schema` is **JSON Schema** (passes through nearly verbatim; optional `strict:true`). Model emits `tool_use` blocks with `stop_reason:"tool_use"`; consumer replies with a new **user** message carrying `tool_result` blocks keyed by `tool_use_id`. Parallel tool_use blocks can appear in one turn; all results go in one user message.
- **Streaming (SSE).** `message_start` (initial usage, input tokens) → per block `content_block_start` / N×`content_block_delta` / `content_block_stop` → `message_delta` (carries `stop_reason` + **cumulative** `usage.output_tokens`) → `message_stop`. Text via `text_delta`; **tool input via `input_json_delta.partial_json` string fragments — concatenate and parse only at `content_block_stop`**. `error` events (e.g. `overloaded_error`) can arrive mid-stream after a 200.
- **Errors.** `{type:"error", error:{type,message}, request_id}`; `request-id` header on every response. 400 `invalid_request_error`, 401 `authentication_error`, 402 `billing_error`, 403 `permission_error`, 404 `not_found_error`, 413 `request_too_large`, 429 `rate_limit_error`, 500 `api_error`, 504 `timeout_error`, **529 `overloaded_error`**. Retryable: 408/409/429/529 and ≥500.
- **Retry signals.** `retry-after` (seconds) on 429/529; rich `anthropic-ratelimit-*` headers (reset is RFC 3339).
- **Usage.** `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`. **Gotcha:** `input_tokens` counts only tokens *after the last cache breakpoint*; total input = `cache_read + cache_creation + input_tokens`.
- **Models (verified 2026-06-17 vs official models/pricing pages).** Curated set = `claude-opus-4-8` (most capable default, 1M ctx), `claude-sonnet-4-6`, `claude-haiku-4-5` — all three confirmed current and correctly priced (§6.5). Opus 4.8 is the safe default top tier. Snapshot-id nuance: Opus 4.8 / Sonnet 4.6 are genuinely **dateless pinned snapshots**, but **`claude-haiku-4-5` is an alias for the dated canonical `claude-haiku-4-5-20251001`** (both resolve). ⚠ **`claude-fable-5` was DROPPED from the curated set.** It is a valid, priced id but was globally DISABLED for ALL customers on 2026-06-12 under a US export-control directive (Anthropic could not segment foreign-national access in time; the pricing/models docs still call it "GA", so the docs are stale on availability). Because a supported model must be servable, and Fable 5's disablement is a global, indefinite provider state, the design **removes it from the registry** rather than shipping a priced-but-unrunnable id; if Anthropic re-enables it, it can be re-added.
- **Official `anthropic-sdk-go`.** GA, idiomatic (`NewStreaming` + `message.Accumulate`), typed `*anthropic.Error` carrying status/request-id/raw body, built-in auto-retry (on by default). A single concrete error type — branch on `StatusCode`.

### 2.2 Google — Gemini API

- **SDK landscape (CONFIRMED current).** The old `github.com/google/generative-ai-go` and `cloud.google.com/go/vertexai/genai` are **both deprecated** (Vertex one removed 2026-06-24). The single GA, maintained library is **`google.golang.org/genai`** (repo `github.com/googleapis/go-genai`), unified across Developer + Vertex backends, uses Go 1.23 range-over-func iterators.
- **Shape.** `[]*genai.Content{{Role, Parts}}`; **role is `"user"` or `"model"`** (not "assistant"). `Part` is a struct of optional pointer fields (`Text`, `FunctionCall`, `FunctionResponse`, …). **System prompt is `config.SystemInstruction`, not in `contents`.** Gen config on `GenerateContentConfig` (`MaxOutputTokens`, `Temperature`, `Tools`).
- **Function calling — CRITICAL CONFLICT.** Declarations pass `Parameters *genai.Schema`, an **OpenAPI-3.0 subset, NOT raw JSON Schema**. Supported: `type` (enum string `"OBJECT"` etc.), `nullable`, `required`, `format`, `description`, `properties`, `items`, `enum`, `anyOf`, `$ref`/`$defs` (written `Ref`/`Defs`). Unsupported (`$schema`, `additionalProperties`, `oneOf`/`allOf`/`not`/`const`, deep recursion) is dropped or 400s. **AgentKit must translate JSON Schema → `genai.Schema` for Google specifically.** Model returns a whole `FunctionCall{Name, Args}`; consumer replies `functionResponse` under role `user`.
- **Streaming.** `GenerateContentStream` returns **`iter.Seq2[*GenerateContentResponse, error]`**. Text deltas via `resp.Text()`. **FunctionCalls arrive whole in one chunk** (NOT streamed as partial JSON — asymmetry vs Anthropic/OpenAI). `UsageMetadata` on the final chunk.
- **Errors.** `genai.APIError`; wire shape `google.rpc.Status {code,message,status,details[]}` (`status` e.g. `RESOURCE_EXHAUSTED`). Retryable: 429/500/503/504. **SDK does NOT auto-retry — AgentKit must.**
- **Retry signals.** No `Retry-After` header; delay is in the body `details[]` as `RetryInfo.retryDelay` (e.g. `"31s"`). `QuotaFailure.quotaId` distinguishes per-minute (retry) vs per-day (fail fast).
- **Usage.** `UsageMetadata{PromptTokenCount, CandidatesTokenCount, TotalTokenCount, CachedContentTokenCount}`. Cached is a read-cache counted *within* prompt tokens.
- **Auth.** Developer API key (`BackendGeminiAPI`, single string) vs Vertex (project+location+ADC). For a neutral library taking explicit credentials, **the Developer API key path is by far simplest.** **Models (verified 2026-06-17 vs ai.google.dev models/pricing pages):** GA/stable text ids = `gemini-2.5-flash`, `gemini-2.5-pro` (tiered >200K), `gemini-3.5-flash` (current-gen default Flash, stable), and the stable cheap workhorse `gemini-3.1-flash-lite`. **The 3.x Pro reasoning model is PREVIEW-only: the served id is `gemini-3.1-pro-preview` (tiered >200K) — there is NO GA `gemini-3.1-pro` or `gemini-3-pro` text id.** ⚠ This contradicts the design registry (D16), which lists a bare `gemini-3.1-pro` as if GA: that id does not resolve and must become `gemini-3.1-pro-preview` (flagged preview) — or be replaced by GA `gemini-2.5-pro` if the curated set is GA-only. Flash naming is also resolved: `gemini-3.5-flash` (stable) and `gemini-3-flash-preview` (preview, prior-gen 3 Flash) are **two distinct models**, not two names for one.
- **Mandatory adapters regardless of wrap/raw choice:** (a) JSON-Schema→`genai.Schema` translator, (b) `assistant`↔`model` role normalization, (c) system prompt out of `contents`.

### 2.3 OpenAI — Responses vs Chat Completions

- **RECOMMENDATION: target the Responses API (`/v1/responses`) for OpenAI proper — but ALSO build a Chat-Completions adapter.** OpenAI explicitly recommends Responses for new projects; the official `openai-go` SDK calls it "the primary API"; newer reasoning models support tools well only there. Crucially, Responses uses **typed content Items and typed stream events**, which map cleanly onto Anthropic/Gemini — whereas Chat Completions' flat `delta` chunks do not. **However**, every OpenAI-*compatible* third party (Z.ai/GLM, and most others) speaks **Chat Completions, not Responses** — so AgentKit needs a Chat-Completions adapter regardless if it wants those providers (see §2.4). Treat them as two OpenAI-family adapters: Responses for OpenAI, Chat Completions (configurable base URL) for OpenAI-compatible endpoints. Chat Completions is not deprecated.
  - **Keep AgentKit stateless:** Responses is stateful by default (`previous_response_id`, server storage). **Ignore that** — resend full history each turn and set `store:false`, keeping the OpenAI adapter symmetric with the other two. Do NOT lean on `previous_response_id`.
- **Shape.** `input`: string or array of typed **Items** (`message`, `reasoning`, `function_call`, `function_call_output`). Message roles `developer` (replaces system) / `user` / `assistant`; system guidance can also go in top-level `instructions`. Token cap `max_output_tokens`. Structured output is `text.format` (NOT `response_format` — common error).
- **Tools.** Internally tagged: `{"type":"function","name","description","parameters":<JSON Schema>,"strict":true}`. Model emits a `function_call` Item with `call_id` + `arguments` (JSON string); consumer replies a **`function_call_output` Item keyed by `call_id`**. Parallel calls supported; loop until a response has no `function_call` Items. (Note: Chat Completions instead nests `{function:{…}}` and uses `role:"tool"` keyed by `tool_call_id` — schemas/keys NOT interchangeable between the two surfaces.)
- **Streaming.** Typed SSE events: `response.created` → `response.output_item.added` → `response.output_text.delta` / **`response.function_call_arguments.delta` (partial JSON fragments)** → `…done` → `response.completed` (carries final `usage` automatically — no `include_usage` opt-in needed).
- **Errors.** `{"error":{message,type,param,code}}`. Never retry 400/401/403/404; retry 408/409/429/500/502/503. `*openai.Error` carries status + raw body.
- **Retry signals.** `x-ratelimit-*` headers; `Retry-After` on 429/503 when present.
- **Usage.** `input_tokens`, `output_tokens`, `total_tokens`, `input_tokens_details.cached_tokens`. (Chat Completions uses `prompt_tokens`/`completion_tokens`/`prompt_tokens_details.cached_tokens` — a rename trap if both were ever supported.)
- **Models (verified 2026-06-17 vs developers.openai.com models/pricing/deprecations).** Curated set = `gpt-5.5-pro` (Responses-only, highest compute), `gpt-5.5` (flagship, ~1.05M ctx), `gpt-5.4` (more-affordable frontier), `gpt-5.4-mini`, `gpt-5.4-nano` (both 400K ctx) — **this matches the design registry (D16), which is the correct set.** Two corrections to *this research's own* earlier drift: (a) **`o4-mini` and `o3` are officially DEPRECATED/superseded** by the gpt-5.x reasoning models (older snapshots scheduled for API removal 2026-12-11) and must NOT be in a forward-looking curated set — drop them; (b) **`gpt-5.4-nano` DOES exist** (a §7 note had wrongly called nano nonexistent); `gpt-5.5-mini`/`gpt-5.5-nano` do not exist. Reasoning defaults differ by model — gpt-5.5 defaults to `medium`, gpt-5.4 defaults to `none` (don't assume a uniform default).
- **Official `openai-go` (v3).** Current, idiomatic; `Responses.New`/`NewStreaming`, built-in retries, `*openai.Error` with raw body.

### 2.4 Z.ai — GLM (Zhipu / BigModel)

The fourth provider, treated as an equal option. **It is OpenAI Chat-Completions-compatible**, so most of this is "same as OpenAI Chat Completions" — the valuable findings are the deltas.

- **Endpoint/auth.** First-party international platform, base URL **`https://api.z.ai/api/paas/v4/`** (chat at `…/chat/completions`); Bearer API key from the z.ai console. **Region gotcha:** separate international (`api.z.ai`) vs China (`open.bigmodel.cn` / bigmodel.cn) surfaces, each with its own account/key — use `api.z.ai` outside China. (A separate Anthropic-Messages-compatible *coding* endpoint exists at `…/api/coding/paas/v4` for Claude Code/Cline — not the path for an OpenAI-style adapter.)
- **Surface = Chat Completions only.** No Responses-API equivalent. Messages array; roles `system`/`user`/`assistant`/`tool`; assistant `tool_calls` with `id`; tool results keyed by `tool_call_id`. Request/response/streaming shapes are **OpenAI Chat-Completions-identical** — the stock OpenAI SDK works against the base URL. The only schema *addition* is GLM's `thinking` object.
- **Tools.** Standard OpenAI `tools` array (`{"type":"function","function":{name,description,parameters}}` with JSON Schema), assistant `tool_calls[]` with stringified `arguments`, `tool`-role results keyed by `tool_call_id`. Parallel tool calls are emitted. **Caveat: the stringified-`arguments` requirement is enforced unevenly across Z.ai base URLs** — the default `api/paas/v4` endpoint tolerates a replayed `arguments` sent as a JSON object, but the strict coding endpoint (`api/coding/paas/v4`, reachable via `zai.base_url`) rejects the object form with `400 Invalid API parameter (type=1210)`; the adapter must emit `arguments` as the JSON string the spec mandates so it works against either. **Caveat: `tool_choice` supports `"auto"` only** — no `"required"`/`"none"`/named forcing; surface a clear error if a caller requests forced tools. Heavy system prompts can suppress GLM's tool/reasoning decisions.
- **Streaming.** Standard OpenAI SSE `data:` chunks, `choices[].delta`, terminal `data: [DONE]`; tool-call argument fragments stream incrementally like OpenAI. Usage in-stream needs `stream_options:{include_usage:true}` (final chunk). **GLM adds `delta.reasoning_content`** (thinking-mode tokens) alongside `delta.content` — the delta parser **must tolerate unknown fields** and not choke on it.
- **Errors — Zhipu-shaped, NOT OpenAI-shaped.** `{"error":{"code":"1302","message":"..."}}` — `code` is a **string-numeric**, no `type`/`param`. Known: 401/`1001,1002,1003` auth (non-retryable); **429/`1302`** concurrency-too-high (**retryable**), **`1303`** request-rate (**retryable**); `1304/1308/1310` quota/limit (retry only after reset — treat non-transient); `1110–1113` balance/overdue/locked (non-retryable); **500/`1230,1234`** internal/network (**retryable**). The retry classifier must key off these **numeric codes**, not OpenAI `error.type`.
- **Retry signals.** No documented `Retry-After` or `x-ratelimit-*` headers — rely on status + body-code classification and own backoff (exponential + jitter; community reports ~1s retries clear 1302). Rate-limit HTTP status is 429.
- **Usage — OpenAI-named.** `usage.{prompt_tokens, completion_tokens, total_tokens}`, with prompt caching via **`usage.prompt_tokens_details.cached_tokens`** (OpenAI-compatible nesting; consistent with the published cached-input price). Maps to the uniform `Usage` exactly like OpenAI Chat Completions.
- **Models.** `glm-5.2` (flagship, ~744B MoE, 1M context, released 2026-06-13), `glm-5.1` (200K), `glm-4.7`/`-flash`, `glm-4.6`. Confirm exact live IDs against `https://docs.z.ai/llms.txt` at integration time.
- **GLM-specific gotchas.** Proprietary `thinking` toggle (`{"type":"enabled"|"disabled"}`, default enabled on 4.6/5.x); `reasoning_content` appears in both non-stream `message` and stream `delta`; `tool_choice=auto`-only; Zhipu string-coded error envelope. **Everything else matches OpenAI Chat Completions exactly.**
- **Implementation take.** Not a fourth bespoke adapter — **reuse the OpenAI Chat-Completions adapter with three deltas**: Zhipu error parsing, `thinking`/`reasoning_content` handling, and the `tool_choice=auto` constraint. This is the cheapest provider to add and is the reason the OpenAI-family path should be built on a **configurable base URL** from the start. No first-party Go SDK needed — point the OpenAI Chat-Completions client (or raw HTTP) at the base URL.

---

## 3. Prior art and its lessons

Surveyed: **langchaingo** (`tmc/langchaingo`), **gollm**, **inercia/go-llm**, **swarmgo**, **cloudwego/eino**, **pgEdge**, and the two most influential non-Go abstractions — **Vercel AI SDK** (TS) and **LiteLLM** (Python).

- **Clean shape = role + ordered list of typed, sealed content blocks** (text / tool-call / tool-result), dispatched by a type switch. Used by the strongest designs (Vercel `parts[]`, langchaingo sealed `ContentPart`, eino, go-llm, pgEdge). **Flat-string content is the recurring anti-pattern** (gollm/swarmgo end up wrapping text in XML and regex-parsing replies).
- **Two structural leaks to design around:** never bake one provider's response envelope (OpenAI `Choices[]`) into the neutral type — use a single `Message` + typed `FinishReason`; and keep provider-specific `map[string]any` extension bags to a minimum (langchaingo's `GenerationInfo`, eino `Extra` metastasize).
- **Streaming.** Three idioms: callbacks (weakest — hide tool-call assembly), channels, typed iterators (strongest). Prefer a **typed iterator/channel of events**. Assemble partial tool-call JSON **once, centrally, keyed by index/id**, and handle the **fragment (OpenAI/Anthropic) vs whole (Gemini)** asymmetry there.
- **Wrap SDKs vs raw HTTP.** The most serious neutral gateways (gollm, langchaingo, bifrost, LiteLLM) **hand-roll HTTP** to avoid three heavy, divergent SDK dependencies and to own errors/retries/usage end-to-end. The three official Go SDKs share no base type (OpenAI+Anthropic use `ssestream.Stream[T]`; Google uses `iter.Seq2`). See §11 for AgentKit's decision — the agents split, and it is the one genuinely open call.
- **Mid-conversation switching** works only if history is a provider-agnostic caller-owned slice of typed blocks. The concrete blocker is **tool-call IDs** (see §5).
- **Error/usage** is where every abstraction leaks hardest: differing field names *and* semantics, and finish-reasons differing in both name and enum (and a control signal for the agent loop). Use typed `Usage` + typed sentinel errors.
- **Borrow from Vercel:** a `warnings[]` pattern — when a provider can't honor a setting, **degrade with an explicit warning** rather than silently. Aligns with explicit-over-implicit.
- **Anti-patterns to avoid:** flat-string content; `map[string]any` as the primary extension mechanism; baking a provider envelope into the neutral type; callback-only streaming; sending raw provider tool-call IDs across a switch; lowest-common-denominator masking that hides genuinely divergent semantics (LiteLLM's chief criticism).

---

## 4. Core Go abstraction (design-informing)

### 4.1 Unified message / content-block model
Sealed interface + concrete block structs (idiomatic Go tagged union), canonical = Anthropic superset:

```go
type Role string // RoleUser, RoleAssistant (canonical)

type Message struct { Role Role; Blocks []Block }

type Block interface{ isBlock() }
type TextBlock       struct{ Text string }
type ToolUseBlock    struct{ ID, Name string; Input json.RawMessage } // structured, not string
type ToolResultBlock struct{ ToolUseID string; Content string; IsError bool }
```

Adapters reconcile: role `assistant`→`model` for Gemini (which also puts `functionResponse` under role `user`); **system prompt is a first-class field on the state object, not a message** (matches Anthropic top-level `system` + Gemini `systemInstruction`; OpenAI gets it as an injected `developer`/`instructions`); tool-call IDs always present (§5).

### 4.2 Streaming surface
**Recommendation: a `*Stream` struct exposing `Events() iter.Seq[Event]` plus terminal `Err() error` and `Usage() Usage` accessors** — the `sql.Rows`/`bufio.Scanner` pattern on Go 1.23+ range-over-func.

```go
for ev := range stream.Events() { /* TextDelta, ToolCallDelta, … */ }
if err := stream.Err(); err != nil { ... }
usage := stream.Usage()
```

Iterators beat channels (which leak goroutines on early `break` and force `select` plumbing) and callbacks (lose composability/early-exit). Early `break` makes `yield` return false → iterator returns and runs `defer` cleanup (close HTTP body) with no leak. Prefer the **terminal `Err()` accessor** over `iter.Seq2[Event,error]` (one stream error invalidates the whole sequence; `Seq2` is awkward and also can't carry setup/teardown errors). Pass `context.Context` as a normal arg, checked inside the loop. Go 1.26 changes no iterator semantics — stable.

### 4.3 Tool definition & JSON Schema
Canonical internal representation = **JSON Schema as `json.RawMessage`**, cached, converted per-provider at the boundary. ⚠ **The no-library constraint excludes `github.com/invopop/jsonschema`** (the de-facto struct→schema generator) — so AgentKit must produce the schema *without* it. Two standard-library-only options for the design author: **(a)** require the consumer to **supply the JSON Schema directly** (`json.RawMessage` / `map[string]any`) when registering a tool — simplest, zero reflection, but drops the typed-struct convenience; **(b)** hand-roll a **minimal `reflect`-based generator** covering the common Go-struct → JSON-Schema cases (structs, scalars, slices, maps, `json`/`jsonschema`-style tags) — more code, keeps the ergonomic `NewTool[In]` edge. Recommendation: **(a) as the guaranteed-correct core surface, with (b) as an optional convenience layered on top** if the typed edge is wanted; either way the registry stores raw JSON Schema and the per-provider boundary conversion is unchanged. Generics only at the registration edge, erased into a non-generic registry interface:

```go
type Tool interface {
    Name() string
    JSONSchema() json.RawMessage
    Call(ctx context.Context, args json.RawMessage) (any, error)
}
func NewTool[In any](name, desc string, fn func(context.Context, In) (any, error)) Tool
```

Anthropic/OpenAI pass the schema through nearly verbatim; **Gemini needs the lossy `jsonSchema → *genai.Schema` converter isolated in one place** (no `$ref`/`oneOf`/`additionalProperties`; nullability via a `Nullable` field; `Enum []string` only). Keep hand-written `map[string]any` schemas available as an escape hatch.

### 4.4 State/config object
A single mutable struct bundling config + history, threaded explicitly into each call; primary verbs as **methods** on it (they mutate `History`, read all config):

```go
type State struct {
    Provider Provider     // swappable mid-conversation
    Model    string
    Creds    Credentials
    Gen      GenSettings  // temperature, max tokens, …
    System   string       // system prompt — first-class field, not a message
    History  []Message
    Tools    []Tool
}
```

**Mid-conversation provider switching is just field mutation between calls** (`s.Provider = …; s.Model = …`); history is plain `[]Message` carried over untouched — the whole reason the message model must be a neutral superset. **Document explicitly: a `*State` is one conversation owned by one goroutine — not safe for concurrent use** (standard Go stance, cf. `sql.Rows`); no hidden locking.

### 4.5 Provider abstraction interface
One narrow internal interface — translation between AgentKit's canonical types and one wire format, nothing more:

```go
type Provider interface {
    Stream(ctx context.Context, req Request) *Stream
}
type Request struct {
    Model string; System string; Messages []Message
    Tools []Tool; Gen GenSettings; Creds Credentials
}
```

The auto-tool-loop, history accumulation, and full transparency (surfacing every message/tool-call/tool-result to the consumer) live in the `State` orchestration layer **above** this interface, not inside providers.

---

## 5. Tool-call identity — the load-bearing cross-provider problem

This is the single key to safe mid-conversation switching, and the agents surfaced a **factual conflict worth resolving in design:**

- Prior-art and the Google-API research found Gemini historically returns **empty `tool_call_id`** and **matches tool results by function name, not id**; meanwhile Anthropic enforces a strict id charset `^[a-zA-Z0-9_-]+$` and OpenAI-style ids like `functions.exec:2` **corrupt an Anthropic session**.
- The core-design research found **Gemini-3 now also emits a per-call `id`** to echo back — i.e. the name-only-matching premise may be outdated on the newest models.

**Recommended design (works under either reading — verify against current Gemini at build time):** AgentKit **mints its own neutral tool-call IDs at write time in Anthropic's strict charset**, and **stores the function name alongside** every tool-call/tool-result. At send time each adapter uses whichever the provider needs — id for Anthropic/OpenAI, and name (or echoed id) for Gemini. Also normalize OpenAI's wire-key difference (`tool_call_id` in Chat Completions vs `call_id` in Responses). This makes history fully portable across a mid-conversation provider switch regardless of how Gemini behaves.

---

## 6. Cross-cutting: errors, retry, usage

### 6.1 Uniform error taxonomy
Sentinel categories for `errors.Is`: `ErrAuthentication`, `ErrPermission`, `ErrInvalidRequest`, `ErrNotFound`, `ErrRateLimited`, `ErrOverloaded`, `ErrServerError`, `ErrTimeout`, `ErrNetwork`, `ErrContextLength`, `ErrContentFilter`, `ErrBilling`, `ErrUnknown`. **Detect by HTTP status first, refine by provider error-type string**, and for context-length/content-filter by message or finish-reason/blockReason.

| Category | Anthropic | OpenAI | Google | Z.ai (status / `code`) |
|---|---|---|---|---|
| Authentication | 401 `authentication_error` | 401 `invalid_api_key` | 401/403 `UNAUTHENTICATED` | 401 `1001/1002/1003` |
| Permission | 403 `permission_error` | 403 | 403 `PERMISSION_DENIED` | 403 |
| InvalidRequest | 400 `invalid_request_error`, 413 | 400 `invalid_request_error` | 400 `INVALID_ARGUMENT` | 400 |
| NotFound | 404 | 404 | 404 `NOT_FOUND` | 404 |
| RateLimited | 429 `rate_limit_error` | 429 `rate_limit_exceeded` | 429 `RESOURCE_EXHAUSTED` | 429 `1302/1303` |
| Overloaded | **529** `overloaded_error` | 503 | 503 `UNAVAILABLE` | (n/a — uses 429/500) |
| ServerError | 500 `api_error` | 500 `server_error` | 500 `INTERNAL` | 500 `1230/1234` |
| Timeout | **504** `timeout_error` | client timeout | 504 `DEADLINE_EXCEEDED` | client timeout |
| ContextLength | 400 (message-matched) | 400 `context_length_exceeded` | 400 (token-limit msg) | 400 (message-matched) |
| ContentFilter | `stop_reason` (not HTTP) | 400 `content_filter` | `blockReason`/`finishReason=SAFETY` | content-flag in response |
| Billing | 402 `billing_error` | 429 `insufficient_quota` | 400 `FAILED_PRECONDITION` | 429 `1110–1113` (balance), `1304/1308/1310` (quota) |

Typed `Error` struct carrying `Category` (sentinel), `Provider`, `StatusCode`, `Message`, `Type`, `RequestID`, `RetryAfter time.Duration`, **`Raw json.RawMessage` (verbatim provider body)**, and wrapped transport `Err`. Implement `Is` (→ Category) and `Unwrap`. **Branch on Category, never string-match messages.** Carry raw bytes untouched — never lossily re-marshal. **Z.ai is the exception that proves the rule:** its envelope is `{"error":{"code","message"}}` with a **string-numeric `code`** (no `type`), so its adapter classifies on HTTP status + numeric code, not OpenAI `error.type` — a separate mapping even though the rest of the surface is OpenAI-identical.

### 6.2 Retry & backoff
Retryable: `ErrRateLimited`, `ErrOverloaded`, `ErrServerError`, `ErrTimeout`, `ErrNetwork`. Never retry 400/401/403/404/413/422, content-filter, context-length, billing. **Honor server signals first** — Anthropic/OpenAI `Retry-After`; Gemini's body `RetryInfo.retryDelay` (no header). Otherwise exponential backoff with **full jitter**. **Streaming idempotency rule (critical): only retry before the first SSE byte is delivered** — once tokens stream to the consumer the turn is non-idempotent and must surface as an error (Anthropic explicitly notes post-200 mid-stream errors). Configurable: max attempts (default ~3–5), base/cap delay, max elapsed, honor-Retry-After toggle. Fixed: jitter algorithm, non-retryable list, the no-retry-after-first-byte rule. Hand-rolled (~60 lines) is recommended for control; `cenkalti/backoff/v5` if a dep is wanted. Always thread `context.Context`.

### 6.3 Usage & cost accounting — the hardest uniformity problem

This is the part the product calls out and the hardest to unify, because the four providers **disagree about what is included in what**. AgentKit now reports both tokens **and dollar cost** (cost is in scope per the product change): the uniform struct exposes enough **disjoint** token categories that cost is computed as `Σ bucket × rate[bucket]`, where `rate[bucket]` comes from AgentKit's baked-in per-model pricing table (the gathered rate data lives in §6.5). The disjoint-bucket design below is what makes that sum exact and provider-uniform.

**Three irreducible mismatches** (each confirmed against live API responses / official docs):
1. **Cached-input inclusion.** Anthropic's `input_tokens` **excludes** cached tokens (cache buckets are additive); OpenAI, Gemini, and Z.ai all report a prompt count that **includes** cached tokens (cached ⊂ input).
2. **Reasoning-output inclusion.** Anthropic, OpenAI, and Z.ai **roll reasoning/thinking tokens into the output count**; Gemini reports `thoughtsTokenCount` **separately**, outside `candidatesTokenCount`. And Anthropic & Z.ai **don't break reasoning out at all** (no separate field) — OpenAI and Gemini do.
3. **Cache-write.** Only **Anthropic** bills (and reports) a cache-*write* bucket, and only it tiers writes 5m vs 1h. OpenAI/Gemini/Z.ai caching is automatic/storage-priced — read discount only, no write token count.

**Inclusion/exclusion table (the crux):**

| Provider | "input" incl. cached? | "output" incl. reasoning? | reasoning broken out? | cache-WRITE bucket? | native `total`? |
|---|---|---|---|---|---|
| **Anthropic** | ❌ no (uncached only) | ✅ yes (rolled in) | ❌ no | ✅ yes (+5m/1h split) | ❌ derive |
| **OpenAI** | ✅ yes | ✅ yes | ✅ `output_tokens_details.reasoning_tokens` | ❌ no | ✅ `total_tokens` |
| **Gemini** | ✅ yes | ❌ **no** (thoughts separate) | ✅ `thoughtsTokenCount` | ❌ no | ✅ `totalTokenCount` |
| **Z.ai/GLM** | ✅ yes | ✅ yes (rolled in) | ❌ no | ❌ no | ✅ `total_tokens` |

**Recommended uniform struct — disjoint buckets that sum to `Total`** (carve reasoning out of output so it can be rated independently; every field a provider can't report stays 0):

```go
// Every field is a DISJOINT bucket; they sum to Total.
type Usage struct {
    InputUncached   int64 // fresh input, never cached
    CacheReadInput  int64 // input served from cache (discounted)
    CacheWriteInput int64 // input written to cache (Anthropic only; else 0)
    CacheWrite5m    int64 // subset of CacheWriteInput, 5m tier (Anthropic only)
    CacheWrite1h    int64 // subset of CacheWriteInput, 1h tier (Anthropic only)
    Output          int64 // visible output, EXCLUDING reasoning where separable
    ReasoningOutput int64 // thinking/reasoning tokens (0 where not separable)
    Total           int64 // sum of the disjoint input/output/reasoning buckets
}
```

**Per-provider mapping (⚠ = subtraction required to make buckets disjoint):**

| Field | Anthropic | OpenAI | Gemini | Z.ai |
|---|---|---|---|---|
| `InputUncached` | `input_tokens` | `input_tokens − cached` ⚠ | `promptTokenCount − cached` ⚠ | `prompt_tokens − cached` ⚠ |
| `CacheReadInput` | `cache_read_input_tokens` | `input_tokens_details.cached_tokens` | `cachedContentTokenCount` | `prompt_tokens_details.cached_tokens` |
| `CacheWriteInput` | `cache_creation_input_tokens` | 0 | 0 | 0 |
| `CacheWrite5m/1h` | `cache_creation.ephemeral_{5m,1h}_input_tokens` | 0 | 0 | 0 |
| `Output` | `output_tokens` (reasoning rolled in — **cannot split**) | `output_tokens − reasoning_tokens` ⚠ | `candidatesTokenCount` (already excl.) | `completion_tokens` (reasoning rolled in — **cannot split**) |
| `ReasoningOutput` | 0 (folded into Output) | `output_tokens_details.reasoning_tokens` | `thoughtsTokenCount` | 0 (folded into Output) |
| `Total` | derive (sum) | `total_tokens` (assert == sum) | `totalTokenCount` (assert == sum) | `total_tokens` (assert == sum) |

**Caveats to document:**
- **Anthropic & Z.ai cannot separate reasoning** — leave `ReasoningOutput=0`; reasoning stays inside `Output`. No cost loss (reasoning bills at the output rate everywhere) but the breakdown is unavailable for those two.
- **OpenAI & Gemini require subtraction** to disjoint the buckets (reasoning out of output; cached out of input — three of four providers need the cached subtraction).
- **Anthropic is the only derived `Total`** (no native total field); for the other three, assert their native total equals the bucket sum as a sanity check (and a regression canary on provider changes).
- **Pricing dimensions** (now computed by AgentKit from its baked-in table — see §6.5): distinct billed rates are uncached-input, cached-read input (discounted), cache-write input (Anthropic only; 5m=1.25×, 1h=2× base), output. Reasoning bills at the **output rate** on all four — but the bucket is kept separate anyway (Gemini's total math depends on tracking it; cost just rates `Output + ReasoningOutput` together). The disjoint-bucket struct above covers every billable category, so the flat per-bucket rate table in §6.5 prices it directly.

### 6.4 Testing strategy
`net/http/httptest` + recorded fixtures + golden SSE files, table-driven. Inject a configurable base URL / `*http.Client` so tests hit a fake server returning fixtures (exercises real JSON/SSE decode + error mapping, no credits). Table-driven error-mapping tests over the §6.1 matrix. Streaming via recorded raw `.sse` byte streams under `testdata/`, asserting assembled turn + `Usage` against golden JSON (`-update` flag). Retry tests with a fake server returning 429/503 N times then 200 and an injected clock — assert attempt count, honored delay, and **that mid-stream failures are not retried**. Live integration tests gated behind `//go:build integration` **and** an env-presence skip; capture fixtures once in a recording mode, scrub keys, commit.

### 6.5 Baked-in pricing data — per-model rate tables

The product change makes cost in-scope, so the design's `Pricing` table (one entry per supported model) must be **populated with real rates**. This subsection holds the gathered data so the design author isn't re-researching it. **Closed set = every model the design exports a constant for; each must have an entry (no model ships unpriced).** Rates are **nano-USD per token** (1e-9 USD; published `$/1M tok × 1000`). Buckets match the design's `Pricing` struct: `InputUncached`, `CacheReadInput`, `CacheWrite5m`, `CacheWrite1h`, `Output`. Reasoning tokens bill at the `Output` rate on all four providers. Gathered **2026-06-17** from each provider's official pricing page — re-verify before a release, as these are live commercial rates.

**Anthropic** — `CacheWrite5m/1h` are real Anthropic buckets. ⚠ Base input/output are published & high-confidence; the **cache rates are derived from Anthropic's conventional multipliers** (read 0.1×, 5m write 1.25×, 1h write 2× base input), *not* read off explicit per-model columns — verify against the live pricing page if exact cache billing matters.

| Model | InputUncached | CacheReadInput | CacheWrite5m | CacheWrite1h | Output |
|---|---|---|---|---|---|
| claude-opus-4-8 | 5000 | 500 | 6250 | 10000 | 25000 |
| claude-sonnet-4-6 | 3000 | 300 | 3750 | 6000 | 15000 |
| claude-haiku-4-5 | 1000 | 100 | 1250 | 2000 | 5000 |

**Google Gemini** (verified 2026-06-17) — no cache-write token bucket (caching is read-discount + separate per-hour storage fee AgentKit does not model); `CacheWrite5m/1h = 0`. ⚠ The 3.x Pro id is the **preview** `gemini-3.1-pro-preview`, NOT the design's bare `gemini-3.1-pro` (no such GA id). `gemini-3.1-flash-lite` added as the stable cheap option.

| Model | InputUncached | CacheReadInput | CacheWrite5m | CacheWrite1h | Output |
|---|---|---|---|---|---|
| gemini-2.5-flash | 300 | 30 | 0 | 0 | 2500 |
| gemini-2.5-pro *(≤200K)* | 1250 | 125 | 0 | 0 | 10000 |
| gemini-3.5-flash | 1500 | 150 | 0 | 0 | 9000 |
| gemini-3.1-flash-lite *(stable, cheap)* | 250 | 25 | 0 | 0 | 1500 |
| gemini-3.1-pro-preview *(≤200K; PREVIEW)* | 2000 | 200 | 0 | 0 | 12000 |

**OpenAI** (verified 2026-06-17) — no cache-write bucket (cached-input read discount only); `CacheWrite5m/1h = 0`. **`o3`/`o4-mini` removed — officially deprecated/superseded (do not ship).** **`gpt-5.5-pro` has NO cached-input discount — its `CacheReadInput` equals `InputUncached` (full 30000 on cached reads).**

| Model | InputUncached | CacheReadInput | CacheWrite5m | CacheWrite1h | Output |
|---|---|---|---|---|---|
| gpt-5.5-pro *(flat — see ⚠ below)* | 30000 | 30000 | 0 | 0 | 180000 |
| gpt-5.5 *(≤272K)* | 5000 | 500 | 0 | 0 | 30000 |
| gpt-5.4 *(≤272K)* | 2500 | 250 | 0 | 0 | 15000 |
| gpt-5.4-mini | 750 | 75 | 0 | 0 | 4500 |
| gpt-5.4-nano | 200 | 20 | 0 | 0 | 1250 |

**Z.ai / GLM** — international `api.z.ai` USD rates; no cache-write bucket (cached-input storage currently free); `CacheWrite5m/1h = 0`.

| Model | InputUncached | CacheReadInput | CacheWrite5m | CacheWrite1h | Output |
|---|---|---|---|---|---|
| glm-5.2 | 1400 | 260 | 0 | 0 | 4400 |
| glm-5.1 | 1400 | 260 | 0 | 0 | 4400 |
| glm-4.7 | 600 | 110 | 0 | 0 | 2200 |
| glm-4.6 | 600 | 110 | 0 | 0 | 2200 |

**Coverage:** every model in the closed set has findable, official pricing — **no gaps**. So "supported ⇒ priced" is achievable for the whole v1 set; no model is forced to ship unpriced.

**⚠ One real constraint conflict — context-length tiered pricing vs the flat `Pricing` struct.** The design's `Pricing` struct is **flat**: one rate per bucket, no notion of prompt length. But three models charge a higher rate above a context threshold, which a flat table cannot represent:

| Model | Threshold | Above-threshold rates (Input / CacheRead / Output, nano-USD/tok) |
|---|---|---|
| gemini-2.5-pro | > 200K input tokens | 2500 / 250 / 15000 (input 2×, output 1.5×) |
| gemini-3.1-pro-preview | > 200K input tokens | 4000 / 400 / 18000 (input 2×, output 1.5×) |
| gpt-5.5 | > 272K input tokens (whole session) | 10000 / 1000 / 45000 (input 2×, output 1.5×) |
| gpt-5.4 | > 272K input tokens (whole session) | 5000 / 500 / 22500 (input 2×, output 1.5×) |

⚠ **`gpt-5.5-pro` is NOT tiered in verified pricing** — the official model page gives a single flat rate (30000 in / 180000 out, no cached discount), with no >272K band. The design registry (D16) currently carries a `gpt-5.5-pro` 272001-tier (60000 / 60000 / 270000) that **could not be confirmed and is likely spurious** — recommend the design drop the pro high-tier (single flat tier) unless re-verified against the live page.

**Design-registry reconciliation (apply in the next design-mode pass).** With the model list re-verified, four deltas between D16 and ground truth:
1. **Google id bug** — D16's `gemini-3.1-pro` does not resolve; the served id is `gemini-3.1-pro-preview` and it is **preview, not GA**. Either rename + flag preview, or substitute GA `gemini-2.5-pro` if the curated set must be GA-only. (Pricing 2000/200/12000 → 4000/400/18000 above 200K is correct for the preview id.)
2. **OpenAI pro tier** — drop the unverified `gpt-5.5-pro` >272K tier; it is flat. Keep `CacheReadInput == InputUncached` (no cached discount) for it.
3. **Anthropic Fable 5 dropped** — `claude-fable-5` is globally disabled for all customers since 2026-06-12 (export control), so it cannot be served; the design **removes it from the curated set/registry** rather than shipping a priced-but-unrunnable id (re-add if Anthropic re-enables it).
4. **OpenAI `o3`/`o4-mini`** — already correctly absent from D16 (this *research* was the stale one); no design change, just confirming D16's set is right.

The tables above bake in the **low-tier (common-case)** rates for the tiered models. With a flat struct, cost is **exact below the threshold and undercounts above it**. Options for the design author: (a) accept the undercount and document it (simplest, and the threshold is rarely hit at 200–272K); (b) extend `Pricing` to carry an optional high-context tier + threshold (most correct, more surface); (c) define the supported-model constants to the low tier only. Recommendation: **(a)** for v1 — document the >threshold undercount — since it keeps the struct flat and the error only appears on very large prompts, but flag it so the choice is deliberate rather than accidental.

---

## 7. Reasoning models — preserved cross-turn state (the second load-bearing problem)

The v1 targets are all newest-generation reasoning models, and "use the newest reasoning APIs unless a model doesn't support it." This is not a cosmetic feature — it reshapes the message model. **The headline finding: three of four providers REQUIRE the model's prior reasoning output to be echoed back, verbatim, in the next request during a tool-use loop, or the turn errors or silently degrades.** AgentKit's auto-tool-loop is exactly such a loop, so this is mandatory, not optional.

**Enabling/controlling reasoning** (uniform neutral knob → provider param):

| Provider | Parameter | Effort/budget | Default | Can disable? |
|---|---|---|---|---|
| Anthropic | `thinking:{type}` + `output_config:{effort}` (`low`/`high`/`xhigh`/`max`) | effort enum, **default `high`**; `budget_tokens` **removed** on Opus 4.8 | **always-on (adaptive) on Opus 4.8** | **no** on Opus 4.8; **yes** on Sonnet 4.6 / Haiku 4.5 (extended-thinking toggle) |
| OpenAI | `reasoning:{effort, summary}` (Responses) | `none…xhigh` | gpt-5.5 `medium`; gpt-5.4 `none` | yes (`none`) |
| Google | `thinkingConfig:{thinkingBudget\|thinkingLevel, includeThoughts}` | 2.5 budget int; 3.x `thinkingLevel` | dynamic | Flash yes (`0`); **2.5 Pro / 3.x Pro no** |
| Z.ai | `thinking:{type}` (+ `reasoning_effort` on 5.2) | enabled/disabled; `high`/`max` | **on** | yes (`disabled`) |

→ **Map a neutral `ReasoningEffort` enum (`Off/Minimal/Low/Medium/High/Max`) to each provider; enforce per-model validity** (`Off` is invalid on **Opus 4.8**, Gemini 2.5 Pro, and 3.x Pro — all always-on reasoning; verified Opus 4.8 cannot disable). Avoid exposing a raw token budget — only Gemini 2.5 still uses one; translate ordinals to a budget int there.

**How reasoning content is delivered** — all as a *distinct* channel, never inline with the answer: Anthropic `thinking` blocks + opaque **`signature`** (raw CoT never returned; summary or omitted); OpenAI `reasoning` Items + **`encrypted_content`** blob (summaries only); Google `thought:true` parts + **`thoughtSignature`** (summaries); Z.ai plain-text **`reasoning_content`** (full text, no signature). **Streaming**: Anthropic `thinking_delta`/`signature_delta`; OpenAI `response.reasoning_summary_text.delta`; Google incremental thought parts; Z.ai `delta.reasoning_content`.

**THE critical constraint — cross-turn preservation in tool loops:**

| Provider | Echo prior reasoning on tool-result turn? | Form | If omitted |
|---|---|---|---|
| **Anthropic** | **Required** (interleaved thinking + tools) | `thinking` blocks **with `signature`**, unchanged, same model | 400 (modified/missing/reordered) |
| **OpenAI** (`store:false`/ZDR) | **Required** | pass back `reasoning` Items with `encrypted_content`; set `include:["reasoning.encrypted_content"]` every request | "reasoning item not found" / lost chain |
| **Google** | **3.x: required**; 2.5: optional | `thoughtSignature` echoed verbatim on the **specific** `functionCall` part, same position | Gemini 3.x **400** "missing thought_signature" |
| **Z.ai** | conditional (`clear_thinking:false`) | plain `reasoning_content`, byte-exact order | default `clear_thinking:true` is drop-safe; preserve mode degrades |

Google's per-part positional binding is the sharpest: the signature rides on a *specific* `functionCall` part (the first, on parallel calls) and must not be merged or reordered.

**Interface implications — concrete recommendations:**
1. **Add a first-class `ReasoningBlock` to the canonical message model** (§4.1), carrying: provider-opaque bytes (`signature`/`encrypted_content`/`thoughtSignature`/raw `reasoning_content`), an optional human-readable summary, and **association metadata** (which tool-call it binds to — required for Gemini). Treat the opaque payload as **preserve-and-replay-verbatim** — never synthesize, mutate, or reorder it. The block must survive the auto-loop and be re-emitted on the tool-result turn for the same provider/model. ⚠ **This block is provider-and-model-bound** — its opaque payload cannot cross a mid-conversation provider switch (unlike text/tool blocks). Design choice for the author: drop reasoning blocks on switch (safe — they're only needed by the model that produced them) and document it.
2. **Uniform `ReasoningEffort` config knob** on the request/state, mapped per §-table above, validated per model.
3. **Surface reasoning summary text** as a distinct streaming event/part (honoring the full-transparency promise), separate from the opaque replay payload. Default providers to emit summaries (Anthropic `display:"summarized"`, OpenAI `summary:"auto"`, Google `includeThoughts:true`). Raw CoT is unavailable on all but Z.ai, so "transparency" = summaries for three of four.
4. **OpenAI:** default `store:false` + auto-inject `include:["reasoning.encrypted_content"]` so the stateless multi-turn tool loop has its reasoning chain.

⚠ **Uncertainty flags (re-verified 2026-06-17 — most now resolved):** `gpt-5.4-nano` **does exist** (prior "nonexistent" note retracted), as do `gpt-5.4-mini`, `gpt-5.5`, `gpt-5.5-pro`, `gpt-5.4`; `gpt-5.5-mini`/`gpt-5.5-nano` do **not** exist; `o3`/`o4-mini` exist but are **deprecated** (drop). Gemini flash naming is resolved: `gemini-3.5-flash` (stable) ≠ `gemini-3-flash-preview` (preview) — two models; the 3.x **Pro** is preview-only (`gemini-3.1-pro-preview`; no GA `gemini-3.1-pro`). Gemini 3.x uses **`thinking_level`** (`minimal`/`low`/`medium`/`high`; `thinking_budget` deprecated-but-accepted), 2.5 uses `thinkingBudget`. **Anthropic Opus 4.8 reasoning is always-on/adaptive and CANNOT be fully disabled** (effort `low`/`high`/`xhigh`/`max`, default `high`; `budget_tokens` removed) — so `EffortOff` is invalid on Opus 4.8, not only on the always-on Gemini Pro models. Still genuinely open: Z.ai hard-fail-vs-degrade on dropped `reasoning_content` under preserve mode; `gpt-5.5-pro`'s exact effort levels/default and whether it accepts `none` (its model page confirms reasoning support but doesn't enumerate levels); and Z.ai's exact error-envelope shape (the official error-code page 404'd on 2026-06-17 — the Zhipu string-numeric `code` convention is assumed, verify against a live 4xx).

---

## 8. Caching models — the dominant multi-turn cost lever

Caching is the biggest cost/latency lever in a multi-turn + tool-loop conversation (a long prefix repeats every turn), and the providers differ on how much consumer control is required — which decides whether AgentKit must expose a caching API or can ride automatic caching.

| Provider | Automatic? | Min tokens | TTL (refresh?) | Cache-write cost | Cache-read | Explicit API |
|---|---|---|---|---|---|---|
| **Anthropic** | **No — opt-in** breakpoints | 4096 (Opus 4.8/Haiku 4.5) / 2048 (Sonnet 4.6) | 5m or 1h, **sliding** | **1.25× (5m) / 2× (1h)** | ~0.1× | `cache_control` breakpoints (max 4) |
| **OpenAI** | **Yes**, prefix-based | 1024 | 5–10m→1h; **24h** via `prompt_cache_retention` (default on gpt-5.5) | **none** | 0.1× (90% off) | none (knobs: `prompt_cache_key`, `prompt_cache_retention`) |
| **Google** | **Yes (implicit) + explicit** | 4096 (3.x) / 2048 (2.5) | implicit opportunistic; explicit 1h default, configurable | none (implicit) / **storage rent** (explicit) | discounted | `CachedContent` API (TTL, by name) |
| **Z.ai/GLM** | **Yes**, automatic | undocumented ⚠ | undocumented ⚠ | none documented | ~0.19× ($0.26/M; free storage promo) | none documented |

**Key asymmetry:** OpenAI, Gemini-implicit, and GLM cache automatically — they need **nothing** beyond a stable prefix. **Anthropic is opt-in: no `cache_control` ⇒ zero caching** — the worst outcome on the dominant cost lever. **Anthropic also uniquely charges to *write* a cache** (1.25×/2×), and **Gemini's explicit caches uniquely charge storage rent** ($/token/hour). **What busts a cache everywhere:** any byte change in the prefix from the start — so tool add/remove/reorder, a system-prompt edit, or a model switch invalidates downstream.

**AgentKit recommendation:**
- **v1 MUST (costs nothing, helps every provider):** (a) **preserve a stable, deterministic prefix** — freeze the system prompt (no `now()`/UUIDs interpolated), emit tools in deterministic order (sort by name, deterministic JSON serialization), never reorder/mutate tools or system mid-conversation, grow `messages` append-only; (b) **inject volatile context late** (trailing message, or a `role:"system"` message on Anthropic — not a prefix edit); (c) **report cached tokens** in the uniform `Usage` (already in §6.3).
- **v1 SHOULD set a default Anthropic breakpoint automatically** — one `cache_control` (5m) on the last block of the stable prefix (after tools+system+early history) whenever Anthropic is selected, guarded by the per-model minimum, so the uniform "just works" surface doesn't silently under-cache on Anthropic. Internal adapter behavior, not user-facing. For long agentic turns, also drop an intermediate breakpoint within the 20-block lookback.
- **Defer (opt-in knobs, not v1):** Anthropic 1h TTL + manual multi-breakpoint placement; Gemini explicit `CachedContent` (storage-rent tradeoff; only wins for very large fixed preambles); OpenAI `prompt_cache_retention:"24h"` / `prompt_cache_key` pass-through. A thin optional `CachePolicy` hint can later map to each mechanism — but v1's job is prefix stability + usage reporting + a sane default Anthropic breakpoint. **No general caching API in v1.**
- ⚠ GLM-5.2 min-cacheable size and TTL are undocumented; gpt-5.4 retention defaults inferred from the gpt-5.5 family — verify at integration.

---

## 9. MCP client — remote tool servers (the new capability)

The product now promises **remote MCP tool servers**. AgentKit is the MCP **client**; it connects to consumer-attached **remote** servers (network only — no subprocess/stdio), discovers their tools, and feeds them into the same auto-loop as custom tools, uniformly across all four providers. The design target is small and well-bounded: AgentKit needs **only the client side** and **only tools** (resources/prompts deferred). The findings below are external — MCP is a published open protocol with an official spec.

### 9.1 Protocol & transport
- **Spec revision.** MCP ships dated revisions; the current GA revision is **`2025-11-25`** (a `2026-06-30` revision is in development). The transport/auth/tools mechanics below are stable across `2025-06-18` → `2025-11-25`. **Pin a revision and send it explicitly** (see header note below). Everything is **JSON-RPC 2.0** over the transport.
- **Target transport = Streamable HTTP.** Two remote transports exist: the **legacy HTTP+SSE** (`2024-11-05`, two endpoints) — **deprecated, do not target** — and **Streamable HTTP** (since `2025-03-26`, current) — **the one to build against**. Streamable HTTP is a **single endpoint URL** that accepts POST (JSON-RPC request; the consumer supplies this URL per server) and optional GET (a standalone server→client SSE stream for notifications, which a tools-only client may skip). **Each request POST gets one of two response content-types — `application/json` (single response) or `text/event-stream` (an SSE stream that eventually carries the response for long-running calls) — and the client must handle BOTH.** A POST carrying only a notification/response returns `202 Accepted`, no body.
- **Client lifecycle.** `initialize` (client sends preferred `protocolVersion` + `capabilities` + `clientInfo`; server replies with its chosen version + capabilities) → `notifications/initialized` → then operations. **Discovery = `tools/list`** (paginated via `cursor`/`nextCursor` — loop until `nextCursor` absent). **Invocation = `tools/call`** with `{name, arguments}`.
- **Wire shapes the design needs.** A tool definition carries `name`, optional `title` (display-only), `description`, **`inputSchema` (JSON Schema)**, optional `outputSchema`, optional untrusted `annotations`. A `tools/call` **result** is `{content[], structuredContent?, isError?}` where `content[]` is an ordered array of typed blocks (`text`, `image`, `audio`, `resource_link`, embedded `resource`). For a **text-only** product, the `text` blocks are what matter (see §9.3 collapse rule).
- **Dynamic tool sets.** `notifications/tools/list_changed` exists (server must declare `capabilities.tools.listChanged`); on receipt the client re-runs `tools/list`. **v1 may defer honoring it** (re-list on demand / on attach) — and there's a caching reason to (§9.3).
- **Session & version headers.** Server MAY return an `Mcp-Session-Id` header on the `InitializeResult`; if so the client **MUST** echo it on every subsequent request. After init, the client **MUST** also send `MCP-Protocol-Version: <negotiated>` on every request — **omitting it makes servers assume `2025-03-26`**, so always set it explicitly. Clean detach = best-effort HTTP `DELETE` with the session header (ignore a `405`).

### 9.2 Client implementation — raw HTTP (decided: no library)
**Decided by the no-third-party-libraries constraint: AgentKit hand-rolls a minimal raw-HTTP Streamable-HTTP MCP client over the standard library.** This is tractable because AgentKit needs only a *sliver* of the protocol — **4 client calls** (`initialize`, `notifications/initialized`, `tools/list`, `tools/call`), tools only, no server/resources/prompts — and is *already* writing bespoke SSE parsing and JSON handling for all four LLM providers. The marginal new machinery is one Streamable-HTTP client: POST a JSON-RPC body; **accept either an `application/json` response or a `text/event-stream` stream** and read the JSON-RPC response out of whichever arrives; carry the `Mcp-Session-Id` and `MCP-Protocol-Version` headers; do the `initialize`→`initialized` handshake. On the order of a few hundred lines, not thousands.

*(Reference only — the existence of the mature official `github.com/modelcontextprotocol/go-sdk`, Anthropic+Google-maintained at stable v1.x with a clean `StreamableClientTransport`/`Connect`/`CallTool` API, is noted so the design author knows the protocol surface is well-trodden and can mirror its proven shapes — `HTTPClient` round-tripper for auth injection, iterator-based `tools/list` pagination. It is **not** a dependency option.)* The one part to get right is the **dual JSON-vs-SSE response path** on a request POST (a server may answer a `tools/call` with either) — AgentKit already owns provider SSE code, so this reuses that muscle rather than introducing new risk.

### 9.3 Integrating MCP tools into the canonical model
- **Reuse, don't special-case.** On attach, connect + `tools/list` once, wrap each MCP tool as an ordinary `Tool` (§4.3) that closes over its server connection, and concatenate into the same registry the auto-loop already drives. The model and providers see no difference. **Route a call back to its server by a stored `(serverHandle, originalMCPName)` binding — NOT by re-parsing a prefix out of the name** (sanitization below is lossy/irreversible). This is the dominant prior-art pattern (Vercel AI SDK, OpenAI Agents SDK, LangChain adapters, eino).
- **Prefixing + name sanitization (separator = `_`).** Provider tool-name charsets are strict: **Anthropic and OpenAI both require `^[a-zA-Z0-9_-]{1,64}$`** — so `.`, `/`, `:` are **illegal** (Gemini tolerates `.`, the others do not). Real MCP servers ship tool names with dots/slashes (`git.commit`, `multi_tool_use.parallel`), which Anthropic/OpenAI **reject**. Recommended scheme: final name = `<serverName>_<mcpToolName>`, then **sanitize the whole string to `^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`** (replace illegal chars with `_`, ensure a letter/`_` start, truncate to ≤64 with a hash suffix on overflow to keep uniqueness). Keep the sanitized→`(server, originalName)` map for routing.
- **Collision = uniform error (already promised).** Detect duplicates **after** prefixing+sanitization (two raw names can sanitize to the same string), against the full merged set *including native tools*, and surface AgentKit's uniform collision error. This matches the **best** prior art (OpenAI Agents SDK hard-errors; LiteLLM prefixes) and **avoids the common anti-pattern** (Vercel/LangChain/langchaingo/eino all silently last-wins shadow).
- **Schema-translation risk (Gemini) — the real one.** MCP `inputSchema` is arbitrary third-party JSON Schema (draft 2020-12; `$ref`/`$defs`/`oneOf`/`additionalProperties` all common) that AgentKit does not control. The §4.3 Gemini converter is **lossy** (`genai.Schema` has no `oneOf`/`$ref`/`$defs`/`additionalProperties`), so under Google a real MCP schema silently drops constraints or 400s (e.g. an untyped `array` with no `items`). No surveyed library handles this well. **Recommendation:** run the converter best-effort (inline `$ref`/`$defs`, map `oneOf`→`anyOf`, strip `$schema`/`additionalProperties`, synthesize `items` for untyped arrays), **detect lossiness and emit a non-fatal `warnings[]`-style notice** (per server+tool, naming dropped keywords) rather than degrading silently — doing better than prior art at exactly this point. Scope the conversion to the **Google boundary only**: don't fail registration when the active provider is Anthropic/OpenAI (which pass JSON Schema near-verbatim); the degradation + warning surfaces if/when the conversation switches to Gemini.
- **Result collapse (text-only).** Concatenate `content[]` in order into one string: `text`→its text; `image`/`audio`→a placeholder marker (e.g. `[image: <mimeType>, N bytes omitted]`) — **never dump base64 into the prompt** (LangChain's anti-pattern; token-expensive and useless to a text model); `resource_link`→its `uri` (+name/desc); embedded `resource`→its `text` if present else a `[resource: <uri>]` marker. **Prefer `structuredContent` when present** (serialize to compact JSON; the spec says servers SHOULD also mirror it into a text block, so either is safe). Do **not** JSON-dump the entire `CallToolResult` struct (eino's anti-pattern — noisy, token-heavy).
- **The two failure channels map exactly onto AgentKit's existing two.** `isError:true` in a *successful* JSON-RPC `result` = the tool ran but its business logic failed → **`ToolResultBlock{IsError:true}` fed back to the model** so the conversation continues (the product's "tool returns an error result" promise). A JSON-RPC `error` object, or any transport/HTTP failure = **AgentKit uniform error** (the "transport fails mid-call" promise). **The decision rule: presence of `result` vs `error` in the JSON-RPC envelope decides it — never inspect `isError` to decide whether to raise; only to set the block flag.** (Avoid eino's anti-pattern of turning `isError:true` into a loop-aborting Go error.)

### 9.4 Transport, auth & failure mapping
- **Auth = static token in a header; no interactive OAuth.** The MCP authorization spec is OAuth 2.1-based (PKCE, protected-resource metadata, `WWW-Authenticate`) — but it governs token *use*, not *acquisition*, at the transport, so the **static-token path is fully spec-compliant**: AgentKit sets `Authorization: Bearer <consumer-supplied token>` (and/or arbitrary consumer headers like `X-API-Key`) on every request and **never runs the OAuth dance**. A server that *requires* full OAuth manifests as **`401` with a `WWW-Authenticate` header** pointing at its metadata; AgentKit deliberately does **not** follow it — instead it surfaces a clean `ErrAuthentication` and should **stash the `WWW-Authenticate` value in `Error.Message`/`Raw`** so the consumer learns "this server wants OAuth, supply a token." `403` = token present but insufficient scope → `ErrPermission`.
- **No new error sentinel needed.** The existing §6.1 taxonomy absorbs MCP cleanly — a new `ErrMCP`/`ErrToolTransport` sentinel would *reduce* the uniformity that is the taxonomy's whole point. Mapping:

| MCP failure | Channel | AgentKit sentinel |
|---|---|---|
| Connection refused / DNS / TLS | HTTP | `ErrNetwork` |
| **Init/handshake fails on attach (mode A)** | HTTP | classify by cause — `ErrNetwork` / `ErrAuthentication` (401) / `ErrNotFound`·`ErrInvalidRequest` (wrong URL / non-MCP 4xx) / `ErrServerError` (5xx). *No dedicated "attach" category.* |
| `401` (+`WWW-Authenticate`) | HTTP | `ErrAuthentication` (stash `WWW-Authenticate`) |
| `403` insufficient scope | HTTP | `ErrPermission` |
| `404` session expired/terminated | HTTP | recover transparently (re-`initialize`) for idempotent ops; surface only if re-init fails |
| `400` missing session-id / bad protocol-version / malformed | HTTP | `ErrInvalidRequest` (client bug — no retry) |
| `429` | HTTP | `ErrRateLimited` (honor `Retry-After`) |
| `5xx` | HTTP | `ErrServerError` |
| **Transport drops mid `tools/call` (mode B)** | HTTP | `ErrNetwork` (or `ErrTimeout`) |
| JSON-RPC `-32601`/`-32602`/`-32600` | JSON-RPC | `ErrInvalidRequest` |
| JSON-RPC `-32603` / server `-32000..-32099` / `-32700` | JSON-RPC | `ErrServerError` |
| **`isError:true`** | result | **NOT an error** → `ToolResultBlock{IsError:true}` to model |

  MCP defines **no tool-specific JSON-RPC codes** beyond the standard set + the server-defined `-32000..-32099` range. `405` on the GET stream / on DELETE is **benign**, not an error.
- **Identifying which server failed.** The §6.1 `Error` carries `Provider`. For MCP, **either** add a dedicated `MCPServer` field (cleaner — keeps `Provider` strictly LLM-valued; recommended) **or** document a `Provider = "mcp:<serverName>"` convention. Populate `Raw` with the verbatim JSON-RPC `error` object (or HTTP error body) exactly as it carries LLM provider raw bodies today; map the JSON-RPC `code` into `Error.Type`.

### 9.5 Retry & lifecycle
- **Do NOT auto-retry `tools/call`.** MCP gives no trustworthy idempotency signal (`annotations.idempotentHint`/`readOnlyHint` are optional **and untrusted**), and a tool may have side effects. Treat a tool invocation like a **non-idempotent POST**: surface mode-B failures (`ErrNetwork`/`ErrTimeout`/`ErrServerError`/`429`) to the caller **without** automatic retry; the model can re-issue the call if appropriate. Mirror the streaming rule from §6.2: **once any byte of a tool-result SSE stream is delivered, never retry.**
- **DO retry discovery.** `initialize` and `tools/list` are idempotent/read-only, so retry them under the standard §6.2 policy (network/timeout/5xx/429 → full-jitter backoff) but **fail-fast** on `401/403/400` and non-MCP `4xx`. So: **attach retries transient transport failures; tool invocation does not.**
- **Session re-establishment.** On `404` (session expired) for a safe/idempotent op, transparently send a fresh `InitializeRequest` (no session id) and retry — spec-mandated client behavior. On a `404` *mid `tools/call`*, re-establish the session but **do not silently replay** the call (side-effect risk) — surface mode-B and let the model/consumer decide.
- **Timeouts & cancellation.** Implement a per-`tools/call` deadline (`ErrTimeout` on fire). To cancel cleanly, send an MCP `CancelledNotification` rather than just dropping the connection (a bare disconnect is not read as cancellation by the server).
- **Attach/detach lifecycle.** **Connect + `tools/list` eagerly on attach** (between turns) so collisions and schema-lossiness surface at attach time, not mid-turn — but **bound it with a connect/list timeout** so a dead server doesn't block attach, and **isolate per-server failures** so one bad server doesn't wipe the whole tool set. Keep the session warm across turns; close it (DELETE) on detach; close on teardown to avoid the connection leak prior art flags.
- **Caching consequence (AgentKit cares — §8).** Re-listing per request or honoring `tools/list_changed` mid-conversation **busts prompt caching** (the tool array is part of the stable prefix). Recommendation: maintain a **deterministic tool order** (native tools first, then servers in attachment order, then each server's tools in `tools/list` order — never map-iteration order), cache the `tools/list` snapshot per server, treat a tool-set change (attach/detach, or an honored `tools/list_changed`) as a deliberate cache-invalidation event (same cost class as a model switch), and consider making `tools/list_changed` handling **opt-in** since a churn-y server would repeatedly bust the cache.

---

## 10. Recommendations carried into design (summary)

1. **Canonical message model = Anthropic-shaped superset**: `[]Message` of `Role` + sealed `[]Block` (text/tool_use/tool_result **+ reasoning**); single `Message` + typed `FinishReason` response, never a `Choices[]` envelope.
2. **System prompt is a first-class `State` field, not a message.**
3. **Streaming = `*Stream` with `Events() iter.Seq[Event]` + terminal `Err()`/`Usage()` accessors.** Assemble partial tool-call JSON centrally, handling fragment-vs-whole (Gemini sends whole).
4. **Tool input schema = JSON Schema (`json.RawMessage`)**, supplied directly by the consumer (no `invopop/jsonschema` — excluded by the no-library rule; optional hand-rolled `reflect` generator for the typed `NewTool[In]` edge, §4.3), with an isolated lossy `→ *genai.Schema` converter for Google; `map[string]any` escape hatch.
5. **Mint neutral tool-call IDs (Anthropic charset) + carry function name** for portable mid-conversation switching. ⚠ Verify current Gemini id behavior at build time (§5 conflict).
6. **Single mutable `*State`** bundling config+history, methods as verbs, provider switch = field mutation, documented single-goroutine.
7. **One-method internal `Provider` interface**; auto-loop/history/transparency live in the orchestration layer above it.
8. **Typed `Error`** (sentinel `Category` + verbatim raw body) and **typed disjoint-bucket `Usage`** (§6.3: uncached/cache-read/cache-write/output/reasoning, summing to total); branch on category, never strings; subtract to disjoint the buckets per provider.
8a. **Baked-in cost (§6.5).** Ship a flat per-model rate table (nano-USD/token, populated in §6.5) keyed to the closed model set so every supported model is priced; cost = `Σ bucket × rate` over the disjoint `Usage`. One unresolved design call: the flat table can't represent the context-length tiers on gemini-2.5-pro / gemini-3.1-pro-preview / gpt-5.5 / gpt-5.4 (gpt-5.5-pro is flat) — recommend baking low-tier rates and documenting the >threshold undercount. (D16 already adopted a tiered `Pricing` struct, so this is largely resolved in design — see §6.5 reconciliation for the remaining id/tier corrections.)
9. **Retry**: honor server delay → else full-jitter backoff; never retry after first stream byte; honor the non-retryable category list.
10. **OpenAI-family = two adapters.** Responses API (stateless, `store:false`, resend history) for OpenAI proper; a **Chat-Completions adapter with configurable base URL** for OpenAI-compatible providers (**Z.ai/GLM**), reused with three deltas (Zhipu error parsing, `thinking`/`reasoning_content`, `tool_choice=auto`-only). Build the OpenAI-compatible path on a configurable base URL from day one so Z.ai (and any other compatible endpoint) is nearly free.
11. **Reasoning is first-class (§7).** Add a preserve-and-replay-verbatim `ReasoningBlock` (opaque signature/encrypted/thoughtSignature + optional summary + tool-call association) that round-trips across the auto-loop; a neutral `ReasoningEffort` knob mapped per model; surface reasoning summaries as a distinct stream event; OpenAI default `store:false` + `include:["reasoning.encrypted_content"]`. Reasoning blocks are provider/model-bound — drop them on a mid-conversation provider switch.
12. **Caching (§8): don't sabotage it, don't build an API for it.** Stable deterministic prefix (frozen system, sorted/deterministic tools, append-only messages), volatile context injected late, cached tokens reported. Set a default Anthropic `cache_control` breakpoint (opt-in provider) so the uniform surface doesn't under-cache. Defer explicit caches/TTL knobs.
13. **Decided — raw HTTP, no third-party libraries** (§11). The no-library constraint settles it: all four provider adapters and the MCP client are hand-rolled over the standard library; SSE parsing, partial-JSON tool accumulation, retry/backoff, and error/usage extraction are AgentKit's own. The official SDKs, MCP `go-sdk`, `invopop/jsonschema`, and `cenkalti/backoff` are excluded.
14. **MCP client (§9): remote-only, Streamable HTTP, tools-only, hand-rolled.** Build a minimal raw-HTTP Streamable-HTTP client (4 calls: `initialize` / `initialized` / `tools/list` / `tools/call`) — the one part to get right is the **dual `application/json`-vs-`text/event-stream` response path**, which reuses AgentKit's existing provider SSE code. Wrap each MCP tool as an ordinary `Tool`; **prefix `<serverName>_<tool>` and sanitize to the strict tool-name charset**, routing by a stored `(server, originalName)` map; **hard-error on collision** (no silent shadow). Reuse the §4.3 Gemini schema converter at the Google boundary, but **warn on lossiness** (MCP schemas are third-party). Map `isError:true`→tool-result-to-model vs JSON-RPC/transport-error→uniform error by **`result` vs `error` envelope**; the §6.1 taxonomy absorbs MCP with **no new sentinel** (add an `MCPServer`/source field for attribution). **Static bearer-token auth, no interactive OAuth**; surface OAuth-required `401`s as `ErrAuthentication`. **Do not auto-retry `tools/call`** (non-idempotent); retry only idempotent discovery. Keep tool order deterministic for caching.
15. **Testing**: httptest + golden SSE fixtures + gated live integration; capture fixtures once. Add MCP-client fixtures (Streamable-HTTP JSON-RPC: `initialize`/`tools/list`/`tools/call`, the JSON-vs-SSE response split, `isError` results, JSON-RPC error objects, `401`+`WWW-Authenticate`) against a fake MCP server.

---

## 11. Resolved — raw HTTP, standard library only (no third-party deps)

**This was the one place the research did not converge; the no-third-party-libraries directive (2026-06-17) closes it: raw HTTP, standard library only.** Recorded here is what that decision *commits AgentKit to build* and what it gives up, so the design author inherits the consequences rather than re-litigating the choice.

The split that existed: the **per-provider agents** (Anthropic/Google/OpenAI) each recommended **wrapping the official SDK** (all GA, idiomatic, providing streaming accumulation, typed errors with raw body, and Anthropic/OpenAI auto-retry); the **prior-art agent** recommended **raw HTTP** (serious neutral gateways hand-roll it to avoid heavy divergent deps and own errors/retries/usage). The directive selects raw HTTP unconditionally — the SDKs are not options.

**What raw HTTP commits AgentKit to hand-roll** (standard library: `net/http`, `encoding/json`, `bufio`, `iter`):
- **Per-provider SSE parsing** — Anthropic/OpenAI emit partial-JSON tool-call fragments to concatenate and parse at block close; Gemini sends whole function calls; Z.ai is OpenAI-Chat-Completions-shaped. One central SSE/event assembler (§4.2) handles the fragment-vs-whole asymmetry.
- **Partial-JSON tool-call accumulation**, keyed by index/id (§4.2, §5).
- **Error + usage extraction per provider** from raw bodies into the typed `Error`/`Usage` (§6.1, §6.3) — this is *required* work regardless of wrap/raw, so raw HTTP loses little here.
- **Retry/backoff** (§6.2) — full-jitter, honor-server-delay, no-retry-after-first-byte — hand-rolled (no `cenkalti/backoff`). Note Google's SDK auto-retries nothing anyway, so this was always partly hand-rolled.
- **Struct→JSON-Schema generation** (§4.3) — consumer supplies the schema directly, or an optional hand-rolled `reflect` generator (no `invopop/jsonschema`).
- **The MCP Streamable-HTTP client** (§9.2) — the 4-call client, dual JSON-vs-SSE response handling, session/version headers.

**What it gives up:** the SDKs' free streaming accumulation, typed-error-with-raw-body, session/handshake lifecycle (MCP), and built-in retry. The mitigating fact throughout: **every provider adapter is bespoke regardless** (the four wire formats don't unify at the SDK level — OpenAI/Anthropic use `ssestream.Stream[T]`, Google uses `iter.Seq2`, Z.ai has no Go SDK at all), so wrapping would have bought less than it appears, and AgentKit owning the whole wire path keeps errors/retries/usage uniform and dependency-free. **Z.ai was already raw-HTTP-only** (no first-party Go SDK; a single Chat-Completions adapter parameterized by base URL serves it and any other OpenAI-compatible endpoint with small per-provider delta hooks) — so the OpenAI-compatible family needed hand-rolling either way.

**Design consequence:** the adapter layer is uniformly raw HTTP across all four providers + the MCP client; there is no dependency footprint to manage and no SDK retry to disable. Build one shared SSE/JSON-RPC HTTP core and parameterize it per provider.
