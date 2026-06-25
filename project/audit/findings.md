# AgentKit — Test-Quality Audit Findings

A read-only audit of every design Verification id (`R-XXXX-XXXX`), checking that the
test carrying the id-comment **genuinely asserts the behavior the id describes** —
not a hollow, tautological, proxy, or partial-coverage test that merely makes the
coverage grep count it.

Method: one subagent per design Decision (D1–D22; D12 is structural/no-ids and was
skipped). Each read the Decision's Verification claims, located the `// R-XXXX-XXXX`
tests, read them in full against the production code, and classified each id
`OK` / `WEAK` / `MISSING`. No code was changed.

## Summary

- **Ids audited:** 149
- **OK** (genuine, falsifiable test): **136**
- **WEAK** (test exists but doesn't really assert the claim): **13**
- **MISSING** (no real test): **0**

Decisions fully OK: D1, D5, D7, D8, D9, D13, D16, D18, D19, D20, D21, D22.

---

## A. Tautological or pure-proxy assertions

The test cannot fail for the right reason — it asserts a restatement of its own setup,
or a different behavior that stands in for the real claim.

| id | Decision | test:line | Why it's weak |
|---|---|---|---|
| `R-IKKQ-Z3B4` | D3 | `block_test.go:14` | Pairing check is a tautology — the test sets `result.ToolUseID = use.ID`, then asserts they are equal, so it can never fail regardless of pairing logic. (The strict-charset minting half *is* genuinely tested.) |
| `R-CCI4-0UEA` | D2 | `orchestration_test.go:471` | Asserts only that a later `Send` succeeds — which is already `R-Y4JJ-1J5G`'s rollback/flag-clear claim. Never asserts the actual claim (HTTP body closed / no goroutine leak); the synchronous fake provider returns a fully-materialized round-trip with no real body or goroutine to observe. |
| `R-B96T-WUUR` | D6 | `openai_test.go:299` | Uses a reasoning level invalid for the model in a *fresh* conversation — never actually carries a value over from a previously-selected model (no model switch). Mechanically identical to the wrong-value case (`R-B7YX-J342`); the carried-over scenario the claim is about is untested. |
| `R-PM3H-UYFS` | D15 | `log_test.go:180` | Claim is "with `Log == nil`, **no** records are written." The test only asserts the turn succeeds and `TotalUsage` is correct; it never asserts the *absence* of records. Proxy for the real (absence) claim. |

## B. Multi-clause claims where only some clauses are exercised

The central behavior is genuinely asserted, but a named sub-clause of the claim is
never tested.

| id | Decision | test:line | Uncovered clause |
|---|---|---|---|
| `R-CBA7-N2NL` | D2 | `orchestration_test.go:217` | `DeepEqual(emitted, History)` is proven only for a single-`TextBlock` message; the claim's enumerated **tool_use-block** and **reasoning-summary** message shapes are never asserted emitted-and-equal-to-History. |
| `R-IPGC-I69W` | D3 | `google_test.go:164` | Fixture has a **single** tool call, so `BoundToID == use.ID` passes trivially; the **parallel / positional disambiguation** that is the heart of the claim is never exercised. |
| `R-6W63-I4FW` | D10 | `mcp_integration_test.go:369` | Merged sorted order stable across turns is asserted; the claim's **attach/detach deterministic re-ordering** clause is not (that path lives under `R-6SIE-CT7T`, which only checks tool removal). |
| `R-Y878-6UDJ` | D11 | `retry_test.go:176` | Per-round-trip budget reset (clause 1) is covered; **"a failure after any round-trip has delivered an event is not retried"** (clause 2) is never exercised. |
| `R-6XDZ-VW6L` | D11 | `mcp_integration_test.go:301` | Retry-on-500 half is covered; the **fail-fast on 401/403/400 and non-MCP 4xx** half is not asserted under this id (the 401/403 tests carry other ids and don't check no-retry / zero-sleep). |
| `R-6YLW-9NXA` | D11 | `mcp_integration_test.go:337` | `tools/call` 500 no-retry (clause 1) is covered; **"once a byte of a tool-result SSE stream is delivered it is never retried"** (clause 2) is untested. |
| `R-6L70-26RN` | D17 | `mcp_integration_test.go:178` | Single-server discovery RPC error is covered; the **unreachable server** case and the **cross-server isolation** clause ("one failing server does not corrupt other servers' tools") are never exercised — every failure test uses a single server. |
| `R-P61J-IHKB` | D11 | `retry_test.go:145` | Asserts the *inverse*: a partial message bundled with an error (never delivered) **is** retried. The claim — "a failure after an event is delivered is never retried, regardless of category" — is structurally unfalsifiable here because round-trips are buffered whole, so an errored round-trip never delivers an event. See theme 1. |

## C. Test diverges from the design (real discrepancy, not just a test gap)

| id | Decision | test:line | Finding |
|---|---|---|---|
| `R-6ZTS-NFNZ` | D4 | `mcp_integration_test.go:370` | Asserts warning count, attribution, and dropped keywords, but **not** the claim's named `Code: WarnToolSchemaLossy` / `Setting: "tool_schema"` — and production actually emits `Setting: "mcp_schema"`. Also exercised through a fake limiter rather than the real Gemini conversion. This is a genuine **design-vs-code mismatch** that needs a decision (fix code, or correct the design), not merely a stronger assertion. |

---

## Cross-cutting themes

1. **The "no-retry after the first byte is delivered" family** — `R-P61J-IHKB`, the
   second clause of `R-Y878-6UDJ`, and the second clause of `R-6YLW-9NXA`. These are
   unfalsifiable as written because the architecture buffers each round-trip whole:
   an errored round-trip never delivers an event, so there is no partial-delivery
   state for a test to assert against. This reads as a **design-level observation**
   (the claim cannot be proven at this seam) rather than a test gap — surface for a
   design decision rather than patch.

2. **Fakes that cannot observe the claim** — `R-CCI4-0UEA` (goroutine leak / body
   close) and `R-6ZTS-NFNZ` (real Gemini conversion) need a different test substrate,
   not a stronger assertion against the existing fake.

3. **Golden-list-bounded completeness** (noted, scored OK) — "every exported model
   constant resolves" claims (e.g. `R-S7V8-5QL3`, `R-V1KQ-IKI6`) are checked against
   hand-curated model lists, not a programmatic enumeration of exported constants. A
   newly added constant omitted from both registry and fixture would slip through.
   Matches the design's stated "golden / belt-and-suspenders" framing, so not a
   finding — but the guarantee is list-bounded, not structurally enforced.
