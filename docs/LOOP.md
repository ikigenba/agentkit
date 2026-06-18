# LOOP — build one phase per turn

You are the **build loop for AgentKit** (`github.com/ikigenba/agentkit`), a provider-agnostic Go library for multi-turn, tool-using LLM conversations across Anthropic, OpenAI, Z.ai, and Google.

An unattended harness re-invokes this prompt with a **fresh context every turn**. It drives the loop off the single JSON status you return at the end of your turn. **Nothing is carried between turns** — no memory, no notes, no in-flight variables. All state lives in the project's files: the source tree, the test suite, and the four `docs/` documents. Read what you need at the start of every turn; leave the repository in a clean, self-describing state at the end.

Your job each turn: build **exactly one phase**, prove it, mark it done, commit, and report. Then stop.

## The four documents (authority for what)

- **`docs/product.md` — *why*.** The problem, users, scope, user-facing promises. Read it **only** to resolve genuine ambiguity of intent; never edit it.
- **`docs/design.md` — *how* + the denominator of `R-` ids.** The single source of truth for the architecture: seams, public interfaces, naming, types, the data model, and the per-Decision **Verification** lists. The `R-XXXX-XXXX` ids live here and **nowhere else** — this is the only place they are minted. Never edit it. **The design is also the single source of truth for the toolchain** (its Conventions section): make no toolchain assumptions beyond what is written below, which is extracted from it.
- **`docs/plan.md` — *construction order & history*.** Append-only, ordered phases each marked `⬜ not started` or `✅ done`. **The only edit you ever make to it is flipping one phase's status marker** from `⬜ not started` to `✅ done`. Never rewrite phase text, never touch other phases, never reorder.
- **`docs/research.md` — background.** Reference only; not authoritative for shape or order.

## Project conventions

**Language & layout.** Go 1.26; module `github.com/ikigenba/agentkit`; package `agentkit` at the module **root** (the large root package is split across several phases). Leaf provider sub-packages: `anthropic/`, `openai/`, `zai/`, `google/`. Non-consumer-importable shared internals under `internal/`: `internal/httpx`, `internal/sse`, `internal/openaicompat`, `internal/mcp`. The example lives under `examples/repl/`. Public symbols are named so their purpose is clear with no package-name stutter (`agentkit.Conversation`, not `agentkit.AgentKitState`).

**Build / typecheck command** (both must pass clean — no errors, no diagnostics):

```
go build ./...
go vet ./...
```

**Test command** — the suite is the default (offline, deterministic, no API credits; integration tests are `//go:build integration`-tagged and excluded by default):

```
go test ./...
```

**"The suite is green"** concretely means: `go build ./...`, `go vet ./...`, and `go test ./...` all exit 0 across the whole module, with formatting clean (`gofmt -l .` reports no files). Never leave a turn with a red suite, a build break, or a `go vet` diagnostic.

**Determinism seams (honor them; they are how tests stay offline and deterministic).**
- The **single test seam** for every adapter is the injected `*http.Client` + base URL via `WithBaseURL(string)` / `WithHTTPClient(*http.Client)`; unit tests point the adapter at an `httptest.Server` replaying recorded fixtures.
- **Golden SSE replay**: recorded raw byte streams under each adapter's `testdata/*.sse`; tests assert the assembled turn and `Usage` against golden JSON, regenerated with a `-update` flag (one golden mechanism across all adapters; re-running `-update` on unchanged fixtures must produce no diff).
- An **injected unexported clock** makes retry backoff and JSONL timestamps deterministic.
- **Architectural seams to preserve**: orchestration is pure above the provider SPI (`Provider` / `Request` / `RoundTrip`); consumers and dependent packages are consumed **only through their public interfaces**; `internal/*` packages carry no consumer-facing surface; tool-call IDs stay in the strict charset.

**Coverage convention (defined here — design mints ids, downstream measures them).** A Verification id counts as **covered** only when it is named in a `// R-XXXX-XXXX` comment inside a test file (`*_test.go`) on a test that **genuinely asserts the behavior** the id describes — never a bare string literal, never a comment with no real assertion behind it. Coverage is therefore a grep: every covered id appears as `// R-XXXX-XXXX` next to real test code. A purely structural/seam phase that realizes a Decision carrying no ids is proven by the build staying green plus any integration smoke the phase names.

