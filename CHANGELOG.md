# Changelog

All notable changes to this project are documented in this file.

## v0.14.0

- Added `toolkit.WebSearch`, a Brave Search API-backed web search tool that
  accepts a consumer-supplied key and supports count, freshness, locale,
  safe-search, and result-type controls. Results are returned as clean, bounded
  JSON containing the useful web and sibling result sections.
- Added `toolkit.WebFetch`, which fetches `http` and `https` pages with a
  per-call adjustable timeout, converts HTML to readable markdown, returns
  other text content verbatim, and refuses binary content.
- Kept both network tools opt-in: neither joins `toolkit.All`, which continues
  to return exactly the six local coding tools. HTML parsing for `WebFetch`
  adds `golang.org/x/net` as a module dependency.

## v0.13.0

- Normalized completed assistant messages so every provider, including Google,
  returns each uninterrupted answer as a single `TextBlock` rather than one
  block per SSE frame. Text separated by reasoning, tool use, or tool results
  remains separate and ordered.
- Fixed Google reasoning replay when Gemini sends a `thoughtSignature` and its
  `functionCall` in different SSE frames. The signature now binds to the call
  instead of being silently detached at the frame boundary.
- Enforced adjacent-text normalization centrally for every current and future
  provider adapter, so assembled messages no longer depend on how a transport
  divides identical content into frames.

## v0.12.0

- Replaced raw tool schemas with AgentKit's canonical, provider-portable schema
  subset. `NewTool` now derives schemas from Go input structs and rejects
  unsupported shapes early, while request validation reports `ErrInvalidConfig`
  instead of allowing a provider to silently ignore an untranslatable
  constraint.
- Rendered canonical tool schemas into each provider's native dialect. Anthropic
  and OpenAI requests now enable strict schema handling, while Google receives
  only the schema vocabulary it supports.
- Made serialized tool definitions deterministic by ordering tools by name in
  every provider request, regardless of the order supplied by the consumer.
- Completed the generated schemas for all six `toolkit` tools with descriptions
  for every input property and required fields aligned with each tool's
  contract.
- Removed `RawTool`, `ToolSchemaTranslator`, and lossy-schema warnings, along
  with the external schema-reflection dependency; consumers now get one
  validated schema path rather than provider-dependent degradation.

## v0.11.0

- Completed every catalog offering with full pricing, reasoning specification,
  and context metadata, removing the blank secondary-offering tier.
- Added OpenRouter offerings for every tracked chat model, with each route's
  reasoning vocabulary documented and confirmed and its rates audited.
- Added per-offering wire-name overrides for the three Anthropic model slugs
  whose wire names contain dots.

## v0.10.0

- Organized the catalog by offering. `catalog.Entry` replaced `Provider` and
  `Routes` with a `Vendor` and an ordered `Offerings` slice, with the default
  route at index zero. Rates, reasoning vocabulary, and context size moved onto
  each offering, so consumers can name a model and receive every route in
  preference order, with each route's terms, in one call. In particular,
  `glm-5.2` through OpenRouter no longer reports Z.ai's figures.
- Changed `Resolve` to return a `Resolution` instead of four values and removed
  the trailing `ok` boolean. Its `Coverage` reports `Curated`, `Passthru`, or
  `Unrouted`, so a missing catalog entry for a provider/model pair is distinct
  from whether that provider can serve the model. Every coverage state still
  returns a runnable model string; none gates execution.
- Derived wire model ids instead of storing them. Consumers name `grok-4.5`,
  optionally with `openrouter`, and `Entry.WireModel` computes the provider's
  namespaced id without exposing or requiring `x-ai/grok-4.5`.
- Added `Offerings`, `Offer`, `Entry.WireModel`, and `catalog.VendorID`. Renamed
  `ListByProvider` to `ListCurated` to reflect that it lists catalog coverage,
  not everything a provider can serve. `Check` now accepts a provider because
  different routes to the same model can accept different values.
- Changed providers to report an `Identity` instead of a name:
  `Provider.Name() string` and `EmbeddingProvider.Name() string` became
  `Identity() Identity`, with `ProviderID` and `AuthMode` carried separately.
  `agentkit.Error` likewise split `Provider` into typed `Provider` and `Auth`
  fields, while JSONL `turn_start` records gained an `auth` field. Consumers
  reading errors or logs now receive `provider` and `auth` independently where
  they previously saw `"openai.subscription"`; `Identity.String()` preserves
  that combined form for display.
- Renamed `zai`'s provider id to `z-ai` in errors and log records. The new
  spelling matches the vendor namespace used to derive OpenRouter ids; the Go
  package remains `zai`.
- Changed `ReasoningSpec.Default` from `agentkit.ReasoningValue` to
  `catalog.ReasoningDefault`. It records `DefaultOff`, `DefaultFixed`,
  `DefaultDynamic`, or the zero `DefaultUnaudited`, and carries a value only
  when one exists. Providers that decide reasoning per request no longer need
  an invented default value.
- Added `ReasoningSpec.CanEnable` so the catalog reports permission to turn
  reasoning on separately from permission to turn it off.
- Added `agentkit.EnableReasoning()`, an explicit on-form lowered to each
  provider's native wire representation. This made `grok-4.20` reasoning
  reachable for the first time because the model reasons only when explicitly
  enabled. Google and OpenAI reject this option with `ErrInvalidConfig` because
  their wires have no bare on-form.
