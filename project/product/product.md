# AgentKit — Product

**Authority: intent.** This document owns *why* AgentKit exists, *for whom*, what is in and out of scope, and the behavior we **promise** the user — stated once, in outcome terms. It does **not** state mechanism, type shapes, exact wire formats, error/category names, retry policies, vector dimensionality numbers, normalization or chunking mechanics, exit codes, or test assertions; those belong to `project/design/design.md`. Where the two could overlap on behavior, this doc states the *promise* (what the consumer observes) and design states the *exact, checkable proof* of that promise. That boundary is load-bearing: it keeps product, design, and plan from overlapping.

## Problem

A developer building an application on top of large language models has to choose a provider — Anthropic, Google, OpenAI — and then write code against that provider's specific API: its message shape, its tool-use protocol, its streaming format, its error responses, its retry expectations. Adopting a second provider, or letting an app switch between them, means a second integration written from scratch and a calling surface that diverges per provider. The provider's idiosyncrasies leak into application code, so the app is coupled to a vendor it would rather treat as a swappable detail.

Beyond writing each tool by hand, the same application increasingly has whole sets of capabilities already exposed through external **MCP (Model Context Protocol) servers** — hosted services, internal tooling — that it would like the agent to use. Wiring those in itself means another bespoke integration: speaking the MCP protocol, discovering the server's tools, and threading them into whatever tool-use shape the chosen provider expects, per provider. The app wants to point at an MCP server and have its tools simply become available to the agent — the same way, across every provider.

The same application also needs to turn **text into embedding vectors** — to power search, retrieval, clustering, or classification — and hits the mirror image of the same problem. Each embedding provider has its own API: its own request shape, its own vector size, its own rules about whether a search query and a stored document should be encoded differently, its own batch-size limits, its own normalization behavior, and its own way of failing when an input is too long. Adopting a provider couples the app to those idiosyncrasies; switching providers means rewriting the integration, even though conceptually all the app wanted was *text in, vector out*.

## Purpose

AgentKit is a Go library that gives an application **one uniform, provider-agnostic surface for the core text operations it needs from an LLM provider**: holding a tool-using, multi-turn text conversation, and turning text into embedding vectors. The single job it does: present the same calling code for these operations across the providers it supports — chat with tools (consumer-defined and MCP-supplied) and message-granular delivery, and embeddings — so the application writes against one input/output surface and the choice of provider and model is **configuration, not a rewrite**.

## Users

Go developers building applications that talk to LLMs. AgentKit's first and most important consumer is the ikigenba family of applications, but it is a general-purpose public library held to a public bar — clean surface, documented, versioned — and "would this be appropriate for any developer to adopt?" is treated as a standing health check on its design. Its users want to write LLM-backed features — chat, agents with tools, and text embeddings for search and retrieval — once and run them against whichever provider and model they configure, without learning each vendor's API.

## Scope

AgentKit covers **two capabilities — a chat surface and an embeddings surface** — sharing one set of foundations (explicit credentials, uniform errors, automatic retry, uniform usage, dollar-cost accounting).

### Chat

- A **multi-turn, text-only chat** with an LLM.
- **Custom tools** defined and registered by the consuming application, which AgentKit invokes and loops on automatically until the model produces a final answer.
- **MCP tool servers.** The consuming application can attach one or more **remote MCP servers**; AgentKit acts as the MCP client — it connects, discovers each server's tools, and feeds them into the *same* automatic tool loop as custom tools, uniformly across every provider. The model and the consumer see them as ordinary tools. The consumer assigns each attached server a name, which prefixes that server's tools so they are uniquely addressable and recognizable. MCP connection details, including any credentials, are supplied explicitly by the consumer.
- **Faithful tool-schema translation.** Whatever input shape a tool declares — a custom tool's or an MCP server's — AgentKit conveys to the selected provider preserving its meaning, even when that provider's native schema support is narrower than the schema as written. AgentKit does not quietly discard the parts a provider cannot express and proceed as if the tool's full contract had been sent; where some part genuinely cannot be carried, it warns clearly (naming the tool and what it could not convey) and still runs the turn. This holds uniformly for custom and MCP tools and across every provider.
- **Message-granular delivery of the exchange.** As a turn progresses, the consumer observes it as an ordered sequence of completed units — each assistant message, each tool-call request, and each tool result — surfaced as it completes. The delivery unit is the completed message, not a sub-message fragment: replies are **not** delivered token-by-token.
- **Anthropic, Google, OpenAI, and Z.ai** as chat providers, each a first-class peer with at least one supported model — a provider reached through API-compatibility is no less first-class, and how it is implemented is not user-visible.
- A consumer-held conversation object that bundles configuration (provider, model, credentials, generation settings) and the running conversation history, threaded explicitly into each call.
- **Reasoning effort — native per model, inspectable, with a safe fallback.** Reasoning/thinking is a generation setting expressed in each model's *own* native terminology and values — the term that model's provider documents (effort, thinking, thinking level, thinking budget, …) and the values that model actually accepts (its discrete levels, or a token budget within its valid range). There is **no** single cross-model vocabulary and **no** cross-model translation: AgentKit does not pretend one model's value means anything on another. A value the selected model natively understands is honored exactly. Anything it does not — an unknown term, an invalid or out-of-range value, or a setting carried over from a previously selected model — is never silently misapplied: AgentKit reports it as a warning and falls back to that model's default reasoning. The native vocabulary is inspectable: for any supported model a consumer can obtain its reasoning term, the values it accepts (or its valid range) and its default — so a consumer can both display and accept exactly what that model supports, without embedding provider knowledge of its own.