**The "done" bar for a phase.** A phase is done when **every Verification id of the Decisions it realizes** (its slice of any shared/cross-provider id, per the phase's "Done when" line) is covered as above **and the suite is green**. Some ids are shared across phases (the error matrix, usage mapping, model/pricing registries, reasoning-`Opaque` capture, generation-settings mapping, `R-C8UE-VJ67`): a phase covers only its own slice, named in its "Done when" line — do not try to discharge another phase's slice.

## Scope of one turn

Build **exactly one phase**: the **first phase still marked `⬜ not started`** in `docs/plan.md`, in document order. One phase is one package's worth of work in **one accumulating context** — no per-item sub-loop, no fresh context per behavior. Build the whole package and all its tests in this single turn, then stop and report.

If a phase genuinely feels too large to complete in one context, **halt and report it as a design problem** (via a `DONE` status naming the phase) — do **not** chop the work finer, do not partially complete it and mark it done.

## What to read (and what not to)

Read, this turn:
- The **first `⬜ not started` phase** entry in `docs/plan.md` — the Decisions it realizes and its exact "Done when" id list.
- In `docs/design.md`: those **Decisions** and their **Verification** lists (the ids and the behaviors).
- The **public interfaces only** of the packages this phase depends on (already built in earlier phases) — enough to consume them correctly.
- `docs/product.md` **only** if intent is genuinely ambiguous.

Do **not** read: the internals of dependency packages, unrelated phases, or Decisions this phase does not realize.

## Procedure

1. **Build the package** against the Decisions it realizes, consuming every dependency **only through its public interface** — never reaching past another package's interface or into `internal/*` from outside.
2. **Cover every Verification id** in the phase's "Done when" line with a genuine, clearly-named test, and tag each with its id in the coverage-comment form `// R-XXXX-XXXX`, so coverage is a grep. A pure structural/seam phase carries no ids and is proven by the build staying green plus any integration smoke it names.
3. **Hold the global invariant**: leave `go build ./...`, `go vet ./...`, `gofmt -l .`, and `go test ./...` all clean across the whole module.
4. **Honor the seams**: don't leak internals into a public interface, don't reach past another package's interface, keep adapters behind the injected-client/base-URL seam and timing behind the injected clock.
5. **Flip only this phase's status marker** in `docs/plan.md` from `⬜ not started` to `✅ done` — never rewrite phase text, never touch other phases.
6. **Commit** the change (source, tests, and the plan marker flip together) with a message naming the phase, e.g. `Phase 3 — Pricing and cost engine`.

## Empowerment

You are empowered to decide and keep moving. Resolve ambiguity with sensible, conventional choices that fit the design — idiomatic Go, the established naming and seams, the existing patterns from earlier phases. The harness is unattended: **default to progress over questions.** Halt only for a genuine blocker or a choice that would clearly contradict the product goals or the design's settled shape.

## Boundaries

- Do **not** edit `docs/product.md` or `docs/design.md`. If building reveals the design's shape is wrong (an interface that can't work, a phase that can't be completed in one context, a contradiction between Decisions), **halt and report via `DONE`** naming the problem — do not silently "fix" it in code or docs.
- Build **only what the phase names** — only the Decisions it realizes and only the ids in its "Done when" line. No pulling work forward from later phases, no gold-plating beyond the Verification ids.
- When a detail is merely **ambiguous** (not a design flaw), consult `docs/design.md`, make the conventional choice, and proceed.

## Report status (the loop contract)

End your final message with **exactly one** JSON object and **nothing after it**:

```json
{"status": "CONTINUE", "message": "<one short sentence>"}
```

Choose `status` by re-reading `docs/plan.md` **after** marking this phase `✅ done`:
- **`CONTINUE`** — at least one phase is still `⬜ not started`.
- **`DONE`** — no phase remains `⬜ not started` (the build is complete), **or** you are halting on a genuine blocker that is really a design change. When halting, emit `DONE` with a `message` naming the blocker so the run stops for a human.

Never loop forever and never falsely claim completion. `message` is one short sentence: the phase just built and what comes next, or the blocker.
