# Phase 45 — Live Gemini integration: a faithfully-translated tool schema is accepted and a tool-using turn round-trips (R-9UK4-JI3L)

*Realizes design Decision 22 (the real-substrate Verification id R-9UK4-JI3L). Depends on Phase 44 (the faithful translation this exercises) and the Decision 13 integration-test convention.*

Phase 44's offline tests prove the **shape** of the translated schema (a `$ref` is inlined, a `oneOf` becomes `anyOf`, residue is reported) against a fake. They cannot prove the one thing that actually matters for a translator: that the schema AgentKit produces is one the **real Gemini API accepts**. A converter could emit a perfectly-shaped `anyOf` the live API still rejects. Per the design's testing responsibility for claims that hinge on a real external contract, this id is exercised against the live provider, double-gated so the default suite stays offline and credit-free.

End state:

- A `//go:build integration` test (in `google/`, alongside the existing integration-tagged tests) that is **skipped** — not failed — when the Gemini credential env var is absent (the D13 / R-WJLM-7QRP discipline).
- When the key is present, the test constructs a tool whose JSON Schema uses a `$ref` (resolved against `$defs`) and a `oneOf`, runs a real tool-using turn against a live Gemini model through the ordinary `Conversation` surface, and asserts that the request is **accepted** (no schema-rejection error from the API) and that the tool call **round-trips to a completed result** — the asserted outcome is a finished live tool-using turn, not a configured value.
- If a replayable offline twin is wanted, its fixture is captured once under the D13 recording discipline (keys scrubbed, committed); the gated live test remains the substrate of record for this id.

**Done when:** R-9UK4-JI3L is covered by an `//go:build integration`, key-gated test that drives a real Gemini tool-using turn with a `$ref`+`oneOf` tool schema and asserts the live API accepted the translated schema and the tool round-tripped to completion; the test is skipped (not failed) when the key is absent; and the offline suite is green per design Conventions.