### Embeddings

- **Text embeddings.** Turning text into embedding vectors through the same provider-agnostic, configuration-driven surface as chat: a consumer-held **embedder object** bundles provider, model, credentials, and embedding options, and turns one or more input texts into vectors. It accumulates token-usage and dollar-cost across calls the way the conversation object does, and reports both per-call and cumulative figures.
- **Embedding providers: OpenAI and Google only.** Embeddings are supported for the providers verified to offer a first-class embeddings API — OpenAI and Google — which is deliberately a **subset** of the chat provider set. Provider and model selection is configuration, chosen from a **fixed, curated, closed set** of supported embedding models — no raw model-string passthrough — and every supported embedding model has a built-in pricing entry by construction. The exact supported embedding model ids and their per-model capabilities are enumerated once, in the design's embedding-model registry — not duplicated here.
- **Provider/model selection is config-only — including between calls.** Choosing OpenAI vs. Google, and which model, is a configuration choice in the embedder object, changeable between calls. Switching is a change to provider/model/options and nothing else in the consumer's calling code. Two consequences are inherent and promised plainly, not hidden: vectors from different models are **not comparable**, so switching models means re-embedding the corpus; and the query/document role (below) is what lets a switched-in provider reach its full retrieval quality.
- **Uniform batch.** A consumer embeds any number of input texts in one call and receives the vectors in **input order**; AgentKit absorbs each provider's per-call size limits internally so the consumer never manages them. A single text is just a batch of one.
- **Query-vs-document role.** Each embed call may carry an optional role — **query**, **document**, or **unspecified** (the default) — so a provider that encodes search queries and stored documents differently for better retrieval is given the right hint, while a provider that treats them the same safely ignores it.
- **Target dimension, optional.** A consumer may request a target vector dimension; AgentKit honors it when the selected model supports it, and otherwise **fails loudly** rather than silently returning a different size. The default is the model's native dimension. This is **per-model** configuration; AgentKit promises no cross-provider dimension parity — vectors from different models are not comparable regardless of dimension, and matching dimensions across models is a consumer storage-schema choice AgentKit neither knows nor promises.
- **Normalized vectors.** The vectors AgentKit returns are **unit-normalized**, uniformly across providers and dimensions, so similarity math behaves identically regardless of which provider or dimension produced them.
- **Inspectable model capabilities.** For any supported embedding model a consumer can obtain its native dimension, the dimensions it can produce, and its maximum input size — so a consumer can present and validate a choice up front, without embedding provider knowledge of its own.

### Shared foundations (both capabilities)

- A consumer-held state object that bundles configuration (provider, model, credentials, options) and is threaded explicitly into each call.
- A uniform error surface, automatic retrying of transient and rate-limit failures, uniform token-usage reporting, dollar-cost reporting from built-in per-model pricing, and full visibility of every result.

It deliberately does **nothing else.** In particular:

