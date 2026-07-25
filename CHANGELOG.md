# Changelog

All notable changes to this project are documented in this file.

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
