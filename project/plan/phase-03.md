# Phase 3 — Pricing and cost engine
*Realizes design Decision 16 (baked-in pricing & cost), struct and math only. Depends on Phase 1.*

`Pricing`, `RateTier`, `Cost`, `Cost.USD()`, and `Pricing.Cost(Usage)` exist — tier selection by a turn's total input tokens and exact nano-USD integer rating of every bucket (reasoning billed at the output rate). The per-provider rate tables and registry-completeness ship with the adapter phases; cumulative `TotalCost` is wired in orchestration/logging.

**Done when:** R-PTEW-5KVY, R-V2SM-WC8V, and R-PX2L-AW41 are covered (rating math against synthetic `Pricing` literals, tier selection by input size, USD conversion); suite green.