- Corrected catalog data measured against the live APIs, most visibly marking
  `kimi-k3` with `CanDisable: true` and recording `grok-4.20` as defaulting to
  off.
- Preserved what AgentKit sends for existing code: an unset reasoning value
  still transmits no reasoning fields, so the provider's own default applies
  exactly as before.

## v0.9.0

- Changed `ocr.Tool` to return a transcript path under `<root>/ocr/` instead of
  under the consumer's cache directory. Consumers that stored, logged, or
  post-processed the returned path now receive the new location.
- Changed the cache layout to one `<stem>-<hash8>.json` file per document,
  replacing each `<stem>-<hash8>/` directory that contained `response.json` and
  `transcript.md`. This invalidates every existing cache entry: old entries are
  neither read nor migrated, so the first call for each document after upgrading
  re-extracts the document and incurs the provider charge again.
- Allowed the cache directory to live outside the agent's working directory and
  remain durable across runs while keeping the transcript under the root where
  the agent's file tools can open it. The previous layout could not provide both
  behaviors at once.
- Changed cached responses whose transcript can no longer be derived to return
  an error instead of silently re-extracting and re-billing the document.
  Corrupt or unreadable cache entries now surface to the consumer.

## v0.8.0

- Added the `ocr` subpackage with an `OCR` tool that extracts text from scanned
  PDFs and raster images using OpenRouter's `file-parser` plugin. Consumers
  construct it with `ocr.New(ocr.APIKey(key), opts...)` and wire it into an
  agent with `ocr.Tool(root, cacheDir, backend)`.
- Wrote extraction artifacts—the raw provider response and a derived
  `transcript.md`—to the consumer-named cache directory. Repeated calls for
  unchanged input are served without a new provider request, and results return
  a bounded preview plus the transcript path so large documents do not flood
  the conversation.
- Normalized images to one-page PDFs so PDFs and images follow the same
  extraction path.
- Surfaced failed or empty extractions as errors without caching them, including
  provider errors returned with a `200` status.
- Changed `toolkit.Read` to refuse non-text files with an error that names the
  detected content type, rather than decoding binary data as text. Detection is
  based on content rather than the file extension; this change can affect
  existing consumers that used `Read` with binary files.

## v0.7.0

- Added the `toolkit` subpackage with six standard coding tools—`Bash`, `Read`,
  `Write`, `Edit`, `Glob`, and `Grep`—as ready-made `agentkit.Tool` values via
  per-tool constructors and `All(root)`.
- Confined file operations to their configured root with symlink-aware path
  checks. `Bash` remains a shell-command escape hatch and does not provide the
  same filesystem confinement.
- Capped tool results at 30,000 characters, made `Edit` refuse ambiguous
  matches, and gave `Bash` timeout handling that kills the command's process
  group.
- Added recursive `**` globbing and made search skip `.git` contents and binary
  files.

## v0.6.0 — 2026-07-18

- Added catalog entries for the vendors reachable only through OpenRouter: xAI
  Grok (`grok-4.5`, `grok-4.3`, `grok-4.20`, `grok-4.20-multi-agent`), DeepSeek
  (`deepseek-v4-flash`, `deepseek-v4-pro`), and Moonshot Kimi (`kimi-k3`,
  `kimi-k2.7-code`, `kimi-k2.6`). Each defaults to the `openrouter` provider and
  carries its vendor-namespaced wire slug, rates, context size, and reasoning
  spec.
- Fixed the `glm-5.1` reasoning spec, which incorrectly carried GLM-5.2's
  `reasoning_effort` enum. Z.ai scopes that parameter to GLM-5.2; `glm-5.1` has
  the `thinking` toggle only.

## v0.5.0 — 2026-07-18

- Removed the `openai/subscription` `BeginLogin`, `Flow`, `AuthorizeURL`, and
  `Complete` login flow. OAuth login is now the responsibility of an external
  login tool.
- Changed the subscription credential file format from the codex CLI wrapper
  shape to the raw token-endpoint response. The account ID is now derived from
  the `https://api.openai.com/auth` JWT claim.
- Changed subscription token refreshes to preserve the existing `refresh_token`
  and `id_token` when the refresh response omits them.

## v0.4.1 — 2026-07-17

- Removed the interactive `Login` and `LoginIO` surface in favor of the
  value-in/value-out `BeginLogin`, `Flow.AuthorizeURL`, and `Flow.Complete`
  flow. The library performs no terminal I/O, and empty or unparsable pasted
  redirect URLs now return a clearer error.

## v0.4.0 — 2026-07-17

- Added credential constructors for configuring provider clients without exposing
  authentication details through the provider interface.
- Added free-flow model support.
- Added consumer-owned cost resolution: provider-reported cost takes precedence,
  followed by conversation pricing, with zero cost and `WarnCostUnknown` when no
  price is available.
- Added the OpenRouter provider.
- Added subscription authentication and documented that subscription-backed
  usage reports notional cost rather than an additional API charge.
- Added the advisory `catalog` package for model metadata.
- Removed the legacy registries, constants, and reasoning and embedding
  inspectors from the shipped public surface.
