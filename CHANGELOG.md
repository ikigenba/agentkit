# Changelog

All notable changes to this project are documented in this file.

## v0.5.0

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