- **No image or audio, input or output.** Chat messages and embedding inputs are text only; vision, multimodal embeddings, and image/audio generation are out.
- **No batch/asynchronous bulk processing or fine-tuning.** AgentKit is about live, interactive requests only.
- **No durable persistence.** State objects live in memory for the life of the object; saving and restoring conversations or embedder state across process restarts is not promised (a consumer may serialize the object itself).
- **No ambient credential sourcing.** AgentKit never reads environment variables, files, or any credential store on its own.
- **No uniformity promise for provider-specific extras.** The uniform core is what's promised; capabilities unique to one provider are not promised to be uniform.
- **MCP brings in tools only.** Resources and prompts that an MCP server may also expose are out of scope (deferred, not rejected); only its tools are surfaced.
- **Remote MCP servers only.** AgentKit reaches MCP servers over a network connection and spawns **no** child processes; local stdio/subprocess MCP servers are out of scope.
- **No interactive OAuth.** The consumer supplies a ready credential for a server; AgentKit does not negotiate or refresh OAuth flows on the consumer's behalf.
- **No MCP provenance promise.** MCP tools appear in the exchange as ordinary tools; AgentKit does not promise a consumer-visible distinction between an MCP tool and a custom tool beyond the name the consumer's server-prefix gives it.
- **Embeddings: no vector storage, indexing, or search.** AgentKit produces vectors; storing, indexing, and querying them is the consumer's concern.
- **Embeddings: no automatic splitting of an over-long single text.** AgentKit chunks a *batch* (a large list of inputs) across a provider's per-call limit, but it does **not** semantically split a single input text that exceeds the model's size limit — that fails loud, and dividing the text into model-sized pieces is the consumer's decision.
- **Embeddings: no cross-model migration.** Switching embedding models means re-embedding; AgentKit does not convert, migrate, or reconcile previously produced vectors.
- **Anthropic and Z.ai are not embedding providers.** Anthropic offers no first-party embeddings API (excluded on that basis); Z.ai could not be confirmed to serve embeddings on the endpoint AgentKit uses, nor priced in dollars, so it is excluded from the embeddings surface (it remains a chat provider). Z.ai embeddings are a possible future direction — deferred, not rejected — should a first-class, dollar-priced endpoint be confirmed.

## Contractual constants

These fixed, promised values the design must use verbatim and never re-declare:

- **Module path:** `github.com/ikigenba/agentkit`
- **Starting version:** `v0.1.0` — the `v0` major signals a public but pre-stable surface that may evolve through research; promotion to `v1.0.0` happens only when stability is promised.
- **Minimum Go version:** Go 1.26.

## What we promise (user-facing behavior)

### Chat

- **One conversation surface across providers.** The consumer constructs a conversation object with a provider, a model, and credentials supplied explicitly at construction, then holds a multi-turn text conversation by passing that object into each call. The calling code is identical regardless of provider; the conversation history accumulates in the object the consumer holds.
- **Provider/model selection is configuration — including mid-conversation.** Choosing Anthropic vs. Google vs. OpenAI vs. Z.ai, and which model, is a configuration choice in the conversation object. That choice can be changed between turns: the accumulated conversation history carries over, and the next turn runs against the newly selected provider and model.
- **Message-granular delivery.** The exchange is delivered as an ordered stream of completed units: each assistant message, each tool-call request, and each tool result is surfaced as soon as it completes, so the intermediate steps of a tool-using run are observable as they happen. Delivery is at message granularity — the consumer receives complete messages, not token-by-token fragments. A turn that drives several tool calls therefore surfaces a sequence of completed messages and tool events, in order, rather than a single opaque final result. Each completed assistant message carries its full content — visible answer text and any reasoning summary — so nothing is withheld by the move away from sub-message delivery.
- **Custom tools, driven automatically.** The consumer defines a tool — a name, a description, the shape of its input, and the code that runs it — and registers it. When the model asks to use a tool, AgentKit calls the consumer's code, feeds the result back to the model, and repeats until the model produces a final answer. The consumer gets the finished result without managing the back-and-forth themselves.
- **MCP tools, attached by configuration.** The consumer attaches a remote MCP server by supplying — explicitly — its connection details, a name, and any credentials it needs. AgentKit connects, discovers the server's tools, and makes them available to the model through the same automatic loop as custom tools; the server's name prefixes its tools so they stay uniquely addressable. Attached servers are part of the conversation object and can be attached or detached **between turns**, mirroring provider/model selection — the tools available to the model reflect the servers currently attached. If a server is unreachable when attached, or a tool's transport fails mid-call, the consumer sees a uniform error; a tool that simply returns an error result is fed back to the model so the conversation continues. A tool whose name would collide with an existing tool surfaces as a uniform error rather than silently shadowing it.
- **Tool schemas are translated faithfully — never silently weakened.** A tool's declared input shape is carried to the provider with its meaning intact, even when the provider's schema dialect is narrower than what the consumer wrote. AgentKit never quietly drops part of a schema and proceeds as if the tool's full contract were conveyed: when some construct cannot be represented for the selected provider, the consumer gets a clear warning that names the tool and the construct left unconveyed, and the turn still runs. The consumer can always tell, after a turn, when a tool's schema was not conveyed in full — uniformly for custom and MCP tools, on every provider.
- **Full transparency of the exchange.** Every message reaches the consumer: the user's messages, the model's turns, the model's tool-call requests, and the tool results — including those from MCP-provided tools. Nothing is filtered out; the consumer can observe the complete trace, not only the final text. A model's visible answer and its tool-call requests are always surfaced as such, regardless of any reasoning or thinking metadata a provider attaches to them: provider-specific reasoning artifacts never suppress a turn's visible text, never silently turn a real answer into a thinking summary, and never swallow a tool call so that it goes unexecuted.
- **Reasoning effort — native per model, with a safe, visible fallback.** A consumer sets reasoning in the selected model's own native term and a value it accepts — the term the provider documents, and the model's discrete levels or a token budget within its valid range. AgentKit imposes **no** single cross-model vocabulary and does **not** translate a value from one model onto another. A value the model natively understands is honored exactly. Anything it does not — an unknown term, an invalid or out-of-range value, or a value left over from a previously selected model after a mid-conversation switch — is reported as a warning and the model's default reasoning is applied instead; it is never silently misapplied and never breaks the turn. For any supported model a consumer can ask AgentKit for that model's reasoning term, the values it accepts (or its valid range) and its default, so the consumer can present *and* accept exactly what the model supports — and can always tell after a turn when its reasoning input was not honored — without embedding any provider-specific knowledge of its own.

