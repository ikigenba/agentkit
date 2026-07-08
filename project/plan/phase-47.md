# Phase 47 — Live deferred-tools integration: a real provider accepts the mid-turn grown tools array (R-DFH0-A8TE)

*Realizes design Decision 23 (the real-substrate Verification id R-DFH0-A8TE). Depends on Phase 46 (the deferred-tools implementation this exercises) and the Decision 13 integration-test convention.*

Phase 46's offline tests prove the mechanics against the fake provider — but the fake accepts whatever it is handed. The claim that actually matters is external: a real provider API accepts a `tools` array that **grows between the round-trips of one turn** and lets the model call the newly appeared tool. Per the design's testing responsibility for claims that hinge on a real external contract, this id is exercised live, double-gated so the default suite stays offline and credit-free.

End state:

- A `//go:build integration` test (root package, alongside the existing integration-tagged tests) that is **skipped** — not failed — when the Anthropic credential env var is absent (the D13 / R-WJLM-7QRP discipline).
- When the key is present, the test builds a `Conversation` with one deferred group whose tool returns a distinctive real result, prompts a live Anthropic model with a task that requires that tool, and asserts the full corrective/load path end-to-end: the model calls `load_tools`, the grown tools array is accepted on the following round-trip (no API rejection), the model calls the loaded tool natively, and the turn completes with a final answer incorporating the tool's result — the asserted outcome is a finished live deferred-load turn, not a configured value.

**Done when:** R-DFH0-A8TE is covered by an `//go:build integration`, key-gated test that drives a real Anthropic turn through `load_tools` to a natively-called loaded tool and a completed final answer; the test is skipped (not failed) when the key is absent; and the offline suite is green per design Conventions.
