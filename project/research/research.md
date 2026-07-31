# AgentKit — Research

**Status: non-contractual.** This document informs the *author* of `project/design/README.md`; nothing downstream (the autonomous build) reads it. It records external ground truth — provider API footprints, prior art, and constraints — so design never has to re-derive them. Design remains the single authority for *how*; where this doc describes a mechanism, design may adopt, refine, or reject it. It is the single **current** statement of that ground truth, rewritten in place: a changed fact replaces its predecessor and a dropped one is removed. There is no history here — construction history lives in git.

**Live commercial values re-verify before release.** Rates, context windows, and model ids below are read off vendor pages and move without notice; the release process owns re-checking them.

**Dependency policy: external libraries require explicit per-case human approval.** The default is the Go standard library (`net/http`, `encoding/json`, `iter`, …); a third-party module enters the build only when the operator has approved that specific module, and approval covers its transitive closure. **Approved:** `github.com/invopop/jsonschema` — struct→JSON-Schema derivation for `NewTool[In]` — together with the indirect modules it pulls in (`bahlo/generic-list-go`, `buger/jsonparser`, `pb33f/ordered-map/v2`, `go.yaml.in/yaml/v4`). **Not approved, and therefore not used:** the official provider SDKs (`anthropic-sdk-go`, `openai-go`, `google.golang.org/genai`), the MCP `go-sdk`, and `cenkalti/backoff`. Every provider adapter and the MCP client are consequently **raw HTTP**: SSE parsing, partial-JSON tool-call accumulation, retry/backoff, and error/usage extraction are all hand-rolled. The SDKs appear below only as reference for *what* behavior AgentKit re-implements.

**Reasoning is native-first.** Reasoning is expressed in **each model's own native term and native values** — the term the provider documents (effort / thinking level / thinking budget / thinking) and the values that model accepts (discrete levels, or a token-budget integer within a range) — with **no cross-model vocabulary and no translation**. A value the model accepts is honored exactly; one it does not is rejected by the vendor and surfaces as that provider's typed error. The advisory catalog exposes each cataloged model's term, accepted values, and default so a consumer can render and validate a choice before spending a round trip; nothing in the request path consults it.

**Remote MCP tool servers are supported.** AgentKit is an MCP **client**: the consumer attaches remote MCP servers (network transport only — AgentKit spawns no subprocesses, so local stdio servers are out of scope), AgentKit connects, discovers each server's tools, and feeds them into the *same* automatic tool loop as custom tools, uniformly across every provider. Only **tools** are surfaced (MCP resources/prompts are out of scope); the consumer names each server and that name **prefixes** its tools; credentials are supplied explicitly with no interactive OAuth. Servers attach and detach between turns, mirroring provider/model switching. See **§9** for protocol, transport, integration, auth, and failure mapping.

The product (`project/product/README.md`) fixes the target: a Go 1.26 library, module `github.com/ikigenba/agentkit`, giving **one uniform surface** for a tool-using, multi-turn, **text-only**, streaming chat plus a text-embeddings surface — provider+model is configuration, switchable mid-conversation. Model strings are **free-flow**: AgentKit maintains no allow-list, the vendor judges the id, and an advisory catalog carries metadata for the models we track. **Dollar-cost accounting is in scope** and honest at the edges: where the provider reports the true charge (OpenRouter) that figure wins, otherwise cost is computed from consumer-supplied rates (typically one catalog lookup), and a call with no rate source still runs — reporting zero cost with a `WarnCostUnknown` warning rather than blocking. Out of scope: images/audio, persistence, ambient credentials.

**Five chat providers, each a first-class peer: Anthropic, Google, OpenAI, Z.ai (Zhipu/BigModel, GLM family), and OpenRouter (aggregator).** A provider reached through API-compatibility or through an aggregator is no less first-class on the public surface; how it is implemented is not user-visible. Implementation splits two ways: Anthropic, Google, and OpenAI are bespoke adapters over native protocols, while Z.ai and OpenRouter share `internal/openaicompat`, an OpenAI-Chat-Completions core parameterized by base URL (see §2.4, §14). OpenAI is reachable two ways, chosen at construction: a platform API key, or a ChatGPT subscription token file (§15). **Embeddings are a narrower set — OpenAI and Google only** (§12).

---

## 1. The central finding

Structural unification across the providers is **genuinely achievable and clean for text chat**. Every serious prior-art abstraction confirms it. The irreducible leaks cluster in exactly four places — **streaming tool-call deltas, tool-call identity, reasoning/thinking state, and token/usage accounting**. AgentKit's *text-only* scope drops images and persistence — but it does **not** get to drop cost (computed from consumer-supplied rates against the usage buckets, or read off the provider where it reports one) and does **not** get to drop reasoning, because the target models are newest-generation **reasoning** models and three providers *require* reasoning state to be echoed back across tool-use turns (see §7). So **three** of the four leak zones are squarely in play and are where the design must concentrate: **tool-call identity (§5), reasoning-state preservation (§7), and token/usage + caching accounting (§6.3, §8)**. Get those three right and the rest of the uniform surface falls out naturally.

The recommended canonical model is **Anthropic-shaped**: a conversation is `[]Message`; each `Message` is a `Role` plus an ordered `[]Block`; blocks are `text` / `tool_use` / `tool_result`. Anthropic's content-block shape is the richest of the providers and the cleanest to down-convert from. OpenAI's Responses API, Google's `Part` struct, and Z.ai's OpenAI-compatible Chat Completions shape all map onto it; the provider adapter owns the translation.

**The five providers split into two implementation families.** Three are *native* protocols with bespoke adapters: Anthropic (Messages API), Google (Gemini), OpenAI (Responses API). The other two — **Z.ai/GLM and OpenRouter** — are OpenAI-Chat-Completions-compatible, so they are not bespoke adapters but consumers of a shared `internal/openaicompat` core parameterized by base URL + key + model, each with a few small deltas (for Z.ai: a Zhipu-shaped error envelope, GLM `thinking`/`reasoning_content` fields, `tool_choice=auto`-only; for OpenRouter: its own `reasoning` object encoding and per-response cost). Building the OpenAI-compatible path around a **configurable base URL** is what makes each additional compatible endpoint nearly free.

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
- **Models.** Tracked ids = `claude-opus-4-8` (1M ctx), `claude-sonnet-4-6`, `claude-sonnet-5`, `claude-haiku-4-5`, `claude-fable-5` (§6.5). Opus 4.8 is the safe default top tier. **Reasoning control (see §7.1 for the full native spec):** Opus 4.8 and Sonnet 4.6 take a native `output_config.effort` enum plus a `thinking` on/off toggle (adaptive-only when on); **Haiku 4.5 has no `effort` field** — its only reasoning-depth control is `thinking:{type:"enabled",budget_tokens}`. All three *can* be disabled (omit `thinking` / `type:"disabled"`); Opus 4.8 is **not** always-on (that is Fable 5 / Mythos 5). `budget_tokens` is removed on Opus 4.8 (400). Snapshot-id nuance: Opus 4.8 / Sonnet 4.6 are genuinely **dateless pinned snapshots**, but **`claude-haiku-4-5` is an alias for the dated canonical `claude-haiku-4-5-20251001`** (both resolve). `claude-fable-5` is always-on reasoning — it is the model that cannot disable thinking, not Opus 4.8.
- **Official `anthropic-sdk-go`.** GA, idiomatic (`NewStreaming` + `message.Accumulate`), typed `*anthropic.Error` carrying status/request-id/raw body, built-in auto-retry (on by default). A single concrete error type — branch on `StatusCode`.

### 2.2 Google — Gemini API