### Embeddings

- **One embeddings surface across providers.** The consumer constructs an embedder object with a provider, a model, and credentials supplied explicitly at construction, then turns text into vectors by passing input text into each call. The calling code is identical whether the provider is OpenAI or Google.
- **Provider/model selection is configuration — config-only swap.** Choosing OpenAI vs. Google, and which model, is a configuration choice in the embedder object, changeable between calls; only the provider/model/options change, not the calling code. The promise carries two stated caveats, plainly visible rather than hidden: vectors from different models are not comparable, so a switch means re-embedding the corpus; and tagging inputs as query vs. document is what lets a switched-in provider reach its full retrieval quality.
- **Batch in, ordered vectors out.** The consumer hands a list of texts (of any size) to one call and gets back one vector per input, in the same order. AgentKit handles each provider's per-call size limits internally, so a batch sized for one provider does not break when the consumer switches to another.
- **Query/document role, honored where it matters.** The consumer may tag each embed call as a query, a document, or leave it unspecified. A provider that distinguishes them uses the hint to improve retrieval; a provider that does not safely ignores it. The same call works on every provider.
- **A requested dimension is honored or refused — never silently changed.** When the consumer requests a target vector dimension, AgentKit returns vectors of exactly that size if the model supports it, and otherwise fails with a clear error. With no request, vectors come back at the model's native dimension. AgentKit makes no cross-provider parity promise — the requested size is about the chosen model only.
- **Vectors come back normalized.** Every vector AgentKit returns is unit-length, uniformly across providers and dimensions, so the consumer can compare vectors with a plain dot product and get consistent results regardless of which provider or dimension produced them.
- **Over-long input fails loud.** An input that exceeds the model's size limit produces a clear error; AgentKit never silently truncates an input and returns a vector as if the whole text had been embedded. Dividing an over-long text into pieces is the consumer's deliberate choice.
- **Inspectable model capabilities.** For any supported embedding model, the consumer can ask AgentKit for that model's native dimension, the dimensions it can produce, and its maximum input size — so the consumer can present choices and validate a requested dimension before calling, without provider-specific knowledge of its own.

### Shared (both capabilities)

- **Uniform, inspectable errors.** Failures arrive as a uniform, classifiable set of errors so the consumer never needs provider-specific error knowledge, and each error carries the raw provider error response inside it for inspection.
- **Automatic resilience.** Transient failures and rate limits are retried by AgentKit; the consumer only sees an error after retries are exhausted.
- **Uniform usage accounting.** Each result carries token-usage information in a uniform shape — for embeddings, the input tokens consumed — so consumption can be tracked without provider-specific parsing.
- **Dollar-cost accounting.** AgentKit ships built-in per-model pricing and reports the dollar cost of usage — per call and cumulatively for the life of the object — so the consumer sees spend without supplying or maintaining rate data themselves. Because every supported model (chat and embedding) has a pricing entry by construction, cost is **always** reported; there is no "cost unavailable" state.

