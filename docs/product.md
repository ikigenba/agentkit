# AgentKit — Product

**Authority: intent.** This document owns *why* AgentKit exists, *for whom*, what is in and out of scope, and the behavior we **promise** the user — stated once, in outcome terms. It does **not** state mechanism, type shapes, exact wire formats, error/category names, retry policies, exit codes, or test assertions; those belong to `docs/design.md`. Where the two could overlap on behavior, this doc states the *promise* (what the consumer observes) and design states the *exact, checkable proof* of that promise. That boundary is load-bearing: it keeps product, design, and plan from overlapping.

## Problem

A developer building an application on top of large language models has to choose a provider — Anthropic, Google, OpenAI — and then write code against that provider's specific API: its message shape, its tool-use protocol, its streaming format, its error responses, its retry expectations. Adopting a second provider, or letting an app switch between them, means a second integration written from scratch and a calling surface that diverges per provider. The provider's idiosyncrasies leak into application code, so the app is coupled to a vendor it would rather treat as a swappable detail.

Beyond writing each tool by hand, the same application increasingly has whole sets of capabilities already exposed through external **MCP (Model Context Protocol) servers** — hosted services, internal tooling — that it would like the agent to use. Wiring those in itself means another bespoke integration: speaking the MCP protocol, discovering the server's tools, and threading them into whatever tool-use shape the chosen provider expects, per provider. The app wants to point at an MCP server and have its tools simply become available to the agent — the same way, across every provider.

## Purpose

AgentKit is a Go library that gives an application **one uniform surface for holding a tool-using, multi-turn text conversation with an LLM**, regardless of which provider backs it. The single job it does: present the same calling code for chat, tools — both consumer-defined and supplied by attached MCP servers — and streaming across Anthropic, Google, OpenAI, and Z.ai, so the application sees one input/output surface and the choice of provider and model is configuration.

## Users

Go developers building applications that talk to LLMs. AgentKit's first and most important consumer is the ikigenba family of applications, but it is a general-purpose public library held to a public bar — clean surface, documented, versioned — and "would this be appropriate for any developer to adopt?" is treated as a standing health check on its design. Its users want to write LLM-backed features (chat, agents with tools) once and run them against whichever provider and model they configure, without learning three vendor APIs.

## Scope

AgentKit covers, for v1:

- A **multi-turn, text-only chat** with an LLM.
- **Custom tools** defined and registered by the consuming application, which AgentKit invokes and loops on automatically until the model produces a final answer.
- **MCP tool servers.** The consuming application can attach one or more **remote MCP servers**; AgentKit acts as the MCP client — it connects, discovers each server's tools, and feeds them into the *same* automatic tool loop as custom tools, uniformly across every provider. The model and the consumer see them as ordinary tools. The consumer assigns each attached server a name, which prefixes that server's tools so they are uniquely addressable and recognizable. MCP connection details, including any credentials, are supplied explicitly by the consumer.
- **Incremental (streaming) delivery** of replies.
- **Anthropic, Google, OpenAI, and Z.ai** as providers, each a first-class peer with at least one supported model — a provider reached through API-compatibility is no less first-class, and how it is implemented is not user-visible. The set of supported models is **fixed and curated**: AgentKit exposes a closed set of pre-approved models and the consumer selects from it — there is no raw model-string passthrough. Provider and model selection is configuration, chosen from that set. The exact supported model ids are enumerated once, in the design's per-provider model registry — not duplicated here, so the two cannot drift. Every supported model has a built-in pricing entry by construction.
- A consumer-held state object that bundles configuration (provider, model, credentials, generation settings) and the running conversation history, threaded explicitly into each call.
- A uniform error surface, automatic retrying of transient and rate-limit failures, uniform token-usage reporting, dollar-cost reporting from built-in per-model pricing, and full visibility of every message in the exchange.

It deliberately does **nothing else.** In particular:

- **No image or audio, input or output.** Messages are text only; vision and image/audio generation are out.
- **No image generation, audio, batch/asynchronous bulk processing, or fine-tuning.** AgentKit is about live, interactive message exchange only.
- **No durable persistence.** The state object lives in memory for the life of the object; saving and restoring conversations across process restarts is not promised (a consumer may serialize the object itself).
- **No ambient credential sourcing.** AgentKit never reads environment variables, files, or any credential store on its own.
- **No uniformity promise for provider-specific extras.** The uniform core is what's promised; capabilities unique to one provider are not promised to be uniform.
- **MCP brings in tools only.** Resources and prompts that an MCP server may also expose are out of scope (deferred, not rejected); only its tools are surfaced.
- **Remote MCP servers only.** AgentKit reaches MCP servers over a network connection and spawns **no** child processes; local stdio/subprocess MCP servers are out of scope.
- **No interactive OAuth.** The consumer supplies a ready credential for a server; AgentKit does not negotiate or refresh OAuth flows on the consumer's behalf.
- **No MCP provenance promise.** MCP tools appear in the exchange as ordinary tools; AgentKit does not promise a consumer-visible distinction between an MCP tool and a custom tool beyond the name the consumer's server-prefix gives it.

**Embeddings** are a committed future direction — AgentKit *will* support embeddings in a later phase — but they are **not part of v1** and are not promised by this version. They are distinct from the permanently-excluded items above.

## Contractual constants

These fixed, promised values the design must use verbatim and never re-declare:

