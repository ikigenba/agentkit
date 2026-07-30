# Phase 87 — Router coverage: OpenRouter offerings for every tracked chat model, and the wire-name override

*Realizes design Decision 26 (advisory model catalog) — slice: R-E7VN-JTZ0, R-EBJC-P573, R-EABG-BDGE. Depends on Phase 86.*

The catalog gains the `WireName` override and the 18 remaining OpenRouter
offerings, making every tracked chat model reachable-and-fully-specified on its
OpenRouter route:

- `Offering` gains `WireName string`; `Entry.WireModel` and `Resolve` return a
  matched offering's non-empty `WireName` verbatim and derive by the provider's
  naming rule otherwise. The override is read only after the offering is matched
  by `Provider`.
- New OpenRouter offerings for the 5 Anthropic, 5 Google, and 8 OpenAI chat
  models, transcribed from research: pricing and context from the §6.5
  OpenRouter route table, reasoning specs and defaults from the §14.7 resolved
  offering table. Exactly three offerings set `WireName` (the dotted slugs
  `anthropic/claude-opus-4.8`, `anthropic/claude-sonnet-4.6`,
  `anthropic/claude-haiku-4.5`); every other offering derives.
- The golden reference table (R-DQ16-AQWE) grows the new offerings including
  their `WireName` cells; the existing catalog-enumerated live checks
  (R-4NJ4-SJ41 wire names, R-DOVZ-2LNS CanDisable) extend automatically and
  stay integration-gated.

New tests in `catalog`:
- R-E7VN-JTZ0 — override precedence: non-empty `WireName` returned verbatim by
  `WireModel` and `Resolution.WireModel`; empty derives (`vendor + "/" + model`
  for OpenRouter, verbatim for direct); changing `Vendor` moves only derived
  names; exactly the three dotted Anthropic OpenRouter offerings carry a
  non-empty `WireName` in shipped data.
- R-EBJC-P573 — every shipped chat entry carries an offering whose provider is
  `agentkit.ProviderOpenRouter`; embedding entries exempt.
- R-EABG-BDGE — for every shipped offering of a **native** provider (provider ≠
  `agentkit.ProviderOpenRouter`) with `Kind == ReasoningRange`, `CanDisable`
  equals `Min == 0 || an "off" sentinel exists`; and the haiku OpenRouter
  offering (`ReasoningRange`, `Min: 1024`, no off sentinel, `CanDisable: true`)
  is asserted present as the exemption that makes the native scoping
  load-bearing.

The superseded derivation-only behavior leaves with its id: tests tagged
`R-LQ3Z-65N5` are removed.

**Done when:**
- `go test ./catalog/...` contains passing tests tagged `R-E7VN-JTZ0`,
  `R-EBJC-P573`, and `R-EABG-BDGE`.
- `grep -rn 'R-LQ3Z-65N5' --include='*_test.go' .` returns nothing.
- The suite is green: `go build ./...` and `go test ./...` exit 0.
