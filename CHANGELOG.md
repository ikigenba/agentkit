# Changelog

All notable changes to this project are documented in this file.

## v0.3.0 — 2026-07-17

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
