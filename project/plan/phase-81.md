# Phase 81 — the explicit reasoning on-value and its per-wire lowering

*Realizes design Decision 6 (generation settings and the native reasoning value).*

Adds `EnableReasoning()` to the root `agentkit` package as a fifth, mutually-exclusive `ReasoningValue` state, and teaches every provider adapter what to do with it. The observable end state:

- `agentkit.EnableReasoning()` exists in `reasoning.go` alongside `Level`, `Budget`, and `DisableReasoning`, carried by the same unexported tag so it can never be combined or half-set with another form.
- `openrouter` lowers it to `{"reasoning":{"enabled":true}}`, `zai` to `thinking:{"type":"enabled"}`, and `anthropic` to `thinking:{"type":"adaptive"}` — each package's own encoder, no shared special-casing.
- `google` and `openai` reject it with `agentkit.ErrInvalidConfig` **before transport**, because those wires express on-ness only as a budget or a level. Neither substitutes a value.
- `ReasoningSpec.Accepts` (in `reasoning.go`) admits the enabled state when the spec's `CanEnable` is set. `CanEnable` is added to `ReasoningSpec` in this phase as an unset-by-default field so this compiles; Phase 82 populates it and enforces its invariants.
- `R-T6G3-NJ7L`'s existing exclusivity test in `reasoning_test.go` is extended from four states to five, so an `EnableReasoning` value is shown to report neither `Disabled()` nor a level nor a budget.

The live check (`R-DGCO-E7GX`) is integration-gated in the existing style (`-tags integration`, real `OPENROUTER_API_KEY`) and is the only test that can prove the on-form does anything: it drives `x-ai/grok-4.20`, whose provider default is off, and compares reasoning-token counts between an unset value and an explicit enable.

**Done when** all of the following hold:

- Each id below is covered by a clearly-named test carrying the id verbatim as a tag:
  - `R-T6G3-NJ7L` — the five `ReasoningValue` states are mutually exclusive; an `EnableReasoning` value never presents as unset, a level, a budget, or disabled.
  - `R-DCOZ-8W8U` — `EnableReasoning()` lowers to `{"reasoning":{"enabled":true}}` on OpenRouter, `thinking:{"type":"enabled"}` on Z.ai, and `thinking:{"type":"adaptive"}` on Anthropic, byte-comparable against the hand-written fragment, with no level or budget field emitted alongside.
  - `R-DDWV-MNZJ` — the Google and OpenAI adapters return `ErrInvalidConfig` for `EnableReasoning()` and issue no HTTP request, asserted with a transport that fails the test if invoked.
  - `R-DF4S-0FQ8` — on a wire with both forms, enable and disable lower to their own distinct fragments and neither is emitted for the other.
  - `R-DGCO-E7GX` — live, integration-gated: against `x-ai/grok-4.20`, an unset `Reasoning` returns a completed response reporting zero `reasoning_tokens` and `EnableReasoning()` returns one reporting more than 100.
- `go build ./...` and `go test ./...` both exit 0 (design Conventions).
- The integration suite compiles: `go vet -tags integration ./...` exits 0.