- **Module path:** `github.com/ikigenba/agentkit`
- **Starting version:** `v0.1.0` — the `v0` major signals a public but pre-stable surface that may evolve through research; promotion to `v1.0.0` happens only when stability is promised.
- **Minimum Go version:** Go 1.26.

## What we promise (user-facing behavior)

- **One conversation surface across providers.** The consumer constructs a state object with a provider, a model, and credentials supplied explicitly at construction, then holds a multi-turn text conversation by passing that object into each call. The calling code is identical regardless of provider; the conversation history accumulates in the object the consumer holds.
- **Provider/model selection is configuration — including mid-conversation.** Choosing Anthropic vs. Google vs. OpenAI vs. Z.ai, and which model, is a configuration choice in the state object. That choice can be changed between turns: the accumulated conversation history carries over, and the next turn runs against the newly selected provider and model.
- **Incremental replies.** Replies are delivered as they are generated, so the consumer can process partial output before a turn completes. Incremental delivery is the delivery surface.
- **Custom tools, driven automatically.** The consumer defines a tool — a name, a description, the shape of its input, and the code that runs it — and registers it. When the model asks to use a tool, AgentKit calls the consumer's code, feeds the result back to the model, and repeats until the model produces a final answer. The consumer gets the finished result without managing the back-and-forth themselves.
- **MCP tools, attached by configuration.** The consumer attaches a remote MCP server by supplying — explicitly — its connection details, a name, and any credentials it needs. AgentKit connects, discovers the server's tools, and makes them available to the model through the same automatic loop as custom tools; the server's name prefixes its tools so they stay uniquely addressable. Attached servers are part of the state object and can be attached or detached **between turns**, mirroring provider/model selection — the tools available to the model reflect the servers currently attached. If a server is unreachable when attached, or a tool's transport fails mid-call, the consumer sees a uniform error; a tool that simply returns an error result is fed back to the model so the conversation continues. A tool whose name would collide with an existing tool surfaces as a uniform error rather than silently shadowing it.
- **Full transparency of the exchange.** Every message reaches the consumer: the user's messages, the model's turns, the model's tool-call requests, and the tool results — including those from MCP-provided tools. Nothing is filtered out; the consumer can observe the complete trace, not only the final text. A model's visible answer and its tool-call requests are always surfaced as such, regardless of any reasoning or thinking metadata a provider attaches to them: provider-specific reasoning artifacts never suppress a turn's visible text, never silently turn a real answer into a thinking summary, and never swallow a tool call so that it goes unexecuted.
- **Uniform, inspectable errors.** Failures arrive as a uniform, classifiable set of errors so the consumer never needs provider-specific error knowledge, and each error carries the raw provider error response inside it for inspection.
- **Automatic resilience.** Transient failures and rate limits are retried by AgentKit; the consumer only sees an error after retries are exhausted.
- **Uniform usage accounting.** Each reply carries token-usage information — input, output, cached, and the like — in a uniform shape, so consumption can be tracked without provider-specific parsing.
- **Dollar-cost accounting.** AgentKit ships built-in per-model pricing and reports the dollar cost of usage — per turn and cumulatively for the conversation — so the consumer sees spend without supplying or maintaining rate data themselves. Because every supported model has a pricing entry by construction, cost is **always** reported; there is no "cost unavailable" state.

## Success criteria (outcomes)

- A consumer can construct a state object with a chosen provider, model, and explicitly supplied credentials, and hold a multi-turn text conversation that returns coherent replies.
- The same calling code produces working conversations against Anthropic, Google, OpenAI, and Z.ai — switching between them is a configuration change only.
- The provider/model can be changed between turns and the conversation continues coherently against the newly selected provider/model, with prior history intact.
- Replies are delivered incrementally; a consumer can process partial output before a turn completes.
- A consumer can define and register a custom tool; when the model requests it, AgentKit invokes the tool and completes the loop automatically, returning the finished result.
- A consumer can attach a remote MCP server with explicitly supplied connection details and credentials, and the model can call that server's (name-prefixed) tools through the same automatic loop, with results fed back — identically across providers.
- Attached MCP servers can be changed between turns; the tools available to the model reflect the servers currently attached, with prior conversation history intact.
- A server that is unreachable when attached, or a transport failure during a tool call, surfaces as a uniform classifiable error; a tool that returns an error result is fed back to the model and the loop continues.
- A tool name that would collide with an existing tool surfaces as a uniform error rather than silently shadowing it, and each attached server's name prefixes its tools so tools remain uniquely addressable.
- The consumer can observe every message in the exchange — user messages, model turns, tool-call requests, and tool results, including MCP tool calls — with nothing filtered out.
- A turn's visible answer text and its tool-call requests are surfaced (and tool calls executed) on every provider even when the model attaches reasoning/thinking metadata to them; reasoning artifacts never suppress visible text, misclassify an answer as a thinking summary, or drop a tool call.
- Failures surface as a uniform, classifiable set of errors, each carrying the raw provider error for inspection.
- Transient failures and rate limits are retried automatically, and the consumer sees an error only after retries are exhausted.
- Each reply carries uniform token-usage information (input, output, cached, etc.).
- AgentKit reports the dollar cost of usage from built-in per-model pricing — per turn and cumulatively — for every supported model; cost is always available (there is no unpriced supported model).
