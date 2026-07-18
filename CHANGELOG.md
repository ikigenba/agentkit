# Changelog

All notable changes to this project are documented in this file.

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