- **SDK landscape (reference only — no Google SDK is an approved dependency, §10).** The old `github.com/google/generative-ai-go` and `cloud.google.com/go/vertexai/genai` are both deprecated; the maintained library is `google.golang.org/genai`. AgentKit speaks the Gemini wire directly over raw HTTP, so `genai.Schema` below names a **wire shape**, not a Go type in play.
- **Shape.** `[]*genai.Content{{Role, Parts}}`; **role is `"user"` or `"model"`** (not "assistant"). `Part` is a struct of optional pointer fields (`Text`, `FunctionCall`, `FunctionResponse`, …). **System prompt is `config.SystemInstruction`, not in `contents`.** Gen config on `GenerateContentConfig` (`MaxOutputTokens`, `Temperature`, `Tools`).
- **Function calling — CRITICAL CONFLICT.** Declarations pass `Parameters *genai.Schema`, an **OpenAPI-3.0 subset, NOT raw JSON Schema**. Supported: `type` (enum string `"OBJECT"` etc.), `nullable`, `required`, `format`, `description`, `properties`, `items`, `enum`, `anyOf`, `$ref`/`$defs` (written `Ref`/`Defs`). Unsupported (`$schema`, `additionalProperties`, `oneOf`/`allOf`/`not`/`const`, deep recursion) is dropped or 400s. **AgentKit must translate JSON Schema → `genai.Schema` for Google specifically.** Model returns a whole `FunctionCall{Name, Args}`; consumer replies `functionResponse` under role `user`.
- **Streaming.** `GenerateContentStream` returns **`iter.Seq2[*GenerateContentResponse, error]`**. Text deltas via `resp.Text()`. **FunctionCalls arrive whole in one chunk** (NOT streamed as partial JSON — asymmetry vs Anthropic/OpenAI). `UsageMetadata` on the final chunk.
- **Gemini DOES stream visible text incrementally, in many SSE frames per reply — measured, not assumed.** AgentKit always calls `:streamGenerateContent?alt=sse`, so every Gemini response arrives as a multi-frame SSE body. Each frame is a whole `generateContentResponse` carrying a `candidates[0].content.parts` slice, and a single reply's visible text is spread across those frames: an observed `gemini-3.1-flash-lite` answer to one prompt arrived as ten frames, splitting mid-word and mid-markdown-bullet (`"I have"`, `" access to a set of specialized tools that allow me to interact with your environment and"`, …). Two further facts the frame-splitting drags along, both confirmed against the adapter with multi-frame bodies: **a single frame's `parts` slice can itself hold two adjacent `text` parts**, and **a `thoughtSignature` can arrive in one frame while the `functionCall` it positionally binds to arrives in the next**. Only the *whole-stream* concatenation of every frame's `parts` reconstructs the reply the model actually produced; any per-frame interpretation sees transport artifacts. The "FunctionCalls arrive whole" asymmetry above is about a *single call's arguments* never being split into partial JSON, and must not be read as "Gemini sends the whole message in one frame" — it does not.
- **Errors.** `genai.APIError`; wire shape `google.rpc.Status {code,message,status,details[]}` (`status` e.g. `RESOURCE_EXHAUSTED`). Retryable: 429/500/503/504. **SDK does NOT auto-retry — AgentKit must.**
- **Retry signals.** No `Retry-After` header; delay is in the body `details[]` as `RetryInfo.retryDelay` (e.g. `"31s"`). `QuotaFailure.quotaId` distinguishes per-minute (retry) vs per-day (fail fast).
- **Usage.** `UsageMetadata{PromptTokenCount, CandidatesTokenCount, TotalTokenCount, CachedContentTokenCount}`. Cached is a read-cache counted *within* prompt tokens.
- **Auth.** Developer API key (`BackendGeminiAPI`, single string) vs Vertex (project+location+ADC). For a neutral library taking explicit credentials, **the Developer API key path is by far simplest.** **Models.** GA/stable text ids = `gemini-2.5-flash`, `gemini-2.5-pro` (tiered >200K), `gemini-3.5-flash` (current-gen default Flash, stable), and the stable cheap workhorse `gemini-3.1-flash-lite`. **The 3.x Pro reasoning model is PREVIEW-only: the served id is `gemini-3.1-pro-preview` (tiered >200K) — there is NO GA `gemini-3.1-pro` or `gemini-3-pro` text id.** Flash naming is also resolved: `gemini-3.5-flash` (stable) and `gemini-3-flash-preview` (preview, prior-gen 3 Flash) are **two distinct models**, not two names for one.
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
- **Models.** Tracked ids = `gpt-5.5-pro` (Responses-only, highest compute), `gpt-5.5` (flagship, ~1.05M ctx), `gpt-5.4` (more-affordable frontier), `gpt-5.4-mini`, `gpt-5.4-nano` (both 400K ctx), and the `gpt-5.6` family (`-sol`, `-terra`, `-luna`). `o3`/`o4-mini` are deprecated and superseded by the gpt-5.x reasoning models; `gpt-5.5-mini`/`gpt-5.5-nano` do not exist. Reasoning defaults differ by model — gpt-5.5 defaults to `medium`, gpt-5.4 defaults to `none` (don't assume a uniform default).
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
- **Models.** `glm-5.2` (flagship), `glm-5.1`, `glm-4.7`, `glm-4.6` — all ~200K context on the plain ids. GLM-5.2's 1M window is a **separate id**, `glm-5.2[1m]`, not a parameter on `glm-5.2`; it is not currently cataloged. Only GLM-5.2 accepts `reasoning_effort`; 5.1 and the 4.x line carry the `thinking` toggle alone (§7.1). Confirm live ids against `https://docs.z.ai/llms.txt` at integration time.
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
type ToolResultBlock struct{ ToolUseID, Name, Content string; IsError bool }
type ReasoningBlock  struct{ Opaque, Summary, BoundToID string }      // preserved thinking state (§7.2)
```

`ReasoningBlock` is first-class in the canonical model, not a provider extension: three of the five providers require prior reasoning output to be echoed back verbatim during a tool-use loop (§7.2), so the block has to survive in neutral history. `ToolResultBlock` carries `Name` alongside the id because Gemini matches results by function name (§5).

Adapters reconcile: role `assistant`→`model` for Gemini (which also puts `functionResponse` under role `user`); **system prompt is a first-class field on the state object, not a message** (matches Anthropic top-level `system` + Gemini `systemInstruction`; OpenAI gets it as an injected `developer`/`instructions`); tool-call IDs always present (§5).

### 4.2 Streaming surface
**The consumption surface is a `*Stream` exposing `Events() iter.Seq[Event]` plus terminal `Err()`, `Usage()`, `Warnings()`, and `Cost()` accessors** — the `sql.Rows`/`bufio.Scanner` pattern on Go 1.23+ range-over-func.

```go
for ev := range stream.Events() { /* TextDelta, ToolCallDelta, … */ }
if err := stream.Err(); err != nil { ... }
usage := stream.Usage()
```

Iterators beat channels (which leak goroutines on early `break` and force `select` plumbing) and callbacks (lose composability/early-exit). Early `break` makes `yield` return false → iterator returns and runs `defer` cleanup (close HTTP body) with no leak. Prefer the **terminal `Err()` accessor** over `iter.Seq2[Event,error]` (one stream error invalidates the whole sequence; `Seq2` is awkward and also can't carry setup/teardown errors). Pass `context.Context` as a normal arg, checked inside the loop. Go 1.26 changes no iterator semantics — stable.

### 4.3 Tool definition & JSON Schema
Canonical internal representation = **JSON Schema as `json.RawMessage`**, cached, rendered per-provider at the boundary. The typed edge derives the schema from the input struct by reflection; there is one constructor, no hand-written-schema escape hatch. Generics live only at the registration edge, erased into a non-generic sealed interface:

```go
type Tool interface {
    Name() string
    Description() string
    JSONSchema() json.RawMessage
    Call(ctx context.Context, input json.RawMessage) (string, error)
    isTool() // sealed: construct via NewTool
}
func NewTool[In any](name, description string, fn func(context.Context, In) (string, error)) Tool
```

`Call` returns `string`, not `any` — a tool result is text fed back to the model, so serializing at the tool boundary keeps the orchestrator free of reflection.

**No single schema satisfies all three providers** (§18): `additionalProperties: false` is mandatory on every object for Anthropic and OpenAI under strict mode and a hard 400 on Gemini, so it can never be a keyword a tool author writes. Every provider therefore needs its own rendering, and the set of constructs a tool may use is the intersection of what all three dialects can express — derived from §18, not chosen. A construct outside it is rejected before any provider call rather than dropped, because §18.4 establishes that a non-strict provider accepting a keyword says nothing about whether it honors it.

**Deferred tools** are a second tool source on the same surface: a consumer may register tools as *deferred*, in which case AgentKit synthesizes one built-in `load_tools` meta-tool whose description carries a generated catalog (per-group blurb + bare tool names). The model calls `load_tools` with exact tool or group names and they become ordinary live tools from the next round trip. The heavy per-tool descriptions and schemas stay out of the request until a tool is actually loaded.

### 4.4 State/config object
A single mutable struct bundling config + history, threaded explicitly into each call; primary verbs as **methods** on it (they mutate `History`, read all config):

```go
type Conversation struct {
    Provider Provider     // swappable mid-conversation; holds its own credentials
    Model    string       // free-flow string; the vendor judges it
    Gen      GenSettings  // temperature, max tokens, native reasoning value
    System   string       // system prompt — first-class field, not a message
    History  []Message
    Tools    []Tool
    Pricing  *Pricing     // consumer-supplied rates, sibling of Model (typically a catalog lookup)
    // plus: Log, Retry, DeferredTools, MCPServers, MaxToolIterations
}
```

**Credentials are not on the conversation** — they are bound into the provider at construction, so a provider value is already authenticated and the conversation never carries secrets. **Mid-conversation provider switching is just field mutation between calls** (`c.Provider = …; c.Model = …`); history is plain `[]Message` carried over untouched — the whole reason the message model must be a neutral superset. **A `*Conversation` is one conversation owned by one goroutine — not safe for concurrent use** (standard Go stance, cf. `sql.Rows`); no hidden locking.

### 4.5 Provider abstraction interface
One narrow internal interface — translation between AgentKit's canonical types and one wire format, nothing more:

```go
type Provider interface {
    RoundTrip(ctx context.Context, req *Request) *RoundTrip
    Name() string // for error attribution
}
type Request struct {
    Model string; System string; Messages []Message
    Tools []Tool; Gen GenSettings
    ProviderOptions json.RawMessage // opaque per-provider fragment, passed through uninterpreted
}
```

The interface is **one round trip**, not one turn: the auto-tool-loop, history accumulation, and full transparency (surfacing every message/tool-call/tool-result to the consumer) live in the `Conversation` orchestration layer **above** it, not inside providers. `Request` carries no credentials — the provider was constructed with them. It also carries no pricing, no reasoning spec, and no capability metadata: **providers hold no model knowledge at all**, which is what lets a day-one model run without a library release. Reasoning lowers **by the value's shape alone** (level / budget / disabled / unset), consulting nothing about the model.

---

## 5. Tool-call identity — the load-bearing cross-provider problem

This is the single key to safe mid-conversation switching, and the providers genuinely disagree:

- Gemini has historically returned an **empty `tool_call_id`** and **matched tool results by function name, not id**; newer Gemini also emits a per-call `id`. Either way, name-matching must keep working.
- Anthropic enforces a strict id charset `^[a-zA-Z0-9_-]+$`, so OpenAI-style ids like `functions.exec:2` **corrupt an Anthropic session**.
- OpenAI's own wire key differs by surface: `tool_call_id` in Chat Completions, `call_id` in Responses.

**Resolution: AgentKit mints its own neutral tool-call ids** at write time, in Anthropic's strict charset with an `ak_` prefix, and **stores the function name alongside** every tool-call and tool-result block. A provider's id is never propagated across a switch. At send time each adapter uses whichever the provider needs — id for Anthropic and OpenAI, name (or echoed id) for Gemini — and normalizes the OpenAI key difference. History is therefore fully portable across a mid-conversation provider switch under either Gemini behavior, with no build-time branch on which one is live.

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

This is the part the product calls out and the hardest to unify, because the providers **disagree about what is included in what**. AgentKit reports both tokens **and dollar cost**: the uniform struct exposes enough **disjoint** token categories that cost is computed as `Σ bucket × rate[bucket]`, where `rate[bucket]` comes from the consumer-supplied rate row (typically a catalog lookup; the gathered rate data lives in §6.5). The disjoint-bucket design below is what makes that sum exact and provider-uniform.

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
- **OpenAI & Gemini require subtraction** to disjoint the buckets (reasoning out of output; cached out of input — three of the four native-usage shapes need the cached subtraction).
- **Anthropic is the only derived `Total`** (no native total field); for the other three, assert their native total equals the bucket sum as a sanity check (and a regression canary on provider changes).
- **Pricing dimensions** (see §6.5): distinct billed rates are uncached-input, cached-read input (discounted), cache-write input (Anthropic only; 5m=1.25×, 1h=2× base), output. Reasoning bills at the **output rate** everywhere — but the bucket is kept separate anyway (Gemini's total math depends on tracking it; cost just rates `Output + ReasoningOutput` together). The disjoint-bucket struct above covers every billable category, so the per-bucket rate tables in §6.5 price it directly.

### 6.4 Testing strategy
`net/http/httptest` + recorded fixtures + golden SSE files, table-driven. Inject a configurable base URL / `*http.Client` so tests hit a fake server returning fixtures (exercises real JSON/SSE decode + error mapping, no credits). Table-driven error-mapping tests over the §6.1 matrix. Streaming via recorded raw `.sse` byte streams under `testdata/`, asserting assembled turn + `Usage` against golden JSON (`-update` flag). Retry tests with a fake server returning 429/503 N times then 200 and an injected clock — assert attempt count, honored delay, and **that mid-stream failures are not retried**. Live integration tests gated behind `//go:build integration` **and** an env-presence skip; capture fixtures once in a recording mode, scrub keys, commit.

### 6.5 Rate data — per-model tables

The catalog (D26) carries maintained rates for the models we track; this subsection is where that data is gathered so the catalog author is not re-researching it. **Coverage is advisory, not a gate:** an uncataloged model runs, and a call with no rate source reports zero cost with a `WarnCostUnknown` warning (D16). Rates are **nano-USD per token** (1e-9 USD; published `$/1M tok × 1000`). Buckets match `RateTier`: `InputUncached`, `CacheReadInput`, `CacheWrite5m`, `CacheWrite1h`, `Output`. Reasoning tokens bill at the `Output` rate on every provider.

**`Pricing` is tiered, not flat.** `Pricing{Tiers []RateTier}` holds rates ordered by `RateTier.MinInputTokens`, and cost selects the highest tier whose threshold the turn's total input reaches. Context-length tiered models are therefore priced exactly rather than undercounted; a single-tier model is just one `RateTier` with `MinInputTokens: 0`.

**Anthropic** — `CacheWrite5m/1h` are real Anthropic buckets. ⚠ Base input/output are published and high-confidence; the **cache rates are derived from Anthropic's conventional multipliers** (read 0.1×, 5m write 1.25×, 1h write 2× base input), not read off explicit per-model columns.

| Model | InputUncached | CacheReadInput | CacheWrite5m | CacheWrite1h | Output |
|---|---|---|---|---|---|
| claude-opus-4-8 | 5000 | 500 | 6250 | 10000 | 25000 |
| claude-sonnet-4-6 | 3000 | 300 | 3750 | 6000 | 15000 |
| claude-sonnet-5 | 3000 | 300 | 3750 | 6000 | 15000 |
| claude-haiku-4-5 | 1000 | 100 | 1250 | 2000 | 5000 |
| claude-fable-5 | 10000 | 1000 | 12500 | 20000 | 50000 |

**Google Gemini** — no cache-write token bucket (caching is a read discount plus a separate per-hour storage fee AgentKit does not model); `CacheWrite5m/1h = 0`. The 3.x Pro id is the **preview** `gemini-3.1-pro-preview`; there is no GA `gemini-3.1-pro`. `gemini-2.5-pro` and `gemini-3.1-pro-preview` are tiered above 200K input tokens.

| Model | InputUncached | CacheReadInput | Output | high tier (>200K) |
|---|---|---|---|---|
| gemini-2.5-flash | 300 | 30 | 2500 | — |
| gemini-2.5-pro | 1250 | 125 | 10000 | 2500 / 250 / 15000 |
| gemini-3.5-flash | 1500 | 150 | 9000 | — |
| gemini-3.1-flash-lite | 250 | 25 | 1500 | — |
| gemini-3.1-pro-preview | 2000 | 200 | 12000 | 4000 / 400 / 18000 |

**OpenAI** — no cache-write bucket (cached-input read discount only). **`gpt-5.5-pro` has no cached-input discount** — its `CacheReadInput` equals `InputUncached` — and it is **single-tier**, with no >272K band. `gpt-5.5` and `gpt-5.4` are tiered above 272K input tokens (whole session).

| Model | InputUncached | CacheReadInput | Output | high tier (>272K) |
|---|---|---|---|---|
| gpt-5.5-pro | 30000 | 30000 | 180000 | — (flat) |
| gpt-5.5 | 5000 | 500 | 30000 | 10000 / 1000 / 45000 |
| gpt-5.4 | 2500 | 250 | 15000 | 5000 / 500 / 22500 |
| gpt-5.4-mini | 750 | 75 | 4500 | — |
| gpt-5.4-nano | 200 | 20 | 1250 | — |
| gpt-5.6-sol | 5000 | 500 | 30000 | — |
| gpt-5.6-terra | 2500 | 250 | 15000 | — |
| gpt-5.6-luna | 1000 | 100 | 6000 | — |

**Z.ai / GLM** — international `api.z.ai` USD rates; no cache-write bucket (cached-input storage currently free). Default route `zai`, also reachable via OpenRouter (`z-ai/<id>`); on the OpenRouter route the aggregator's reported cost wins at runtime (D16), and that route's own rates are cataloged from the OpenRouter route table below — the direct rates here apply only to the `zai` offering. ⚠ OpenRouter prices GLM-4.7 differently ($0.40 in / $1.75 out) because third parties host the open weights — one more reason reported-cost precedence matters at runtime.

| Model | InputUncached | CacheReadInput | Output |
|---|---|---|---|
| glm-5.2 | 1400 | 260 | 4400 |
| glm-5.1 | 1400 | 260 | 4400 |
| glm-4.7 | 600 | 110 | 2200 |
| glm-4.6 | 600 | 110 | 2200 |

**OpenRouter-routed vendors — xAI (Grok), DeepSeek, Moonshot (Kimi).** These three have no native AgentKit adapter, so the aggregator is their only route: each entry's default provider **is** `openrouter` and its wire id is the vendor-namespaced slug. Because OpenRouter reports the true charge on every response (§14.2), these rates are advisory display data that the reported figure overrides in practice.

Where OpenRouter and the vendor's own page disagree, **the OpenRouter figure is authoritative here**, since that is the route actually billed. Where OpenRouter publishes no discrete cached-read rate — which is the case for every Grok and DeepSeek model, and for Kimi K2.6 — the vendor-direct cache rate is carried instead and flagged; those cells mix sources deliberately.

xAI Grok — all four are tiered above 200K input tokens; cached-read is xAI-direct (OpenRouter publishes none). Grok's headline OpenRouter price matches xAI's low tier exactly, so there is no conflict to resolve.

| Model | OpenRouter slug | Context | InputUncached | CacheReadInput | Output | high tier (>200K) |
|---|---|---|---|---|---|---|
| grok-4.5 | `x-ai/grok-4.5` | 500000 | 2000 | 300 | 6000 | 4000 / 600 / 12000 |
| grok-4.3 | `x-ai/grok-4.3` | 1000000 | 1250 | 200 | 2500 | 2500 / 400 / 5000 |
| grok-4.20 | `x-ai/grok-4.20` | 2000000 | 1250 | 200 | 2500 | 2500 / 400 / 5000 |
| grok-4.20-multi-agent | `x-ai/grok-4.20-multi-agent` | 2000000 | 1250 | 200 | 2500 | 2500 / 400 / 5000 |

⚠ Context for the 4.20 family is contested: xAI's own docs say 1M, OpenRouter says 2M. The OpenRouter figure is used, being the served route. ⚠ For `grok-4.20-multi-agent`, effort selects the number of collaborating sub-agents (4 or 16), **not** reasoning depth, and every sub-agent's tokens bill — real cost at `high`/`xhigh` runs well above the headline rate.

DeepSeek — V4 is unified: the historical `deepseek-chat` / `deepseek-reasoner` split no longer exists, thinking being a request parameter instead, and both legacy aliases retire 2026-07-24. V3.2 is no longer addressable on DeepSeek's own API and survives only via OpenRouter and open weights; it is deliberately not cataloged. Input/output below are OpenRouter's; cache-read is DeepSeek-direct, rounded to this table's granularity (V4-Flash's true $0.0028/M → 3, V4-Pro's $0.003625/M → 4).

| Model | OpenRouter slug | Context | InputUncached | CacheReadInput | Output |
|---|---|---|---|---|---|
| deepseek-v4-flash | `deepseek/deepseek-v4-flash` | 1048576 | 90 | 3 | 180 |
| deepseek-v4-pro | `deepseek/deepseek-v4-pro` | 1048576 | 435 | 4 | 870 |

⚠ OpenRouter undercuts DeepSeek direct on V4-Flash (90/180 vs 140/280) but matches it exactly on V4-Pro. ⚠ V4-Pro's 435/870 may be a lapsed promotional rate — a third-party source puts list at 1740/3480 — but DeepSeek's own page shows it plainly with no expiry, so the published figure is used.

Moonshot Kimi — the instruct-vs-thinking model split is gone: `kimi-k2-thinking` and the rest of the K2 preview family retired 2026-05-25, and thinking is a per-request mode from K2.5 onward. Input/output are OpenRouter's; K2.6's cache-read is Moonshot-direct (OpenRouter publishes none). K3's rates match direct exactly.

| Model | OpenRouter slug | Context | InputUncached | CacheReadInput | Output |
|---|---|---|---|---|---|
| kimi-k3 | `moonshotai/kimi-k3` | 1048576 | 3000 | 300 | 15000 |
| kimi-k2.7-code | `moonshotai/kimi-k2.7-code` | 262144 | 720 | 149 | 3490 |
| kimi-k2.6 | `moonshotai/kimi-k2.6` | 262144 | 660 | 160 | 3410 |

⚠ OpenRouter prices for the open-weight Kimi models are provider-dependent and move; K2.7-code and K2.6 both run below Moonshot direct because third parties host the weights. Deliberately excluded: `kimi-k2.5` (EOL 2026-08-31), `kimi-k2.7-code-highspeed` (Moonshot-direct only, so unreachable without a native adapter), and Moonshot's `moonshot-v1-*` era.

**OpenRouter route rates — every tracked chat model's aggregator offering (audited 2026-07-29, `GET /api/v1/models`).** Every catalog offering carries rates — including OpenRouter secondaries — because when the route reports its charge it is provably cheap to establish the rate, and having it cataloged means the consumer has all pricing at model-picking time (D26). These are the OpenRouter-published per-token prices in nano-USD for the 22 routes that are secondaries of natively-served models (the OpenRouter-primary vendors above already carry theirs). Reported cost still wins at runtime (D16); these cells serve pre-flight prediction and pickers. Bucket-mapping rules applied: OpenRouter's `input_cache_read` → `CacheReadInput`; its `input_cache_write` → `CacheWrite5m` where it is a real per-token write price (Anthropic routes, where it equals the vendor's 1.25× convention, and the OpenAI 5.6 family, likewise 1.25×), and **0 for Google routes**, whose OpenRouter `input_cache_write` figure is a storage-style number that maps to no `RateTier` bucket (same as the native Google entries); `CacheWrite1h` is 0 on every OpenRouter route (unpublished). An absent `input_cache_read` (gpt-5.5-pro) is carried equal to `InputUncached`, matching the native no-discount fact. OpenRouter publishes a single price per route — no context tiering — so every OpenRouter offering is single-tier even where the native route tiers. Fractional nano cells (provider-mix averages) are rounded half-up and flagged.

| Model (OpenRouter route) | Context | InputUncached | CacheReadInput | CacheWrite5m | Output | notes |
|---|---|---|---|---|---|---|
| claude-opus-4-8 | 1,000,000 | 5000 | 500 | 6250 | 25000 | = native rates |
| claude-sonnet-4-6 | 1,000,000 | 3000 | 300 | 3750 | 15000 | = native rates |
| claude-haiku-4-5 | 200,000 | 1000 | 100 | 1250 | 5000 | = native rates |
| claude-fable-5 | 1,000,000 | 10000 | 1000 | 12500 | 50000 | = native rates |
| claude-sonnet-5 | 1,000,000 | 2000 | 200 | 2500 | 10000 | ⚠ below native (3000/15000) |
| gemini-2.5-flash | 1,048,576 | 300 | 30 | 0 | 2500 | |
| gemini-2.5-pro | 1,048,576 | 1250 | 125 | 0 | 10000 | single-tier (native tiers >200K) |
| gemini-3.5-flash | 1,048,576 | 1500 | 150 | 0 | 9000 | |
| gemini-3.1-flash-lite | 1,048,576 | 250 | 25 | 0 | 1500 | |
| gemini-3.1-pro-preview | 1,048,576 | 2000 | 200 | 0 | 12000 | single-tier (native tiers >200K) |
| gpt-5.5-pro | 1,050,000 | 30000 | 30000 | 0 | 180000 | no cache-read discount, as native |
| gpt-5.5 | 1,050,000 | 5000 | 500 | 0 | 30000 | single-tier (native tiers >272K) |
| gpt-5.4 | 1,050,000 | 2500 | 250 | 0 | 15000 | single-tier (native tiers >272K) |
| gpt-5.4-mini | 400,000 | 750 | 75 | 0 | 4500 | |
| gpt-5.4-nano | 400,000 | 200 | 20 | 0 | 1250 | |
| gpt-5.6-sol | 1,050,000 | 5000 | 500 | 6250 | 30000 | OpenRouter bills cache writes here |
| gpt-5.6-terra | 1,050,000 | 1250 | 125 | 1563 | 7500 | ⚠ half native; cw rounded from 1562.5 |
| gpt-5.6-luna | 1,050,000 | 500 | 50 | 625 | 3000 | ⚠ half native |
| glm-5.2 | 1,048,576 | 678 | 126 | 0 | 2130 | ⚠ provider-mix averages, rounded |
| glm-5.1 | 204,800 | 966 | 179 | 0 | 3036 | ⚠ rounded (cr from 179.4) |
| glm-4.7 | 204,800 | 400 | 80 | 0 | 1750 | |
| glm-4.6 | 204,800 | 500 | 100 | 0 | 2000 | |

⚠ OpenRouter context lengths disagree with the native routes in places (glm-5.2: 1,048,576 vs `zai`'s 202,752; glm-5.1/4.7/4.6: 204,800 vs 202,752; the Claude models: 1,000,000/200,000 matching native). Each offering carries its own route's figure.



## 7. Reasoning models — native-first control + preserved cross-turn state

The models AgentKit targets are newest-generation reasoning models. Reasoning is not cosmetic — it reshapes the message model in **two** independent ways, each load-bearing:

- **§7.1 — controlling reasoning (the native-first knob).** Reasoning is set in each model's **own native term and values**, with **no cross-model enum and no translation**, plus an inspectable per-model spec in the advisory catalog. A value the model does not accept is rejected by the **vendor** and surfaces as that provider's typed error — AgentKit substitutes nothing.
- **§7.2 — preserving reasoning across tool-loop turns.** **Three providers REQUIRE the model's prior reasoning output to be echoed back, verbatim, in the next request during a tool-use loop, or the turn errors or silently degrades.** AgentKit's auto-tool-loop is exactly such a loop, so this is mandatory. It is orthogonal to §7.1.

### 7.1 Native-first reasoning control

The native vocabulary genuinely does **not** unify: most providers use a discrete **effort/level enum**, some use an integer **token budget**, and some expose only an on/off **toggle**. Values and defaults differ per model *within* a provider. This heterogeneity is why a universal cross-model enum is not viable — there is no honest ordinal ladder spanning a `budget_tokens` integer and a `low/high/xhigh/max` enum, and "nearest" is undefinable across them.

**Three axes, not one.** A model's reasoning surface answers three separate questions, and conflating them is what makes the data hard to read:

1. **What values may I send?** — an enum of native level strings, an integer budget in `[Min,Max]`, or nothing at all (a bare toggle).
2. **May I switch it on or off explicitly?** — whether the wire has an on-form and an off-form the model accepts. These are two independent permissions, not one: several models accept an explicit *on* while rejecting *off*.
3. **What happens if I send nothing?** — the provider's default, which is one of *off*, a *fixed* value equivalent to something the caller could have sent, or **dynamic**, meaning the provider decides per request and the caller has no say.

The third answer is not expressible in the vocabulary of the first. "Dynamic" is not a value on the ladder; it is the absence of a caller-chosen one. Recording it as a sendable value forces a reader to decode a magic number (Gemini's `-1`) to learn a fact about who is in control.

**Per-model native reasoning vocabulary:**

| Model | Native term (wire field) | Value kind | Accepted values / range | Default | Enable? | Disable? |
|---|---|---|---|---|---|---|
| **claude-opus-4-8** | effort (`output_config.effort`) + `thinking` on/off | enum | `low` `medium` `high` `xhigh` `max` | **off** (measured: no thinking block, 4/4) | yes (`thinking:{type:"adaptive"}`) | **yes** (omit / `type:"disabled"`) |
| **claude-sonnet-4-6** | effort (`output_config.effort`) + `thinking` on/off | enum | `low` `medium` `high` `max` (**no `xhigh`**) | **off** (measured, 4/4) | yes (adaptive) | **yes** |
| **claude-sonnet-5** | effort (`output_config.effort`) + `thinking` on/off | enum | `low` `medium` `high` `xhigh` `max` | fixed `medium` (measured: thinking 4/4) | yes | **yes** |
| **claude-opus-5** | effort (`output_config.effort`) + `thinking` on/off | enum | `low` `medium` `high` `xhigh` `max` | fixed `medium` (measured: thinking 4/4) | yes | **yes** |
| **claude-fable-5** | effort (`output_config.effort`) | enum | `low` `medium` `high` `xhigh` `max` | fixed `medium` (measured: thinking 4/4) | n/a | **no** (always-on) |
| **claude-haiku-4-5** | thinking budget (`thinking.budget_tokens`) | **int budget** | `1024 … 4096` (**no `effort` field — 400 if sent**) | **off** (measured, 4/4) | n/a | **yes** (`type:"disabled"`/omit) |
| **gpt-5.5-pro** | effort (`reasoning.effort`) | enum | `high` `xhigh` *(est.)* | dynamic (measured: reasons 2/2) | n/a | **no** (no `none`; always-on) |
| **gpt-5.5** | effort (`reasoning.effort`) | enum | `none` `low` `medium` `high` `xhigh` | fixed `medium` (measured: reasons 6/6) | n/a | yes (`none`) |
| **gpt-5.4** | effort (`reasoning.effort`) | enum | `none` `low` `medium` `high` `xhigh` | **off** (measured, 4/4) | n/a | yes (`none`) |
| **gpt-5.4-mini** | effort (`reasoning.effort`) | enum | `none` `low` `medium` `high` `xhigh` | **off** (measured, 4/4) | n/a | yes (`none`) |
| **gpt-5.4-nano** | effort (`reasoning.effort`) | enum | `none` `low` `medium` `high` `xhigh` | **off** (measured, 4/4) | n/a | yes (`none`) |
| **gpt-5.6-sol** | effort (`reasoning.effort`) | enum | `none` `low` `medium` `high` `xhigh` | dynamic (measured: reasons ~2/14, prompt-dependent) | n/a | yes (`none`) |
| **gpt-5.6-terra** | effort (`reasoning.effort`) | enum | `none` `low` `medium` `high` `xhigh` | dynamic (measured: reasons 5/6) | n/a | yes (`none`) |
| **gpt-5.6-luna** | effort (`reasoning.effort`) | enum | `none` `low` `medium` `high` `xhigh` | fixed `medium` (measured: reasons 6/6) | n/a | yes (`none`) |
| **gemini-2.5-flash** | thinking budget (`thinkingConfig.thinkingBudget`) | **int budget** | `0 … 24576`; `0`=off, `-1`=dynamic | **dynamic** (`-1`) | n/a | **yes** (`0` accepted, measured: 200, 0 thought tokens) |
| **gemini-2.5-pro** | thinking budget (`thinkingConfig.thinkingBudget`) | **int budget** | `128 … 32768`; `-1`=dynamic | **dynamic** (`-1`) | n/a | **no** (measured: `0` → 400 *"only works in thinking mode"*; `64` → 400 *"choose a value between 128 and …"*) |
| **gemini-3.5-flash** | thinking level (`thinkingConfig.thinkingLevel`) | enum | `minimal` `low` `medium` `high` | fixed `medium` (measured: reasons) | n/a | **no** (`minimal` = floor, not off) |
| **gemini-3.1-flash-lite** | thinking level (`thinkingConfig.thinkingLevel`) | enum | `minimal` `low` `medium` `high` | fixed `medium` *(by tier)* | n/a | **no** (`minimal` = floor) |
| **gemini-3.1-pro-preview** | thinking level (`thinkingConfig.thinkingLevel`) | enum | `low` `medium` `high` (**no `minimal`**) | fixed `high` (measured: 1022 reasoning tokens) | n/a | **no** (always-on) |
| **glm-5.2** | `thinking` on/off + `reasoning_effort` | enum + on/off | effort `high` `max`; `thinking.type` `enabled`/`disabled` | dynamic, reasons (measured: reasoning content on every run) | yes | **yes** (`type:"disabled"`) |
| **glm-5.1** | `thinking` on/off | on/off only | `enabled` / `disabled` (**no effort**) | dynamic, reasons (measured) | yes | **yes** |
| **glm-4.7** | `thinking` on/off | on/off only | `enabled` / `disabled` (**no effort**) | dynamic, reasons (measured) | yes | **yes** |
| **glm-4.6** | `thinking` on/off | on/off only | `enabled` / `disabled` (**no effort**) | dynamic, reasons (measured) | yes | **yes** |
| **grok-4.5** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` accepted; `enabled:false` **rejected** | **dynamic** (measured: reasons 3/3) | **yes** | **no** (measured: 400 *"Reasoning is mandatory for this endpoint and cannot be disabled"*) |
| **grok-4.3** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` / `enabled:false` | **dynamic** (measured: reasons 3/3) | **yes** | **yes** (measured: 200, 0 reasoning tokens) |
| **grok-4.20** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` / `enabled:false` | **off** (measured: 0 reasoning tokens 6/6) | **yes** (measured: 399/438 reasoning tokens) | **yes** |
| **grok-4.20-multi-agent** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` accepted; `enabled:false` **rejected** | **dynamic** (measured: 1424–2401 reasoning tokens) | **yes** | **no** (measured: 400, mandatory) |
| **deepseek-v4-flash** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` / `enabled:false` | **dynamic** (measured: reasons 1/3 novel, 2/3 control — genuinely per-request) | **yes** | **yes** |
| **deepseek-v4-pro** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` / `enabled:false` | **dynamic** (measured: reasons 2/3 novel, 2/3 control) | **yes** | **yes** |
| **kimi-k3** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` / `enabled:false` | **dynamic** (measured: reasons 3/3) | **yes** | **yes** (measured: 200, 0 reasoning tokens) |
| **kimi-k2.7-code** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` accepted; `enabled:false` **rejected** | **dynamic** (measured: reasons 2/2) | **yes** | **no** (measured: 400, mandatory) |
| **kimi-k2.6** | `reasoning` on/off (`reasoning.enabled`) | on/off | `enabled:true` / `enabled:false` | **dynamic** (measured: reasons 3/3) | **yes** | **yes** |

Rows marked *(measured)* were probed against the live API with a novel prompt (see "Measuring a default", below). Rows without that marker rest on vendor documentation.

**Reading the table for design: the value space is one of three shapes** — a discrete enum of native level strings, an integer token budget within `[min,max]` with sentinel meanings (`0`=off, `-1`=dynamic), or a bare on/off with no depth control. The awkward edges are Gemini 2.5 (`0` disables Flash but Pro's floor is 128) and Anthropic's two-axis effort-enum-plus-thinking-toggle, with Haiku dropping effort entirely.

**Off-ness is shape-dependent, and that is why it must be recorded rather than derived.** On a toggle it is an explicit off-form the model may reject. On a range it is whether zero is reachable: `gemini-2.5-flash` has `Min: 0` and an off sentinel, while `gemini-2.5-pro` has `Min: 128`, so every legal request still thinks — the range floor, not a separate switch, is what makes reasoning mandatory there. On an enum it is whether the level set contains a genuine off value, and nothing but convention distinguishes `gpt-5.4`'s `none` (which is off) from `gemini-3.5-flash`'s `minimal` (which is not). Only the range case is mechanically derivable; enum and toggle off-ness must be measured.

**Two-axis models collapse cleanly into one spec.** GLM 5.2 and DeepSeek V4 each expose an on/off toggle *plus* an effort enum; both are modelled as `Kind: Enum` with the level set and `CanDisable: true`, since a toggle is exactly "the disabled value is accepted." A toggle pinned *on* (Kimi K2.7-code, Grok 4.5, Grok 4.20-multi-agent) is `Kind: Toggle, CanEnable: true, CanDisable: false`. No fourth shape is needed.

Two vendor behaviors deliberately do **not** survive into the spec, because the spec's job is to say which values are accepted, not what the vendor does with them afterward: DeepSeek V4 silently maps `low`/`medium` **up** to `high` and `xhigh` up to `max` rather than erroring, and GLM 5.2 similarly folds its wider documented enum down to two distinct behaviors. Both are recorded as the two-value enums the vendor actually distinguishes.

#### Measuring a default

Provider defaults are largely undocumented, so they were measured: one request per model with no reasoning field on the wire at all, recording whether a reasoning block appeared and how many reasoning tokens the provider billed.

**Prompt choice dominates the result, and getting it wrong inverts the answer.** An early probe used the `$1.10` bat-and-ball puzzle, which is heavily represented in training data; models that recognize it answer from recall and emit no reasoning, producing false negatives. An identically-structured prompt with novel numbers — *"A stapler and a pencil cost $2.47 in total. The stapler costs $1.83 more than the pencil."* — reversed the verdict for five models. A second unfamiliar prompt (a train-timetable subtraction) served as the control, confirming the novel prompts discriminate rather than always firing. **Any future re-measurement must use novel inputs**, or it will report contamination as vendor behavior.

Two results only a repeated probe can produce:

- **A default can be genuinely nondeterministic.** `deepseek-v4-flash` reasoned on 1 of 3 and then 2 of 3 identical requests; `deepseek-v4-pro` on 2 of 3 and 2 of 3. There is no fixed value to record for these models, which is what "dynamic" exists to say.
- **Accepted is not the same as effective.** All nine OpenRouter-routed models return 200 for `reasoning:{effort:"low"|"high"}`, but only `grok-4.20-multi-agent` shows a differentiated response (1,744 reasoning tokens at `low` against 9,334 at `high`); the rest are flat within run-to-run noise. Acceptance of a level therefore does not establish that the model has one, and these entries stay `Kind: Toggle` until a level probe with adequate repetition says otherwise. This is an open measurement item, not a settled fact.

**A model whose default is off may still be enableable.** `grok-4.20` emitted zero reasoning tokens on 6 of 6 unset requests and 399/438 on explicit `reasoning:{enabled:true}`. Its reasoning is reachable only through an explicit on-value, so a client with no way to *enable* reasoning cannot reach that model's reasoning at all — the on-form is load-bearing, not a redundant twin of the off-form.

**Recommended introspection API (Go) — covers all three shapes with one discriminated type.** A consumer (agentrepl `--help`, a validator) reads this as data and never embeds provider knowledge:

```go
type ReasoningKind int
const (
    ReasoningEnum  ReasoningKind = iota // discrete native level strings
    ReasoningRange                      // integer token budget in [Min,Max]
    ReasoningToggle                     // on/off only, no depth control
)

// ReasoningSpec is the inspectable native-vocabulary descriptor for one model.
type ReasoningSpec struct {
    Term       string           // native label: "effort" | "thinking level" | "thinking budget" | "thinking"
    Kind       ReasoningKind
    Levels     []string         // Kind==Enum: accepted native strings, in the model's own order
    Min, Max   int              // Kind==Range: inclusive valid budget range
    Sentinels  []Sentinel       // Kind==Range: magic ints with native meaning (0=off, -1=dynamic)
    CanEnable  bool             // an explicit on-form exists and the model accepts it (Kind==Toggle only)
    CanDisable bool             // an explicit off-form exists and the model accepts it
    Default    ReasoningDefault // what the provider does when nothing is sent
}
type Sentinel struct{ Value int; Meaning string } // e.g. {0,"off"}, {-1,"dynamic"}

// ReasoningDefault answers "what happens if I send nothing?" — deliberately a
// different type from ReasoningValue, which answers "what may I send?".
type ReasoningDefault struct {
    Mode  ReasoningDefaultMode
    Value ReasoningValue // set only when Mode == DefaultFixed
}
type ReasoningDefaultMode int
const (
    DefaultUnaudited ReasoningDefaultMode = iota // zero value: not measured or sourced
    DefaultOff                                   // does not reason unless asked
    DefaultFixed                                 // equivalent to the caller sending Value
    DefaultDynamic                               // provider chooses per request; not caller-controllable
)

// Introspection is catalog data, not a provider interface: catalog.Lookup(model)
// returns the Entry (whose Reasoning is a *ReasoningSpec, nil where unknown),
// catalog.Check(model, v) reports whether a value is accepted, and
// catalog.ListByProvider(p) enumerates entries for picker/--help rendering.
// Providers expose no introspection of their own — they hold no model knowledge.
```

`DefaultUnaudited` as the **zero value** is load-bearing: an unrecorded default and a measured "off" must not read alike, or every un-probed entry silently claims a fact nobody established.

**Setting reasoning natively — a tagged `ReasoningValue`** carrying exactly one native form, so the native value flows to the adapter untranslated. The zero value means "unset → whatever the provider does, no fields sent":

```go
type ReasoningValue struct { /* tag + level string + budget int, fields unexported */ }
func Level(s string) ReasoningValue    // native level: Level("high"), Level("xhigh")
func Budget(n int) ReasoningValue      // native budget: Budget(8000)
func EnableReasoning() ReasoningValue  // explicit on (lowered to each wire's native on-form)
func DisableReasoning() ReasoningValue // explicit off (lowered to each wire's native off-form)
```

`DisableReasoning()` and `EnableReasoning()` are first-class rather than overloaded `0`/`none`/`max`, because which value means off or on is wire-specific — the consumer expresses intent and the adapter lowers it to that wire's native form (`{"reasoning":{"enabled":false}}`, `thinking:{type:"disabled"}`, `reasoning.effort:"none"`, `thinkingBudget:0`). **Not every wire has a bare on-form**: OpenRouter (`reasoning.enabled:true`), Z.ai (`thinking.type:"enabled"`) and Anthropic (`thinking.type:"adaptive"`) do, while Google and OpenAI express on-ness only as a budget or a level and have none.

**No AgentKit-side validation, fallback, or coercion — the vendor is the judge.** The value the consumer sets is lowered to the wire **by its shape alone** and sent as given. A value the model accepts is honored exactly; one it does not is rejected by the provider and surfaces as that provider's typed error, attributable and loud. Nothing is ever silently substituted, and there is no "apply the model's default instead" path.

The zero value means "unset": an untouched `GenSettings` sends no reasoning fields at all, so a consumer that ignores reasoning is unaffected and a non-reasoning model is safe by default even when uncataloged. This is also what makes the catalog's recorded default *true rather than aspirational* — because AgentKit sends nothing, what the user gets on an untouched setting is exactly the provider's own behavior, matching what they would have got calling the vendor API directly.

**Pre-send politeness is a consumer choice, outside the request path.** A consumer that wants to validate a `/set` before spending a round trip, or render a model's vocabulary in `--help`, calls `catalog.Check(model, v)` — advisory only. The warning channel that *does* exist is narrow and structural: `Warning{Setting, Code, Detail}` with codes `WarnToolChoiceForced`, `WarnToolSchemaLossy`, and `WarnCostUnknown`, read off the stream via `Warnings()` alongside `Err()`/`Usage()`/`Cost()`. There are no reasoning warning codes, because there is no reasoning fallback to report.

**Why a universal `ReasoningEffort` enum is not viable.** (1) A cross-model "nearest" requires rebuilding the very ordinal ladder being removed, and it is **undefinable** across a discrete enum and a `thinkingBudget` integer without arbitrary bucketing. (2) The per-model value sets genuinely differ even *within* effort-enum providers (`xhigh` exists on Opus but not Sonnet; gpt-5.4 defaults to `none` while gpt-5.5 defaults to `medium`; GLM and DeepSeek use `high`/`max`, not `low`/`medium`/`high`), so one enum would either over-promise values a model rejects or under-expose values it supports. (3) Nine of the tracked models have **no** level vocabulary at all, so a level-shaped API cannot express them. (4) Silent lossy coercion is precisely the bug class a verification harness exists to expose. Native-first plus an advisory catalog is strictly more truthful and not materially more code, since the per-provider native lowering had to exist in each adapter anyway.

**Why a single fixed default (e.g. "medium everywhere") was rejected.** It is conceptually simple and practically unworkable: nine tracked models have no levels to receive it; six models (three Anthropic 4.x, three gpt-5.4) default to off, so injecting a level would silently turn reasoning on and bill the user for tokens they never requested; `gemini-2.5-pro` has no off and a floor of 128; and models whose default is genuinely dynamic have no level to match. It would also make AgentKit substitute a value the consumer did not choose, which is the exact behavior native-first exists to forbid. Pass-through matches the vendor API by construction and costs no code, since an unset value already sends nothing.

### 7.2 Preserved cross-turn reasoning state (unchanged by native-first)

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
2. **Native-first reasoning knob + introspection (§7.1)** on the request/state — a tagged `ReasoningValue` (native level / native budget / disabled / unset), validated against the selected model's `ReasoningSpec` at request-build time, warning + falling back to the model's default on non-native input. *(This replaces the former "uniform `ReasoningEffort` enum" recommendation; §7.1 is the authority.)* The §7.1 reasoning-control knob and this §7.2 `ReasoningBlock` are independent: the knob says *how hard* to think (native, validated, fallible); the block carries the model's *prior* opaque reasoning state forward verbatim. Both round-trip through the auto-loop.
3. **Surface reasoning summary text** as a distinct streaming event/part (honoring the full-transparency promise), separate from the opaque replay payload. Default providers to emit summaries (Anthropic `display:"summarized"`, OpenAI `summary:"auto"`, Google `includeThoughts:true`). Raw CoT is unavailable on all but Z.ai, so "transparency" means summaries nearly everywhere.
4. **OpenAI:** default `store:false` + auto-inject `include:["reasoning.encrypted_content"]` so the stateless multi-turn tool loop has its reasoning chain.

⚠ **Model-id flags:** `gpt-5.4-nano` **does exist**, as do `gpt-5.4-mini`, `gpt-5.5`, `gpt-5.5-pro`, `gpt-5.4`; `gpt-5.5-mini`/`gpt-5.5-nano` do **not** exist; `o3`/`o4-mini` exist but are **deprecated** (drop). Gemini flash naming: `gemini-3.5-flash` (stable) ≠ `gemini-3-flash-preview` (preview); the 3.x **Pro** is preview-only (`gemini-3.1-pro-preview`; no GA `gemini-3.1-pro`). Gemini 3.x uses `thinkingLevel`, 2.5 uses `thinkingBudget` (an int; deprecated-but-accepted on 3.x — never send both, it 400s).

Reasoning-specific open items (see the §7.1 table for the per-model spec):
- **CORRECTION — Opus 4.8 *can* be disabled.** Current Anthropic docs (effort + adaptive-thinking pages) show Opus 4.8 thinking is **off unless `thinking:{type:"adaptive"}` is set**, and `{type:"disabled"}` is accepted — so the prior "always-on / cannot disable" claim was **wrong for Opus 4.8** and attaches instead to **Fable 5 / Mythos 5** (not in the curated set). Confirmed unchanged for Opus 4.8: `budget_tokens` removed (400), effort enum (default `high`).
- **CORRECTION — Haiku 4.5 has no `effort` field.** It is a classic extended-thinking model: `thinking:{type:"enabled",budget_tokens}` only; sending `effort` 400s. Its native reasoning term is a **token budget**, not an effort enum — a genuine native divergence the universal enum would have masked.
- **Sonnet 4.6 effort set excludes `xhigh`** (`low/medium/high/max`); `xhigh` is Opus-only (and Fable/Mythos 5).
- **`gpt-5.5-pro` effort levels/default are estimates** (`high`/`xhigh`, default `high`, no `none` → always-on): the model page renders the field but did not surface the exact enumeration; grounded on the consistent Pro lineage (gpt-5-pro = `high`-only; gpt-5.2-pro = `medium/high/xhigh`). Verify against a live 400 before relying on it.
- **`gpt-5.4-mini`/`-nano` defaults** (`none`) and their acceptance of `xhigh` are estimates (official launch post says `xhigh` was added for both; one secondary source disputes nano) — gate `xhigh` on nano behind a check if strictness matters.
- **Gemini 2.5 budget ranges** are verified (Flash `0–24576`, Pro `128–32768`); `-1`=dynamic, `0`=off (Flash only; Pro rejects `0`). **`gemini-3.1-flash-lite` default** (`medium`) is assigned by tier analogy — verify via a live `models.get`.
- **GLM `reasoning_effort` is glm-5.2-confirmed, glm-5.1-likely** (`high`/`max`, default `max`); glm-4.6/4.7 have on/off only. Hosted z.ai uses `thinking:{type:"disabled"}` to disable — **not** the open-weights `enable_thinking` field.
- Still genuinely open (preservation side, §7.2): Z.ai hard-fail-vs-degrade on dropped `reasoning_content` under preserve mode; Z.ai's exact error-envelope shape (the error-code page 404s — Zhipu string-numeric `code` assumed; verify against a live 4xx).

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

## 9. MCP client — remote tool servers

The product promises **remote MCP tool servers**. AgentKit is the MCP **client**; it connects to consumer-attached **remote** servers (network only — no subprocess/stdio), discovers their tools, and feeds them into the same auto-loop as custom tools, uniformly across every provider. The design target is small and well-bounded: AgentKit needs **only the client side** and **only tools** (resources/prompts deferred). The findings below are external — MCP is a published open protocol with an official spec.

### 9.1 Protocol & transport
- **Spec revision.** MCP ships dated revisions; the current GA revision is **`2025-11-25`** (a `2026-06-30` revision is in development). The transport/auth/tools mechanics below are stable across `2025-06-18` → `2025-11-25`. **Pin a revision and send it explicitly** (see header note below). Everything is **JSON-RPC 2.0** over the transport.
- **Target transport = Streamable HTTP.** Two remote transports exist: the **legacy HTTP+SSE** (`2024-11-05`, two endpoints) — **deprecated, do not target** — and **Streamable HTTP** (since `2025-03-26`, current) — **the one to build against**. Streamable HTTP is a **single endpoint URL** that accepts POST (JSON-RPC request; the consumer supplies this URL per server) and optional GET (a standalone server→client SSE stream for notifications, which a tools-only client may skip). **Each request POST gets one of two response content-types — `application/json` (single response) or `text/event-stream` (an SSE stream that eventually carries the response for long-running calls) — and the client must handle BOTH.** A POST carrying only a notification/response returns `202 Accepted`, no body.
- **Client lifecycle.** `initialize` (client sends preferred `protocolVersion` + `capabilities` + `clientInfo`; server replies with its chosen version + capabilities) → `notifications/initialized` → then operations. **Discovery = `tools/list`** (paginated via `cursor`/`nextCursor` — loop until `nextCursor` absent). **Invocation = `tools/call`** with `{name, arguments}`.
- **Wire shapes the design needs.** A tool definition carries `name`, optional `title` (display-only), `description`, **`inputSchema` (JSON Schema)**, optional `outputSchema`, optional untrusted `annotations`. A `tools/call` **result** is `{content[], structuredContent?, isError?}` where `content[]` is an ordered array of typed blocks (`text`, `image`, `audio`, `resource_link`, embedded `resource`). For a **text-only** product, the `text` blocks are what matter (see §9.3 collapse rule).
- **Dynamic tool sets.** `notifications/tools/list_changed` exists (server must declare `capabilities.tools.listChanged`); on receipt the client re-runs `tools/list`. **v1 may defer honoring it** (re-list on demand / on attach) — and there's a caching reason to (§9.3).
- **Session & version headers.** Server MAY return an `Mcp-Session-Id` header on the `InitializeResult`; if so the client **MUST** echo it on every subsequent request. After init, the client **MUST** also send `MCP-Protocol-Version: <negotiated>` on every request — **omitting it makes servers assume `2025-03-26`**, so always set it explicitly. Clean detach = best-effort HTTP `DELETE` with the session header (ignore a `405`).

### 9.2 Client implementation — raw HTTP (decided: no library)
**AgentKit hand-rolls a minimal raw-HTTP Streamable-HTTP MCP client over the standard library** (the MCP `go-sdk` is not an approved dependency).** This is tractable because AgentKit needs only a *sliver* of the protocol — **4 client calls** (`initialize`, `notifications/initialized`, `tools/list`, `tools/call`), tools only, no server/resources/prompts — and is *already* writing bespoke SSE parsing and JSON handling for the LLM providers. The marginal new machinery is one Streamable-HTTP client: POST a JSON-RPC body; **accept either an `application/json` response or a `text/event-stream` stream** and read the JSON-RPC response out of whichever arrives; carry the `Mcp-Session-Id` and `MCP-Protocol-Version` headers; do the `initialize`→`initialized` handshake. On the order of a few hundred lines, not thousands.

*(Reference only — the existence of the mature official `github.com/modelcontextprotocol/go-sdk`, Anthropic+Google-maintained at stable v1.x with a clean `StreamableClientTransport`/`Connect`/`CallTool` API, is noted so the design author knows the protocol surface is well-trodden and can mirror its proven shapes — `HTTPClient` round-tripper for auth injection, iterator-based `tools/list` pagination. It is **not** a dependency option.)* The one part to get right is the **dual JSON-vs-SSE response path** on a request POST (a server may answer a `tools/call` with either) — AgentKit already owns provider SSE code, so this reuses that muscle rather than introducing new risk.

### 9.3 Integrating MCP tools into the canonical model
- **Reuse, don't special-case.** On attach, connect + `tools/list` once, wrap each MCP tool as an ordinary `Tool` (§4.3) that closes over its server connection, and concatenate into the same registry the auto-loop already drives. The model and providers see no difference. **Route a call back to its server by a stored `(serverHandle, originalMCPName)` binding — NOT by re-parsing a prefix out of the name** (sanitization below is lossy/irreversible). This is the dominant prior-art pattern (Vercel AI SDK, OpenAI Agents SDK, LangChain adapters, eino).
- **Prefixing + name sanitization (separator = `_`).** Provider tool-name charsets are strict: **Anthropic and OpenAI both require `^[a-zA-Z0-9_-]{1,64}$`** — so `.`, `/`, `:` are **illegal** (Gemini tolerates `.`, the others do not). Real MCP servers ship tool names with dots/slashes (`git.commit`, `multi_tool_use.parallel`), which Anthropic/OpenAI **reject**. Recommended scheme: final name = `<serverName>_<mcpToolName>`, then **sanitize the whole string to `^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`** (replace illegal chars with `_`, ensure a letter/`_` start, truncate to ≤64 with a hash suffix on overflow to keep uniqueness). Keep the sanitized→`(server, originalName)` map for routing.
- **Collision = uniform error (already promised).** Detect duplicates **after** prefixing+sanitization (two raw names can sanitize to the same string), against the full merged set *including native tools*, and surface AgentKit's uniform collision error. This matches the **best** prior art (OpenAI Agents SDK hard-errors; LiteLLM prefixes) and **avoids the common anti-pattern** (Vercel/LangChain/langchaingo/eino all silently last-wins shadow).
- **Schema-translation risk (Gemini) — the real one.** MCP `inputSchema` is arbitrary third-party JSON Schema (draft 2020-12; `$ref`/`$defs`/`oneOf`/`additionalProperties` all common) that AgentKit does not control. The Gemini wire schema has no `oneOf`/`$ref`/`$defs`/`additionalProperties`, so under Google a real MCP schema would silently drop constraints or 400 (e.g. an untyped `array` with no `items`). No surveyed library handles this well. The §4.3 translation therefore runs faithfully (inline `$ref`/`$defs`, map `oneOf`→`anyOf`, strip `$schema`/`additionalProperties`, synthesize `items` for untyped arrays) and **emits `WarnToolSchemaLossy` naming the dropped keywords** rather than degrading silently. Scope the conversion to the **Google boundary only**: don't fail registration when the active provider is Anthropic/OpenAI (which pass JSON Schema near-verbatim); the degradation + warning surfaces if/when the conversation switches to Gemini.
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

## 10. Dependency policy — raw HTTP, approved libraries only

External libraries require **explicit per-case human approval**; the default is the standard library. The approved set today is `github.com/invopop/jsonschema` (struct→JSON-Schema derivation for `NewTool[In]`) and its transitive closure. Everything else is unapproved and therefore absent — in particular the official provider SDKs, the MCP `go-sdk`, and `cenkalti/backoff`.

The consequence is that **every provider adapter and the MCP client are raw `net/http`**, over a shared `internal/httpx`. Hand-rolled accordingly: SSE framing and parsing, partial-JSON tool-call accumulation, retry/backoff with full jitter, and error/usage extraction per provider. This is also the direction the strongest prior art goes — the serious neutral gateways (gollm, langchaingo, bifrost, LiteLLM) hand-roll HTTP rather than carry several heavy, divergent SDKs, and the three official Go SDKs share no base type (OpenAI and Anthropic use `ssestream.Stream[T]`, Google uses `iter.Seq2`), so wrapping them would mean writing the unifying layer regardless.

The SDK details recorded in §2 are reference for *what behavior* AgentKit re-implements — auto-retry policies, error field sets, stream accumulation — not options under consideration.


## 12. Embeddings — a generic API across providers

The embeddings surface mirrors the chat surface as closely as possible: provider + model + credentials in a consumer-held embedder object, free-flow model strings with advisory catalog metadata, consumer-supplied per-model pricing, explicit credentials, the same error/retry rails. The animating goal: **changing the embedding model is a config-only problem** — only the provider/model name changes. Embeddings are **not hot-swappable** — a vector is comparable only to vectors from the *exact same* model, so any switch means re-embedding the corpus. "Config-only" is a statement about *code and configuration*, never about *stored vectors*. Per-provider doc URLs are at §12.8.

### 12.1 The central finding

The "config-only swap" promise is **mostly honest, but only if AgentKit's adapter layer absorbs four real divergences** (dimensionality bounds, normalization, batch limits, truncation behavior) and exposes **one** unavoidable per-call input (query-vs-document role). The embeddings surface is **OpenAI and Google only** — a deliberate subset of the five chat providers. Anthropic offers no first-party embeddings API at all; Z.ai and OpenRouter embeddings could not be confirmed on the endpoints AgentKit uses, so both were evaluated and excluded rather than rejected outright (see §12.2).

### 12.2 Provider surfaces (verified)

**Anthropic — excluded, grounded.** Anthropic has **no first-party embeddings API or model** and **no embeddings endpoint on api.anthropic.com**. Their docs say verbatim *"Anthropic does not offer its own embedding model"* and direct customers to **Voyage AI** (a MongoDB product since Feb 2025 — *not* owned by Anthropic; the relationship is a recommendation). Anthropic stays a first-class chat provider and is simply absent from the embeddings surface.

**OpenAI — clean, OpenAI-native.**
- Models: `text-embedding-3-small` (native **1536** dims), `text-embedding-3-large` (native **3072**), legacy `text-embedding-ada-002` (1536, fixed — no shortening).
- Dimensions **requestable** on the v3 models via `dimensions` (Matryoshka). Returned vectors are **L2-normalized**, including when shortened.
- Endpoint `POST /v1/embeddings`; `input` accepts **string or array** (batch up to **2048** items); **8192** tokens/input; over-length **errors (400), never silent truncation**. `encoding_format` float|base64.
- Response `data[]` with `index` (order guaranteed); `usage` = `prompt_tokens`/`total_tokens` only (**no output tokens**).
- Pricing /1M input tokens: 3-small **$0.02**, 3-large **$0.13**, ada-002 **$0.10** (Batch API ≈ 50% off).
- Auth: `Authorization: Bearer <key>`. **No task-type / query-vs-document distinction** (symmetric).

**Google (Gemini API) — capable, but the most divergent.**
- Current model: **`gemini-embedding-001`** (GA, native **3072** dims). A newer multimodal **`gemini-embedding-2`** exists (3072, 8192-token input; GA-vs-preview status ambiguous — flag). **Deprecated/shut down: `text-embedding-004` (Jan 2026), `embedding-001` (Oct 2025)** — do not target. Baseline = `gemini-embedding-001`.
- Dimensions **requestable** via `outputDimensionality` (range **128–3072**; recommended 768/1536/3072).
- **`task_type`** (RETRIEVAL_QUERY, RETRIEVAL_DOCUMENT, SEMANTIC_SIMILARITY, CLASSIFICATION, CLUSTERING, …): optional but **changes the produced vector**; query and document sides are **asymmetric**. `gemini-embedding-2` drops `task_type` (instructions go in the prompt) — a per-model capability difference.
- Endpoint base `…/v1beta/models/<model>:embedContent` (single) and `:batchEmbedContents` (array). **Max input 2048 tokens** for `-001`. Default behavior **silently truncates** oversized input unless `autoTruncate:false`.
- **Normalization footgun:** full 3072 is normalized, but `gemini-embedding-001` **does not normalize truncated (<3072) outputs — the caller must**. (`gemini-embedding-2` does auto-normalize.)
- Usage reported (`usageMetadata.promptTokenCount`). Pricing /1M input tokens: `gemini-embedding-001` **$0.15** ($0.075 batch). Free tier exists.
- Auth: **Gemini API key** via `x-goog-api-key` (simple). *Vertex AI* is a different endpoint requiring **OAuth2 bearer + project/region in URL** — AgentKit should use the **Gemini API key** path to mirror the chat provider, not Vertex.

**Z.ai and OpenRouter — evaluated, excluded.** Z.ai is OpenAI-compatible in shape (`POST /api/paas/v4/embeddings`, `{model,input,dimensions}` → `{data[],usage}`, model `embedding-3` at native 2048 dims, batch max 64), **but embeddings are fully documented and priced only on the China platform** (`open.bigmodel.cn`, `embedding-3` at ¥0.5 CNY/1M). On the international `api.z.ai` endpoint — the one AgentKit uses — the official SDKs expose `embeddings.create`, yet the international docs carry no embeddings page and the international pricing page lists no embedding model in USD. Availability and USD pricing are therefore unconfirmed on the endpoint AgentKit actually calls, and Z.ai ships **no** `Embedder`. OpenRouter is excluded on the same grounds. Both remain first-class chat providers; embeddings for them are deferred, not rejected, should first-class endpoints be confirmed.

### 12.3 Where "config-only swap" holds, needs a caveat, or fails

**TRUE — adapter absorbs it, consumer changes only provider/model name:**
- Parameter-name and wire differences (`dimensions` vs `outputDimensionality`, base URL, auth header, request/response JSON).
- A uniform `[]string` **batch** surface — *provided* the adapter **auto-chunks** to each provider's item/token ceiling (OpenAI 2048 / Google batch ~hundreds / **Z.ai 64**) and reassembles in input order. Without chunking, a batch sized for OpenAI breaks on Z.ai.
- A requested **target dimension** — honored when the chosen model supports it. Note this is **per-model config, not a cross-provider constraint**: vectors are incomparable across models regardless of dimension, so dimension parity buys nothing on the similarity math. The *only* reason a consumer might standardize a dimension across models is a **fixed-dimension downstream store** (e.g. a `pgvector(1024)` column) — a consumer storage-schema convenience AgentKit neither knows nor promises. AgentKit's job is just to honor a requested dimension or **fail loud** if that model can't produce it. (Per-model support, for the consumer who chooses to standardize: 1024 is producible on OpenAI 3-*/Google `gemini-embedding-001`/Z.ai `embedding-3`; 768 on OpenAI/Google but not Z.ai's enumerated set.)

**TRUE ONLY WITH A CAVEAT / requires the library to do work:**
- **Normalization.** Only config-only if AgentKit **L2-normalizes client-side** (mandatory for truncated `gemini-embedding-001`; undocumented for Z.ai). The honest promise is *"AgentKit returns unit-normalized vectors,"* not *"providers do."* Re-normalizing already-unit vectors is idempotent and cheap.
- **Dimension request.** Per-model config (not cross-provider — see above). Bounded by native size and per-model support; legacy models (`ada-002`, `embedding-2`) can't downsize. **Fail loud** on an unsupported request rather than silently returning native dims.
- **Truncation.** Only uniform if AgentKit forces **fail-loud** (`autoTruncate:false` on Google so it errors like OpenAI) and documents per-model token ceilings. Silent truncation is the most dangerous case — it corrupts retrieval quality with no signal.

**FALSE — cannot be pure config:**
- **Query-vs-document role** must be a **per-call input** (e.g. an `InputType: Query | Document | Unspecified` enum), not config. Google needs it for retrieval quality (asymmetric vectors); OpenAI/Z.ai safely ignore it. Hiding it in config would force a consumer to hold two clients. Expose it from day one so a later swap *to* Google doesn't silently underperform.
- **The vectors themselves.** Different model ⇒ incomparable vectors ⇒ re-embed the corpus. Permanent and already accepted.

### 12.4 Usage & pricing — the chat types do not fit

Embeddings bill **input tokens only**; there are no output tokens, no reasoning, and (in practice) no cache tiers. The chat `Usage` (`usage.go`: `Output`, `ReasoningOutput`, `CacheWrite5m/1h`, …) and `Pricing.Cost` (`pricing.go`: multiplies `(Output+ReasoningOutput)·tier.Output`, selects context tiers) are structurally wrong here — reusing them carries ~six always-zero fields and an output term that shouldn't exist.

Embeddings therefore carry **their own small pair of types** rather than reusing chat's: `EmbeddingUsage{InputTokens, Total}` (Total == InputTokens for both providers) and a **flat single-rate** `EmbeddingPricing{InputToken}` in nano-USD per token — no tiers, no output term. Neither provider tiers embeddings by context length, so a flat rate is both sufficient and accurate. They reuse the root `Cost` type so dollar accounting stays one currency. Rates: OpenAI `text-embedding-3-small` $0.02/1M (20), `text-embedding-3-large` $0.13/1M (130); Google `gemini-embedding-001` $0.15/1M (150).

### 12.5 Codebase fit — the shipped shape

The embeddings surface plugs into the existing rails with a clear reuse/new split (anchors: `orchestration.go` Provider interface and `Conversation`, `pricing.go`, `usage.go`, `error.go`, `retry.go`, `internal/httpx`, `internal/openaicompat`).

**Reused directly:** the `Error` taxonomy and sentinel categories, `RetryPolicy` + backoff, `internal/httpx`, and per-provider credential injection (`New(apiKey, opts…)` plus the provider's auth header).

**Parallel to chat, deliberately not overloading chat types:**
- A separate **`EmbeddingProvider` SPI** — `Embed(ctx, *EmbedRequest) *EmbedRoundTrip` plus `Name()` — kept distinct from the chat `Provider` because embeddings have no System/Tools/History/Reasoning. A provider package implements both interfaces (`openai.New(...)` exposes `NewEmbedder`, as does `google.New(...)`).
- A consumer-held **`Embedder`** (provider, model, target dimension, role, retry, log, optional `Pricing`) — the embeddings analogue of `Conversation`, minus the conversational machinery. One call returns vectors + usage; **no streaming, no tool loop.**
- **Request/result types:** `EmbedRequest{Model, Inputs []string, Role InputType, Dimensions, Retry}` and `EmbedResult{Vectors [][]float32, Warnings}`, with usage and cost read through accessors.
- **Advisory catalog metadata** per embedding model — `EmbeddingInfo{Pricing, NativeDimension, MinDimension, MaxDimension, MaxInputTokens}` — for up-front display and consumer-side validation. It is advisory: the `Embedder` does not consult it, and an uncataloged embedding model runs.
- An **`internal/openaicompat` embeddings variant** (`/v1/embeddings` shape) used by OpenAI; Google has its own `embedContent`/`batchEmbedContents` adapter (different shape, `task_type`, normalization fix-up).
- **Adapter-owned behaviors** that make the promise honest: unconditional client-side L2-normalization, batch auto-chunking with order-preserving reassembly (2048 inputs per request on the OpenAI-compatible path, 100 on Google), and fail-loud over-length handling (`autoTruncate:false` on Google).

**Fail-loud on dimensions is a post-response check, not a pre-flight table lookup.** `Embedder` compares the *returned* vector length against the requested `Dimensions` and returns `ErrInvalidConfig` on mismatch. Dimension bounds live in the advisory catalog and are never enforced in the request path — the provider is the enforcer, consistent with free-flow models.

**Cost is consumer-supplied and optional.** `Embedder.Pricing` is a `*EmbeddingPricing` the consumer sets, typically from one catalog lookup. When it is nil the call still succeeds and reports zero cost with a `WarnCostUnknown` warning — the same warned-zero contract chat uses. There is no "priced by construction" guarantee, because there is no closed model set to guarantee it over.

**The permanent caveats**, documented in the promise rather than hidden: vectors from different models are incomparable, so a model switch means re-embedding the corpus; and tagging inputs query-vs-document is what lets a switched-in provider reach full retrieval quality (Google uses `task_type`; OpenAI ignores the distinction).

### 12.8 Provider doc URLs (embeddings)

- OpenAI: `developers.openai.com/api/project/guides/embeddings`; model pages `…/models/text-embedding-3-large` / `-3-small`; launch post `openai.com/index/new-embedding-models-and-api-updates/`.
- Google: `ai.google.dev/gemini-api/project/embeddings`; `…/models/gemini-embedding-001`; `ai.google.dev/api/embeddings`; pricing `ai.google.dev/gemini-api/project/pricing`; deprecations `ai.google.dev/gemini-api/project/deprecations`.
- Z.ai: China (authoritative) `docs.bigmodel.cn/cn/guide/models/embedding/embedding-3`; international `docs.z.ai` (no embeddings page); SDKs `github.com/zai-org/z-ai-sdk-python` / `…-java`.
- Anthropic (no first-party embeddings): `platform.claude.com/project/en/project/build-with-claude/embeddings` ("Anthropic does not offer its own embedding model").

## 13. Progressive tool discovery — deferred tools

The context cost of a large tool set is front-loaded: every registered tool's full description + input JSON Schema is serialized into the provider `tools` array on **every** round-trip, resident for the life of the conversation whether or not the model ever calls it. A measured real-world example (the ikigenba suite's `prompts` service, which exposes its sibling services' MCP tools to an in-run agent): 6 services, 79 tools, ~31 KB of serialized descriptions+schemas ≈ 8k tokens; a full deployment (~11 services, ~120 tools) lands around 15–18k tokens per round-trip. Beyond raw cost there is tool-choice dilution — models pick worse when offered ~120 tools than when offered the relevant dozen.

### 13.1 Prior art

- **Claude Code (the agent harness).** Deferred tools appear by *name only* in context, alongside a one-paragraph blurb per MCP server. A built-in `ToolSearch` tool fetches a named tool's full description + schema on demand — and that act makes the tool *directly callable* from then on; calling an unfetched deferred tool fails with a validation error pointing at ToolSearch. Per-tool schemas are pay-as-you-go; loaded tools are never unloaded within a session.
- **Anthropic API "tool search" beta.** Provider-native equivalent: tools marked deferred are excluded from the prompt; a server-side search tool loads them mid-conversation. Confirms the pattern is load-bearing enough for a first-party API feature — but it is **single-provider**, so a cross-provider library must implement the pattern above its SPI, not delegate to it.
- **MCP ecosystem.** MCP servers publish a server-level `instructions` string in the `initialize` response — a natural source for the per-group blurb a catalog needs (the ikigenba services all publish one).

### 13.2 Mechanics that matter for a library implementation

- **The provider contract is per-request.** Every provider (Anthropic, OpenAI Responses, Gemini, Z.ai) treats the `tools` array as free to differ between calls; a tools array that *grows between round-trips of one turn* is accepted — this is exactly what harnesses like Claude Code already do in production. Still a real-substrate claim: a mock accepts anything, so only a live call proves it.
- **History references constrain removal, not addition.** Replayed `tool_use` blocks referencing a tool absent from the current `tools` array are rejected or degraded by some providers — so *unloading* is hazardous while *loading* is safe. Monotonic growth is the safe shape.
- **Prompt-cache interaction (Anthropic explicit caching).** The cache matches byte-identical prefixes at breakpoints. Appending new tools at the **tail** of the tools array preserves the previously cached prefix (old prefix re-read from cache, fresh write for the tail only); inserting into the middle — e.g. by re-sorting the merged set alphabetically — invalidates everything after the insertion point on every subsequent round-trip. Gemini/OpenAI implicit caching is prefix-based too, but Gemini's dialect conversion re-sorts tool declarations anyway (adapter-owned), so the append guarantee is meaningful chiefly where the explicit breakpoint lives.
- **Catalog placement.** The blurb+names index must be resident for the model to know what exists. Two candidate homes exist: the system prompt (requires the library to mutate consumer prose) or the *description of the search/load meta-tool itself* (rides the tools block, cache-eligible, owned by the tool surface). AgentKit uses the meta-tool description — the non-invasive spot for a library, since `Request.System` stays verbatim consumer prose.

### 13.3 What AgentKit built

One synthesized `load_tools` meta-tool whose description carries the generated catalog (per-group blurb + bare tool names). It takes **exact tool or group names, batched** — deliberately *not* the fuzzy search Claude Code's `ToolSearch` offers, because the full catalog is always visible, so the model never needs to *find* a tool, only to *load* it. A group name loads every tool in that group; a name matching neither yields a per-name error line while the other names in the same call still load. The result text carries each loaded tool's description and input schema, so the model can compose a correct call before the next round-trip's tools array even arrives.

## 14. OpenRouter — the aggregator wire, cost reporting, and routing

Sources: OpenRouter's docs/use-cases/usage-accounting, docs/features/provider-routing, and docs/use-cases/oauth-pkce.

### 14.1 The wire

- One API for every model it serves: **OpenAI Chat-Completions-compatible**, base `https://openrouter.ai/api/v1`, bearer auth (`Authorization: Bearer <key>`). SSE streaming as standard chat-completions. This is the same protocol family AgentKit's `internal/openaicompat` already speaks.
- Model ids are **vendor-namespaced free-form slugs** (`anthropic/claude-fable-5`, `z-ai/glm-5.2`); new models are typically served day-one. There is no meaningful closed model set — hundreds of ids, changing weekly. ⚠ The slug's model half does not always equal the vendor's own model id: OpenRouter spells three tracked Anthropic models with **dots** where Anthropic uses hyphens — `anthropic/claude-opus-4.8`, `anthropic/claude-sonnet-4.6`, `anthropic/claude-haiku-4.5` (verified against the live model list 2026-07-29; the hyphenated joins are not served). Every other tracked route follows the `vendor/model` join exactly.
- **Model-suffix routing shortcuts** ride the model string itself: `:nitro` (= sort by throughput), `:floor` (= sort by price). A free-form model string passthrough gets these for free.

### 14.2 Cost reporting (the fact the cost seam leans on)

- **Every response carries usage + cost automatically** — no request parameter needed; `usage: {include: true}` and `stream_options: {include_usage: true}` are no-ops.
- Response `usage` carries: `prompt_tokens`, `completion_tokens`, `total_tokens`, plus detail breakdowns (`cached_tokens`, `reasoning_tokens`, `cache_write_tokens`, …) and **`cost`** — the total charged, denominated in OpenRouter credits (USD-pegged) — plus `cost_details.upstream_inference_cost`.
- **Streaming**: the usage/cost object arrives in the **final SSE frame** — the same place `openaicompat` already reads usage from.
- **BYOK**: with a vendor key attached to the OpenRouter account, `cost` holds OpenRouter's fee and `cost_details.upstream_inference_cost` holds the vendor-side spend (populated only for BYOK; otherwise 0/null). Effective total = `cost + upstream_inference_cost` — correct on both payment paths with zero client configuration.
- Pricing structure: no per-token markup on the credits path (5.5% credit-purchase fee); BYOK = direct vendor rates, ~1M requests/month free of routing fee, 5% beyond. At AgentKit-consumer volumes (thousands of calls/month) BYOK overhead ≈ $0. **BYOK vs credits requires nothing in client code** — it is account configuration.

### 14.3 Provider routing controls

A top-level request-body object `provider: {…}` steers routing: `order` (provider sequence), `allow_fallbacks` (default true), `only`/`ignore` (provider allow/deny lists), `sort` (`"price"`/`"throughput"`/`"latency"`), `max_price` (per-Mtok spend caps), `quantizations`, `require_parameters`, `data_collection: "deny"`, `zdr` (zero-data-retention only). These evolve frequently — a reason to pass them as an opaque body fragment rather than a typed core surface.

### 14.4 Reasoning parameter

OpenRouter normalizes reasoning across models via a top-level `reasoning` request object: `{"effort": "high"}` (effort-enum models), `{"max_tokens": N}` (budget models), `{"enabled": false}` (disable). This is a *different encoding* than Z.ai's `thinking`/`reasoning_effort` fields on the same chat-completions wire, so the openrouter adapter has its own reasoning lowering: `{"reasoning":{"effort":…}}` for a level, `{"reasoning":{"max_tokens":N}}` for a budget, `{"reasoning":{"enabled":false}}` to disable, and nothing at all when unset. What a given route validly accepts is **documented per model**: each entry in `GET /api/v1/models` may carry a `reasoning` descriptor — `supported_efforts` (descending), `default_effort` (a UI pre-select hint, *not* the silence behavior), `default_enabled` (the on/off state when nothing is sent), and `mandatory` (the model rejects being disabled). §14.7 records the audited descriptors and confirmations for every tracked route. ⚠ The gateway's *parser* is far more permissive than any model's documented set — it returns 200 for undocumented effort values, out-of-range budgets, and toggles it then silently ignores — so acceptance without error establishes nothing about validity.

### 14.6 Vendors reachable only through OpenRouter

xAI (Grok), DeepSeek, and Moonshot (Kimi) have **no native AgentKit adapter**, so the aggregator is their only route. Their catalog entries therefore differ in kind from GLM's: the default provider **is** `openrouter` and the wire id is the vendor-namespaced slug, whereas GLM defaults to `zai` and lists OpenRouter as an alternate route. A cataloged entry that named a bare vendor id with no route would resolve to a slug OpenRouter does not serve, so the route entry is load-bearing rather than decorative.

Because §14.2's reported cost overrides table rates on this path, the rates recorded for these vendors in §6.5 are advisory display data. The slugs themselves are the part that must be right, and a golden test cannot falsify them — only a live call can.

### 14.5 Auth acquisition

API keys are created in the dashboard, or minted programmatically via **OAuth PKCE** (`https://openrouter.ai/auth` → callback `code` → POST `https://openrouter.ai/api/v1/auth/keys` → an ordinary API key). The PKCE flow's end product is a static key — construction-time credential handling is unaffected by how the key was obtained. Usage bills to the authenticating user's account.

### 14.7 OpenRouter route audit — reasoning descriptors and confirmations (2026-07-29)

A full audit of the 22 OpenRouter routes that are secondaries of natively-served tracked models: the documented per-model `reasoning` descriptor from `GET /api/v1/models`, plus live confirmation probes (13 requests per route — silence ×2, seven effort values, two budgets, both toggles; ~280 requests total). The rates gathered in the same pass live in §6.5. This subsection ends in the **resolved offering table** the catalog transcribes; the resolution rule it applies is D26's ("documented and confirmed; ambiguity resolves toward the native provider; never from acceptance alone").

**Documented descriptors (verbatim from the models endpoint):**

| Route | `supported_efforts` | `default_effort` | `default_enabled` | `mandatory` |
|---|---|---|---|---|
| anthropic/claude-opus-4.8 | max,xhigh,high,medium,low | high | false | false |
| anthropic/claude-sonnet-4.6 | max,high,medium,low | medium | — | false |
| anthropic/claude-haiku-4.5 | — | — | — | false |
| anthropic/claude-fable-5 | max,xhigh,high,medium,low | high | — | true |
| anthropic/claude-sonnet-5 | max,xhigh,high,medium,low | high | true | false |
| google/gemini-2.5-flash | — | — | — | false |
| google/gemini-2.5-pro | — | — | — | true |
| google/gemini-3.5-flash | high,medium,low,minimal | medium | true | true |
| google/gemini-3.1-flash-lite | high,medium,low,minimal | minimal | true | false |
| google/gemini-3.1-pro-preview | high,medium,low | medium | — | true |
| openai/gpt-5.5-pro | xhigh,high,medium | medium | — | true |
| openai/gpt-5.5 | xhigh,high,medium,low,none | medium | true | false |
| openai/gpt-5.4 (+mini,nano) | xhigh,high,medium,low,none | medium | false | false |
| openai/gpt-5.6-sol (+terra,luna) | max,xhigh,high,medium,low,none | medium | true | false |
| z-ai/glm-5.2 | xhigh,high | high | true | false |
| z-ai/glm-5.1 | — | — | true | false |
| z-ai/glm-4.7 | — | — | true | false |
| z-ai/glm-4.6 | — | — | — | false |

**Confirmations (live probes).** All 22 slugs resolve and serve. The deterministic signals, which are the only probe results treated as knowledge:
- **Every documented `mandatory: true` matched an observed hard rejection** of both off-forms (`effort: "none"`, `enabled: false`) with the identical error *"Reasoning is mandatory for this endpoint and cannot be disabled."* — fable-5, gpt-5.5-pro, gemini-2.5-pro, gemini-3.5-flash, gemini-3.1-pro-preview. No `mandatory: false` route ever rejected an off-form.
- **claude-haiku-4.5 enforces a real budget bound**, self-documented in its rejection: *"reasoning.max_tokens must be between 1024 and 63999 for this model."* The only route where a bound surfaced.
- **Off-forms cleanly zero reasoning tokens** on every non-mandatory route that reasons by default (all four glm routes; gemini-2.5-flash's budget probes versus its silent default).
- Everything else — 200s for undocumented effort values, huge budgets on effort-only routes, toggles with no observable effect — is the permissive gateway parser (§14.4) and establishes nothing. Reasoning-token counts on the trivial probe prompt are not monotonic in effort anywhere and are weak evidence either way; where they contradict a documented default (e.g. sonnet-5's `default_enabled: true` versus zero observed tokens), the ambiguity resolves per D26's rule.

**Resolved offering table** — the reasoning-spec cells the catalog ships for each OpenRouter offering, with the resolution basis per row. `default_effort` is a UI hint by OpenRouter's own definition, so silence *values* come from the native route's established default wherever OpenRouter's vocabulary matches native's (pass-through logic); where OpenRouter documents its own divergent vocabulary (glm-5.2), its documented cells win.

| Model @ openrouter | Kind / vocabulary | CanEnable | CanDisable | Default | basis |
|---|---|---|---|---|---|
| claude-opus-4-8 | Enum effort: low,medium,high,xhigh,max | f | t | Off | descriptor `default_enabled:false`, silence confirmed off ×2 |
| claude-sonnet-4-6 | Enum effort: low,medium,high,max | f | t | Fixed(high) | descriptor silent on default → native (fixed high) |
| claude-haiku-4-5 | Range budget: 1024–63999, no sentinels | f | t | Off | bound from live rejection; default off = native, confirmed |
| claude-fable-5 | Enum effort: low,medium,high,xhigh,max | f | **f** | Fixed(medium) | `mandatory` documented + rejection-confirmed; silence value → native (medium) |
| claude-sonnet-5 | Enum effort: low,medium,high,xhigh,max | f | t | Fixed(medium) | `default_enabled:true` agrees with native on-ness; value → native (medium) |
| gemini-2.5-flash | Range budget: 0–24576, sentinel 0=off | f | t | Dynamic | descriptor documents no efforts; budget per prose docs + native; bounds/default → native |
| gemini-2.5-pro | Range budget: 128–32768, no sentinels | f | **f** | Dynamic | `mandatory` rejection-confirmed; bounds/default → native |
| gemini-3.5-flash | Enum thinking level: minimal,low,medium,high | f | **f** | Fixed(medium) | descriptor complete; mandatory rejection-confirmed; native agrees |
| gemini-3.1-flash-lite | Enum thinking level: minimal,low,medium,high | f | t | Fixed(medium) | descriptor `mandatory:false`; silence value → native (medium; `default_effort:minimal` is a pre-select hint) |
| gemini-3.1-pro-preview | Enum thinking level: low,medium,high | f | **f** | Fixed(high) | mandatory rejection-confirmed; silence value → native (high) |
| gpt-5.5-pro | Enum effort: medium,high,xhigh | f | **f** | Fixed(high) | mandatory rejection-confirmed; descriptor adds `medium` to native's set; silence value → native (high) |
| gpt-5.5 | Enum effort: none,low,medium,high,xhigh | f | t | Fixed(medium) | descriptor = native exactly |
| gpt-5.4 | Enum effort: none,low,medium,high,xhigh | f | t | Fixed(none) | `default_enabled:false` = native fixed none |
| gpt-5.4-mini | Enum effort: none,low,medium,high,xhigh | f | t | Fixed(none) | as gpt-5.4 |
| gpt-5.4-nano | Enum effort: none,low,medium,high,xhigh | f | t | Fixed(none) | as gpt-5.4 |
| gpt-5.6-sol | Enum effort: none,low,medium,high,xhigh | f | t | Fixed(medium) | descriptor's extra `max` is unconfirmed and absent natively → dropped per ambiguity rule |
| gpt-5.6-terra | Enum effort: none,low,medium,high,xhigh | f | t | Fixed(medium) | as gpt-5.6-sol |
| gpt-5.6-luna | Enum effort: none,low,medium,high,xhigh | f | t | Fixed(medium) | as gpt-5.6-sol |
| glm-5.2 | Enum effort: high,xhigh | f | t | Fixed(high) | descriptor documents its **own** vocabulary (`high,xhigh` ≠ native `high,max`), proving no pass-through — its cells win, including `default_effort:high` with `default_enabled:true` |
| glm-5.1 | Toggle thinking | t | t | Fixed(enabled) | descriptor `default_enabled:true`; native toggle agrees |
| glm-4.7 | Toggle thinking | t | t | Fixed(enabled) | as glm-5.1 |
| glm-4.6 | Toggle thinking | t | t | Fixed(enabled) | descriptor silent on default → native (enabled); silence-reasons probes corroborate |

`CanEnable` is `t` only on Toggle rows by the standing D26 convention (on an enum or range, on-ness is expressed by naming a level or budget). `Term` strings follow the existing catalog vocabulary: `"effort"` for enum rows, `"thinking budget"` for range rows, `"thinking"` for toggle rows.

## 15. OpenAI ChatGPT-subscription auth — the Codex-backend path

Verified against a real Codex CLI `~/.codex/auth.json` (structure only): OpenAI permits and endorses using a ChatGPT Plus/Pro subscription from third-party harnesses (OpenCode et al. do this in production). Not contractually guaranteed — Anthropic's and Google's equivalents are restricted — a risk noted, not a blocker.

### 15.1 The credential store

The credential file AgentKit consumes is the **raw RFC 6749 token-endpoint response, verbatim** — exactly the bytes a generic OAuth login tool (the standalone `oauth-login` CLI) prints to stdout, saved to a file. Observed shape of a real `auth.openai.com/oauth/token` response:

- `access_token` — short-lived JWT bearer, carries `exp`
- `refresh_token` — opaque, long-lived; rotates on refresh
- `id_token` — OIDC JWT
- plus standard fields AgentKit ignores: `token_type`, `expires_in`, `scope`

**There is no top-level `account_id` in the token response.** The ChatGPT account id lives *inside* the JWTs, as `chatgpt_account_id` under the `https://api.openai.com/auth` claim — present in the `id_token` and in login-time `access_token`s. Empirically, corroborated by third-party harness reports, access tokens returned by *refresh* can lack the claim, so the id must be extracted at load from `id_token` first, `access_token` as fallback, and any file rewrite must preserve the `id_token` so the file stays self-sufficient.

The official codex CLI's `~/.codex/auth.json` is a **different wrapper shape** (`tokens.{access_token,refresh_token,id_token,account_id}`, `last_refresh`, `auth_mode`) with the account id duplicated as a plain field. AgentKit no longer reads that shape; its earlier expectation of a top-level `account_id` in the token response was the confirmed cause of every real login failing at the exchange step. Sessions go stale after ~8 days without a successful refresh.

### 15.2 The wire

- Endpoint: `https://chatgpt.com/backend-api/codex/responses` — the **Responses API shape** (which AgentKit's `openai` adapter already speaks: `store:false`, `include:["reasoning.encrypted_content"]`, System→`instructions` are already fixed adapter behavior). OAuth tokens are **rejected** by `api.openai.com/v1/*`.
- Required headers: `Authorization: Bearer <access_token>`, `chatgpt-account-id: <account_id>`, `originator: codex_cli_rs`, `OpenAI-Beta: responses=experimental`.
- Body constraints enforced by the backend: `store:false`, mandatory `instructions`, `include:["reasoning.encrypted_content"]` — all already the adapter's fixed behavior.
- Model set is gated by the backend (gpt-5.x-codex family); an unsupported model fails loudly with a backend error — consistent with free-flow model strings.

### 15.3 Login (external) & refresh (AgentKit's job)

- **Login is outside AgentKit.** The one-time OAuth 2.0 PKCE login against `auth.openai.com` (public client id `app_EMoamEEZ73f0CkXaXp7hrann`, registered redirect `http://localhost:1455/auth/callback`, scope `openid profile email offline_access`) is performed by the standalone generic `oauth-login` CLI, which serves the loopback callback itself and prints the token response verbatim to stdout; the consumer saves that output as the credential file AgentKit loads. AgentKit ships no login flow.
- Refresh: `POST auth.openai.com/oauth/token` (`grant_type=refresh_token`, same client id); rotated tokens are written back atomically, in the same raw token-response shape, carrying forward the prior `refresh_token` and `id_token` when the response omits them (refresh responses can omit both the `id_token` and the account claim — §15.1). Refresh proactively before `exp` — a 5-minute skew ahead of expiry. There is deliberately **no** reactive refresh-on-401 path: a 401 from the backend maps straight to `ErrAuthentication` and the turn fails loudly rather than silently retrying with a new token.
- **Refresh-token lineages are per-login.** Copying one credential file to two machines shares a lineage — rotation on one invalidates the other. Two independent logins coexist fine (like two signed-in devices), sharing only the account-level rate-limit pool. Consequence for testing: live tests against a shared-lineage file must read/use only; refresh-and-rewrite is exercised only against fakes or a file whose lineage is expendable.

### 15.4 Cost

The backend reports **no cost** and the subscription is flat-rate; the only computable per-call figure is API-rate-equivalent pricing. Decision: subscription-mode turns are costed exactly like API-key turns (catalog/consumer-supplied rates), documented as **notional API-rate-equivalent, not actual spend**. No flag distinguishes it — documentation only.

## 16. Local coding tools — prior art for the `toolkit` subpackage

Three implementations of the same "coding agent" tool family already exist and inform the toolkit's semantics; the toolkit exists so consumers stop re-writing this set.

### 16.1 The two in-family implementations

- **agent-repl** (`github.com/ikigenba/agentrepl`, `internal/tools`, ~95 lines): the minimal four — `bash`, `read`, `write`, `edit` (lowercase names) — cwd-relative, no path confinement, `edit` replaces **all** occurrences unconditionally, `bash -lc`, nonzero exit rendered as a trailing `[exit status N]` on a **normal** result (nil error).
- **prompts service** (`ikigenba/main/prompts`, `internal/tools`, ~694 lines): the full six with **TitleCase names** (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `Grep`), rooted at a sandbox dir with symlink-aware escape rejection (`confinePath` resolves the existing prefix of a path via `EvalSymlinks` before the containment check), `Read` with 1-based `offset`/`limit` line slicing, `Edit` with `old_string`/`new_string`/`replace_all` (first-occurrence when unset — no uniqueness guard), `Glob` via `filepath.Glob` returning a sorted JSON array of base-relative slash paths, `Grep` walking with `regexp` returning sorted `file:line:text` JSON, `bash -c` with cwd = root and a **Go error** on nonzero exit. No output caps, no `.git`/binary skipping, no timeout.

### 16.2 Field conventions models are trained on (Claude Code)

- Tool names are TitleCase: `Bash`, `Read`, `Write`, `Edit`, `Glob`, `Grep`; input fields `file_path`, `old_string`, `new_string`, `replace_all`, `offset`, `limit`, `command`, `timeout`.
- Bash output is truncated at **30,000 characters** by default (`BASH_MAX_OUTPUT_LENGTH`); `timeout` is an optional per-call input in **milliseconds** with a 120,000 ms default.
- `Edit` **errors when `old_string` is not unique** unless `replace_all` — the guard that prevents silent wrong-location edits.
- `Glob` supports `**` recursive patterns; models use `**/*.go` reflexively.

### 16.3 Stdlib constraints

- `filepath.Glob`/`fs.Glob` implement plain `path.Match` semantics — **no `**` support**; `**` degrades to a single `*` segment silently. Recursive matching therefore needs a hand-rolled `WalkDir` + per-segment match (the no-new-dependencies rule excludes `doublestar`).
- The classic git binary heuristic — a NUL byte in the first 8 KB — is trivially implementable with stdlib and matches user expectations for "grep skips binaries".

---

## 17. Document text extraction — OpenRouter's `file-parser`, measured

Everything in this section was measured directly against the live OpenRouter API in July 2026, using real scanned documents (bank statements and scanned books, 1 to 385 pages). Where a figure is quoted it was observed, not read off a vendor page.

**The headline:** OpenRouter's `file-parser` plugin with `engine: mistral-ocr` extracts scanned PDFs into structured Markdown at roughly **$0.002 per page**, and the parsed text is returned in `choices[0].message.annotations` — *beside* the chat model's reply, produced before the model sees it. The model contributes nothing to it.

### 17.1 The request that works

```
POST https://openrouter.ai/api/v1/chat/completions
{ "model": "<any model that accepts a file block>",
  "messages": [{"role":"user","content":[
      {"type":"file","file":{"filename":"x.pdf","file_data":"data:application/pdf;base64,…"}}]}],
  "max_tokens": 1,
  "plugins": [{"id":"file-parser","pdf":{"engine":"mistral-ocr"}}] }
```

- **No text content part is required.** A request carrying *only* the file block returned the complete transcript. There is no prompt to engineer; instructions are irrelevant because the extraction happens before the model.
- **`max_tokens: 1` is sufficient** and does not truncate the extraction: a 1-token budget still returned the full 2,403-byte transcript.
- Engines: `mistral-ocr`, `native` (the model's own vision), and `pdf-text` (deprecated, text-layer only, useless for scans). With no engine specified OpenRouter tries native first and falls back.
- OpenRouter bills the OCR against its own Mistral relationship, so the per-page fee lands on the OpenRouter account.

### 17.2 The chat model contributes nothing (measured)

The `model` field is required only because `file-parser` is a plugin on `/chat/completions`. It is a **billing knob with no effect on output**:

| model | transcript bytes | sha256 (first 8) |
|---|---|---|
| `google/gemini-3.5-flash` | 2403 | `a7a84dc0` |
| `openai/gpt-5-nano` | 2403 | `a7a84dc0` |
| `google/gemini-2.5-flash-lite` | 2403 | `11ff77f3` |

Two unrelated model families produced **byte-identical** transcripts. The single difference (`Continued\n8/2498/1` vs `Continued 8/2498/1`) also appeared between two runs on the *same* model, so it is **mistral-ocr's own run-to-run nondeterminism** at the line-break level, not a model effect. Consequence for testing: live assertions must use substrings, never hashes or exact equality.

**The model tax is real and scales with the document.** OpenRouter injects the full extraction into the chat model's prompt, so input tokens are billed over the whole document on every call — 1,142 prompt tokens for 2 pages *with an empty prompt*, 158,051 for 114 pages. It cannot be avoided on this path and cannot be routed to subscription auth, because the plugin exists only on the API-key path.

**`openai/gpt-5-nano` returned no `usage` block at all** (no `prompt_tokens`, no `cost`), which would silently degrade cost accounting (§ dollar-cost). A default model should be one that reports usage.

### 17.3 The response contract

Parsed text lives at `choices[0].message.annotations[0].file.content`. Verified invariants from 1 to 114 pages:

- **`annotations` is always exactly one entry**, `type: "file"`, `name` = the uploaded filename. Everything scales inside `file.content`; the array never grows.
- **`file.content` is a mixed list**: a `<file name="…">` text sentinel, one text item **per physical page in order**, `image_url` items interleaved after the page they came from, and a `</file>` text sentinel.
- **Text-item count is exactly *pages + 2*** at every size tested (2→4, 8→10, 38→40, 88→90, 114→116).
- **Blank pages keep their slot** — a near-empty page returned its own 1-character item rather than being omitted, so page ordinal stays aligned with physical page.
- **Printed page numbers do not match physical ones** (physical page 4 printed "Page 3 of 6"); physical ordering is the reliable index.
- **`image_url` items are capped, apparently at 8 per document**, regardless of length: three documents of 38, 88, and 114 pages (the latter two heavily illustrated) each returned exactly 8. They are a truncated sample, **not** a faithful image extraction, and must not be presented as complete.
- **Reading `content` naively is a context bomb.** On a single-page statement, one `image_url` item held **25,409 characters** of base64 — 94% of that response's payload.

### 17.4 Limits and failure shapes

- **Errors arrive as HTTP 200 with an `error` body.** A 385-page / 24.1MB PDF returned status **200** whose entire body was `{"error":{"message":"Downloaded image content cannot exceed 30MB","code":413}}`. Code that branches on the status line and reaches for `choices[0]` fails on what looks like success. **The body must be inspected first.**
- **The 30MB ceiling is on extracted image content, not the file.** A 23.3MB / 114-page PDF (base64 payload **31.1MB**, itself over 30MB) succeeded, while a 24.1MB / 385-page scanned-art book failed. Neither raw size nor encoded size explains it; the message's wording does. **A pre-flight size guard is therefore unsound** — it would reject working documents and admit failing ones.
- Quiet-empty is the other failure shape: HTTP 200, a normal-looking response, and no `annotations` at all (this is what images do, §17.5).

### 17.5 The plugin is a no-op for images (measured)

| sent | plugin | annotations |
|---|---|---|
| PNG as `file` block | `mistral-ocr` | `[]` |
| JPEG as `file` block | `mistral-ocr` | `[]` |
| PNG as `file` block | none (control) | `[]` |
| PNG as `file` block, named `page1.pdf` | `mistral-ocr` | `[]` |

All four returned identical prompt-token counts, meaning the image was tokenized by the model's **native vision** in every case. The plugin does not touch images and the filename does not route it.

**Wrapping works.** A raster image embedded in a minimal one-page PDF *does* go through the OCR path and returns full `annotations`. Confirmed for both a lossless-Flate PNG embed and a byte-for-byte `DCTDecode` JPEG embed; the two produced **byte-identical OCR text** apart from one line break, at a 4× payload-size difference, so the input encoding does not affect fidelity. The wrapper needs only `image/png`, `image/jpeg`, and `compress/zlib` — no `gs`, no ImageMagick, no new module (and `convert`/`magick`/`gs` are absent from the target sandbox anyway).

### 17.6 Alternatives evaluated and not chosen

- **Mistral OCR API direct** (`mistral-ocr-latest`, and the newer **OCR 4**). One endpoint handles **both** documents and images, returns parsed text with no model in the loop, and OCR 4 additionally returns **bounding boxes, typed blocks (titles, tables, signatures), and per-word confidence scores** — exactly what would let a consumer detect column flattening mechanically. Listed at $4/1k pages ($2 via batch). **Not chosen:** it needs a second vendor account, and API-key access is routed through OpenRouter. The lost geometry is a real, named cost of that constraint: `file-parser` returns Markdown only.
- **Self-hosting Mistral OCR.** Supported by the vendor for data-privacy and sovereignty requirements. This is the escape hatch if sending financial documents to a third party ever becomes unacceptable — the exit is the *same engine run locally*, not a different vendor. Other self-hostable options: olmOCR, dots.ocr, Surya, PaddleOCR-VL, Tesseract.
- **OpenAI.** Has **no dedicated OCR endpoint** (re-verified July 2026). Every OpenAI-plus-OCR pattern is either "prompt GPT vision to transcribe" or "run Tesseract first, then clean up with GPT". So there is currently nothing to reach on the subscription-auth path even in principle.
- **Native vision on any chat model.** Works, but returns the model's prose rather than a parsed transcript, carrying paraphrase risk on financial figures and paying reasoning tokens to re-emit text.

### 17.7 Fidelity characteristics (measured, not defects to fix)

- **Tables come back as Markdown pipe tables**, and transaction tables extracted cleanly: `| Mar 03 | Mar 04 | EXAMPLE MERCHANTANYTOWNXX | $12.34 |` (figures here and below are synthetic; real statements were used for the measurements but are not reproduced).
- **Values can land in the wrong column.** Reproduced in three separate runs on the same statement: `| Average Balance (Ledger) | 1,234.56+ |   |` puts the figure under *Number* instead of *Amount*. The engine is faithful to where ink sits, not to what a column means. Mitigation is to validate figures against the document's own totals, never column position.
- **Whitespace inside a cell is unreliable** — `EXAMPLE STOREANYTOWNXX` runs merchant, city, and state together. Never split a cell on spaces.
- Logos degrade to their word mark; photographs become `![img-0.jpeg]` references; advertising copy is extracted in full as ordinary headings, so a classifier must expect to ignore it.
- Identity fields (bank name, account type, last four, statement date) all appear in the top third of page 1, so classification rarely needs more than the first page.

### 17.8 Cost and scale reference (measured)

"Transcript chars" below is the **sum of the `type == "text"` items only** — what actually reaches a transcript. Do not confuse it with the JSON-encoded size of `file.content`, which is dominated by base64 `image_url` payloads and runs far larger (for the 114-page book, 920k encoded versus 560k of actual text).

| pages | transcript chars | prompt tokens | cost | wall time |
|---|---|---|---|---|
| 1 (wrapped image) | 1,508 | 1,682 | $0.0048 | 4.2s |
| 2 | 2,399 | 1,991 | $0.0072 | 4.1s |
| 8 (dense tables) | 22,957 | 11,390 | $0.0333 | 5.9s |
| 38 | 77,902 | 23,644 | $0.0784 | 7.4s |
| 88 | 397,833 | 118,426 | $0.1878 | 16.0s |
| 114 | 559,805 | 158,051 | $0.2438 | 20.2s |

Steady at roughly **$0.002 per page**. Note the scale gap between typical and tail: a bank statement is 2–8 pages and ~2–23k characters, while a scanned book is ~560k characters — roughly 19× any sane single tool-result cap. That gap is what forces extraction to disk rather than an inline return, and makes caching load-bearing rather than a nicety.

## 18. Tool-schema dialects — measured, per provider (2026-07-30)

Every row below was **measured against the live API**, not read from documentation. This matters:
documentation was wrong in all three cases checked. Google's discovery document omits `allOf`/
`oneOf`/`not`, which the API accepts. OpenAI's error text says `additionalProperties` "must be
false" while schema-valued forms are accepted. Anthropic's published strict-mode keyword list
implies `minLength`/`maxLength` are unsupported; both are accepted at any value. Treat this
section as ground truth and the vendor prose as a lagging indicator.

Probe substrates: Gemini `gemini-2.5-flash-lite` on `v1beta:generateContent` (~70 requests);
OpenAI `gpt-4o-mini-2024-07-18` on Chat Completions (~45 schema fixtures, each run both
model-chooses and forced `tool_choice`); Anthropic `claude-haiku-4-5-20251001` on `/v1/messages`
with `anthropic-version: 2023-06-01`.

### 18.1 Google Gemini — `functionDeclarations[].parameters`

`parameters` is the protobuf message `google.ai.generativelanguage.v1beta.Schema`, parsed with
strict protobuf-JSON. **Any key that is not a proto field fails the whole request with 400
`INVALID_ARGUMENT` — "Unknown name … Cannot find field".** There is no lenient mode. This is the
single most important fact about the Gemini dialect: an allowlist is mandatory, and a denylist
breaks the moment a new JSON Schema keyword appears upstream.

Accepted fields (22 documented): `type`, `description`, `title`, `nullable`, `format`, `enum`,
`properties`, `required`, `items`, `minimum`, `maximum`, `minLength`, `maxLength`, `minItems`,
`maxItems`, `minProperties`, `maxProperties`, `pattern`, `anyOf`, `propertyOrdering`, `default`
(accepted and explicitly ignored per its own field doc), `example`. `allOf`, `oneOf` and `not`
are also accepted and are real Schema-typed proto fields, but are **absent from the discovery
document** — their semantics are unverified and they should not be relied on.

Rejected with 400: `additionalProperties`, `$ref`, `$defs`, `definitions`, `$schema`, `$id`,
`$anchor`, `$comment`, `const`, `patternProperties`, `propertyNames`, `if`/`then`/`else`,
`dependentSchemas`, `dependentRequired`, `examples`, `exclusiveMinimum`, `exclusiveMaximum`,
`prefixItems`, `uniqueItems`, `multipleOf`, `contains`, `unevaluatedProperties`,
`unevaluatedItems`, `additionalItems`, `readOnly`, `writeOnly`, `deprecated`, `discriminator`.

Details: `type` takes a single value only — `["string","null"]` is rejected, and nullability is
expressed with `nullable: true`. `enum` is `string[]`; numeric enums have no representation.
Both camelCase and snake_case parse (protobuf-JSON), but spelling is otherwise case-sensitive.

Two facts that bound alternatives considered and not chosen. **`parametersJsonSchema`** is a
mutually-exclusive sibling field taking untyped full JSON Schema; nothing is parse-rejected there,
including invented keys, because unsupported keywords are dropped silently — it trades a loud
failure for quiet semantic drift, which is why it is not used. **Vertex AI's `Schema` is a strict
superset** of the Gemini API's, additionally accepting `additionalProperties`, `ref` and `defs`;
the same schema therefore behaves differently on the two Google surfaces.

### 18.2 OpenAI — `tools[].function.parameters` with `strict: true`

Accepted **and grammar-enforced** (verified with adversarial prompts asking for violating values,
e.g. `maxLength: 3` against a 26-character request returned `"abc"`): `minimum`, `maximum`,
`exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`, `minLength`, `maxLength`, `pattern`,
`minItems`, `maxItems`, `enum`, `const` (only with a sibling `type`), `patternProperties`,
`$ref`/`$defs` including **recursive** self-reference, `anyOf` inside a property, `title`,
`default` (accepted, ignored), `$schema` at the root.

Rejected: `allOf`, `oneOf`, `not`, `if`/`then`/`else`, `const` without `type`, root-level `anyOf`
(rejected even without strict — `parameters` must be `type: "object"` unconditionally), and
**`format: "uri"`**. The `uri` rejection is specific, not a general `format` restriction: `email`,
`uuid`, `date`, `date-time`, `ipv4`, `ipv6`, `duration`, `time`, `hostname` all
pass (each verified live, at top level and on a nested object's string
property; a garbage value like `zzz` is rejected, so acceptance is a positive
allowlist hit).

Two structural rules. **Every key in `properties` must appear in `required`** — there are no
optional properties; optionality is expressed as `"type": ["T","null"]` or
`anyOf: [{"type":"T"},{"type":"null"}]`, both accepted and both genuinely decoding `null`.
**`additionalProperties` may not be omitted or `true`, at any nesting level** — a nested object
missing it is a 400 — but a *schema-valued* `additionalProperties` is accepted despite the error
text, and is enforced (an extra key was permitted with its value coerced to the declared type).

### 18.3 Anthropic — `tools[].input_schema` with `strict: true`

`strict` is a real sibling field of `name`/`description`/`input_schema` on a custom tool
(an invented sibling returns "Extra inputs are not permitted"; `strict: true` returns 200).

**Calibration that qualifies every "accepted" below:** the strict validator is type-keyed, and
only the `object` branch validates its keyword set. `{"type":"string","zzzUnknown":true}` is
accepted; the same unknown key on an object is rejected. So on non-object subschemas, "accepted"
means "did not 400", not "is honored", unless the keyword was demonstrably inspected.

Accepted: `minLength` and `maxLength` **at any value**, `pattern` (genuinely compiled — a
lookahead is rejected as invalid regex, so the engine is RE2-like with no lookahead/lookbehind),
`const`, `enum` (string, integer, or untyped), `allOf`, `anyOf`, `$ref` with `$defs` or draft-07
`definitions`, `format` (allowlisted; see below), `title`, `default`, `minItems` with value 0 or 1.

The `format` allowlist was measured exhaustively: exactly ten values are accepted — `date-time`,
`date`, `time`, `duration`, `email`, `hostname`, `ipv4`, `ipv6`, `uuid`, `uri` — and every other
probed value 400s ("For 'string' type, format '…' is not supported"): `uri-reference`,
`uri-template`, `iri`, `iri-reference`, `idn-email`, `idn-hostname`, `regex`, `json-pointer`,
`relative-json-pointer`, `byte`, `binary`, `int32`, `int64`, `password`, `zzz`. The list applies
uniformly at any depth (nested objects, array items, the string branch of a nullable union). Two
qualifications: the check is keyed on the declared type — on non-string types arbitrary format
values pass (`{"type":"integer","format":"zzz"}` is 200) — and the whole check exists only under
`strict: true` (non-strict accepts `zzz` on a string). Note the OpenAI contrast: `uri` is accepted
here and rejected there, `ipv6` accepted by both, so neither provider's allowlist contains the
other's and only the intersection is portable.

**Nullable type unions are accepted and honored under strict.** `"type": ["string","null"]` and
the `anyOf` spelling (`[{"type":"string"},{"type":"null"}]`) both return 200, as does a bare
`"type": "null"`, at top level and nested. The list is parsed, not ignored: `["string","bogus"]`
is rejected. Object rules survive union membership — an object branch of a union still requires
`additionalProperties: false` and still rejects unknown keys. Forced tool calls never returned a
value outside the declared union (a `["integer","null"]` schema pushed toward a string answer
emitted `null`). Two validator gaps observed alongside: a *known* keyword on the wrong type is
tolerated (`minimum` on a string is 200 despite being rejected on numbers), and unlike OpenAI
there is no required-must-list-all rule (a partial `required` array is 200).

Rejected: `minimum`, `maximum`, `multipleOf`, `exclusiveMinimum` (all on numeric types),
`maxItems`, `uniqueItems`, `minItems` with any value other than 0 or 1 (the error text leaks the
allowlist), `oneOf`, `not`, `patternProperties`, and **recursive `$ref`** ("Circular reference
detected … Self-referencing or mutually-referencing definitions are not supported").

Structural rules, inverted from OpenAI's in two places. **Optional properties ARE allowed** — a
property may be omitted from `required`, and `required` may be absent entirely. And
**`additionalProperties: false` is mandatory on every object including nested ones**, with `true`
*and schema-valued forms* both rejected — the opposite of OpenAI, which accepts a schema value.

### 18.4 Non-strict mode — neither provider validates anything

With `strict: false`, or `strict` absent, **every** form that 400s under strict is accepted by
both Anthropic and OpenAI, and is silently ignored rather than enforced. Verified behaviourally
rather than by status code: under non-strict OpenAI, `pattern: "^[0-9]{3}$"` emitted
`"hello world"` and `minimum: 100` emitted `5`. Anthropic non-strict likewise accepted `minimum`,
`oneOf`, `additionalProperties: true`, recursive `$ref`, and even `format: "zzz"`.

This is the finding that forces strict mode on. Non-strict does not convey a constraint weakly —
it does not convey it at all, while returning 200. A translation layer that treats "the provider
accepted it" as "the provider honors it" would ship silently weakened tool contracts on two of
three providers and never report it.

### 18.5 The aggregators — no stable dialect of their own

**OpenRouter** has no fixed dialect: it documents passing `parameters` through byte-for-byte to
OpenAI-interface upstreams, *transforming* for custom-interface upstreams (Google, Anthropic), and
falling back to rendering tools into a YAML prompt template for models with no native tool
support. The effective dialect is therefore the routed upstream's, and the transform is
undocumented and observed lossy: pydantic-ai ships an OpenRouter-only Gemini schema transformer
because `$defs`/`$ref`/`anyOf` through OpenRouter produced **wrong tool arguments rather than a
400**, while the same schema works against the Google API directly.

**Z.ai** publishes no keyword contract at all — only "a JSON Schema object". Rich draft-07 schemas
with `allOf`/`anyOf` are accepted, then under-honored by the model. It supports `tool_choice: auto`
only, and has no `response_format: json_schema` and no per-function `strict`.

Both fail silently and in opposite directions, so neither can serve as an oracle for whether a
schema was too rich. The practical consequence is that a schema must be narrowed **before** it is
sent, client-side, rather than discovered by probing.