## Success criteria (outcomes)

### Chat

- A consumer can construct a conversation object with a chosen provider, model, and explicitly supplied credentials, and hold a multi-turn text conversation that returns coherent replies.
- The same calling code produces working conversations against Anthropic, Google, OpenAI, and Z.ai — switching between them is a configuration change only.
- The provider/model can be changed between turns and the conversation continues coherently against the newly selected provider/model, with prior history intact.
- A turn is delivered as an ordered sequence of completed units — each assistant message, tool-call request, and tool result surfaced as it completes, in order — so the intermediate steps of a tool-using run are observable; delivery is at message granularity, not token-by-token, and each completed assistant message carries its full text and any reasoning summary.
- A consumer can define and register a custom tool; when the model requests it, AgentKit invokes the tool and completes the loop automatically, returning the finished result.
- A consumer can attach a remote MCP server with explicitly supplied connection details and credentials, and the model can call that server's (name-prefixed) tools through the same automatic loop, with results fed back — identically across providers.
- Attached MCP servers can be changed between turns; the tools available to the model reflect the servers currently attached, with prior conversation history intact.
- A server that is unreachable when attached, or a transport failure during a tool call, surfaces as a uniform classifiable error; a tool that returns an error result is fed back to the model and the loop continues.
- A tool name that would collide with an existing tool surfaces as a uniform error rather than silently shadowing it, and each attached server's name prefixes its tools so tools remain uniquely addressable.
- The consumer can observe every message in the exchange — user messages, model turns, tool-call requests, and tool results, including MCP tool calls — with nothing filtered out.
- A tool whose declared input shape uses constructs the selected provider's schema dialect cannot natively represent still works: AgentKit conveys what it can preserving meaning, and for any part it cannot convey, surfaces a clear warning naming the tool and that construct rather than silently weakening the tool — uniformly for custom and MCP tools, on every provider.
- A turn's visible answer text and its tool-call requests are surfaced (and tool calls executed) on every provider even when the model attaches reasoning/thinking metadata to them; reasoning artifacts never suppress visible text, misclassify an answer as a thinking summary, or drop a tool call.
- For any supported chat model, a consumer can obtain that model's native reasoning term, the values it accepts (a discrete set, or a valid token-budget range) and its default — without any provider-specific knowledge of its own.
- A reasoning value the selected model natively understands is honored exactly, with no warning.
- A reasoning input the selected model does not natively understand — an unknown term, an invalid or out-of-range value, or a setting carried over from a previously selected model — produces a warning and the model's default reasoning is applied; it is never silently misapplied and never breaks the turn.

### Embeddings

- A consumer can construct an embedder object with a chosen provider (OpenAI or Google), model, and explicitly supplied credentials, and turn one or more input texts into embedding vectors.
- The same calling code produces working embeddings against OpenAI and Google — switching between them is a configuration change only.
- The provider/model can be changed between calls; only configuration changes, and subsequent calls run against the newly selected provider/model.
- A consumer can embed a list of texts of any size in one call and receive exactly one vector per input, in input order, without managing any provider per-call size limit.
- A consumer can tag an embed call as a query, a document, or unspecified; the same call succeeds on every supported provider, and a provider that distinguishes the roles uses the hint.
- A consumer can request a target vector dimension and receive vectors of exactly that size when the model supports it; an unsupported request fails with a clear error rather than returning a different size; with no request, vectors are the model's native dimension.
- Every returned vector is unit-normalized, regardless of provider or requested dimension.
- An input that exceeds the selected model's size limit produces a clear error; AgentKit never silently truncates the input.
- For any supported embedding model, a consumer can obtain that model's native dimension, the dimensions it can produce, and its maximum input size — without provider-specific knowledge of its own.

### Shared

- Failures surface as a uniform, classifiable set of errors, each carrying the raw provider error for inspection.
- Transient failures and rate limits are retried automatically, and the consumer sees an error only after retries are exhausted.
- Each result carries uniform token-usage information (for embeddings, input tokens consumed).
- AgentKit reports the dollar cost of usage from built-in per-model pricing — per call and cumulatively — for every supported chat and embedding model; cost is always available (there is no unpriced supported model).
