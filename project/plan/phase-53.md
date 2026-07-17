# Phase 53 — Root cost & options seam: resolution, reported cost, provider options

*Realizes design Decision 16 (cost resolution seam), with the structural root changes of Decisions 1, 6, and 9 it rides on. Depends on Phase 52.*

The root `agentkit` package moves to the consumer-owned cost seam. `Conversation` gains the `Pricing *Pricing` sibling of `Model` (D1). The `Provider` SPI no longer requires a `Pricing(model)` method; `Request` gains the opaque `ProviderOptions json.RawMessage` field; `RoundTrip` gains `ReportedCost() (Cost, bool)` with the corresponding constructor slot (D9). The orchestrator drops the pre-turn pricing lookup and gate entirely and resolves cost per round trip: provider-reported first, else `Conversation.Pricing.Cost(usage)`, else `Cost(0)` plus a new `Warning{Setting:"cost", Code:WarnCostUnknown}` on that round trip and in its D15 record (D16). The `WarningCode` enum gains `WarnCostUnknown`. Sub-packages receive only the mechanical accommodation this compile requires (constructor-slot signatures); their behavioral rework is Phases 54–58. Existing suite stays green throughout.

**Done when:** the suite is green and each of this phase's ids is covered by a clearly-named test —
- R-CZ7D-VSL4 — reported cost wins over supplied `Pricing`; `Stream.Cost()`/`TotalCost()` reflect it.
- R-D0FA-9KBT — no reported cost + `Pricing` set → `Stream.Cost() == Pricing.Cost(usage)`.
- R-D1N6-NC2I — neither source → turn succeeds, `Cost()==0`, exactly one `WarnCostUnknown` per affected round trip, repeated per turn, present in the log record, gone once `Pricing` is set.
