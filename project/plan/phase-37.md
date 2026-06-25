# Phase 37 — Falsifiably pin the nil-Log no-write guarantee (de-proxy R-PM3H-UYFS)

*Realizes design Decision 15 (structured JSONL event log & conversation lifecycle — the nil-Log claim R-PM3H-UYFS). Depends on Phase 07 (the JSONL event log) and Phase 05 (the orchestration tool loop that drives every record-emitting site).*

Close audit Finding A/`R-PM3H-UYFS` (`project/audit/findings.md`, section A): the claim is "with `Log == nil`, **no** records are written," but the test (`log_test.go:180`, `TestNilLogDisablesRecordWriting`) only asserts the turn succeeds and `TotalUsage()` is correct — a proxy that never exercises the absence-of-writes guarantee.

A literal "assert the records that were written are zero" is not observable through a `nil` writer — there is nothing to inspect. The falsifiable guarantee behind the claim is that **every record-emitting site is gated on `Log != nil`**: a regression that writes a record unconditionally would dereference the nil `io.Writer` and panic. To pin that, the nil case must drive a turn that reaches *every* record site, paired with a positive control proving the same turn is genuinely log-active when a writer is present.

End state:

- A positive-control sub-test runs a **tool-using, multi-record turn** (so it exercises `turn_start`, an assistant `message`, a `tool_use`, a `tool_result`, `usage`, and `turn_end`) with `Log` set to a `bytes.Buffer`, and asserts the buffer contains the full expected set of record types — proving this exact turn is log-active.
- A nil-Log sub-test runs the **same multi-record turn** with `Log == nil` and asserts the turn completes without panic and with the identical successful outcome (`Stream.Err() == nil`, `History` committed, `TotalUsage()` correct). Because every record-emitting site is reached with a nil writer, a future regression that writes any record type unconditionally panics and fails this test — making the no-write guarantee falsifiable rather than incidental.
- The design claim's wording is left unchanged; this phase strengthens the test only (section A is a test-quality finding, not a design mismatch).

**Done when:** R-PM3H-UYFS is covered by a clearly-named test whose nil-Log path drives a turn reaching every record-emitting site without panic, backed by a positive control proving the same turn writes the full record set when a writer is present; the other D15 ids stay green; the suite is green per design Conventions. No production change is expected; if exercising every site under nil exposes a real unguarded write, fix the production code.
