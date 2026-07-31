# AgentKit — Design

**Authority: shape and its proof.** This document owns *how* AgentKit is built — its seams, public interfaces, naming, struct/type definitions, data model — and *how each behavior is proven* by tests. `project/product/README.md` owns the *why*, the users, the scope, and the user-facing promises; this document never re-declares the why. Design *uses* the product's contractual constants by value (module path `github.com/ikigenba/agentkit`, starting version `v0.1.0`, minimum Go 1.26) but does not own them. This is the single, current statement of the architecture: when a decision changes, this doc is rewritten in place to stay true — stale decisions are removed, not stacked. Construction history lives in **git**, never in the spec; the plan holds only pending work.

## Requirement ids

- Each Decision ends with a **Verification** list: the concrete behaviors that decision requires.
- Every Verification item carries a **minted id** of the form `R-XXXX-XXXX`, minted with `idgen -p R` — never hand-written or reused.
- One id, one behavior, in exactly one place. The ids live inline in these Verification lists and nowhere else — there is no separate requirements document.
- When the design is rewritten in place, existing ids are never renumbered; a newly added behavior gets a fresh id, and a removed behavior takes its id with it.

## Conventions

- **Language/module.** Go 1.26; module `github.com/ikigenba/agentkit`; package `agentkit` at the module root. Public symbols are named so their purpose is clear from the name alone, with no package-name stutter (`agentkit.Conversation`, not `agentkit.AgentKitState`).
- **Concurrency stance.** A `*Conversation` is one conversation owned by one goroutine; it is not safe for concurrent use and does no internal locking (cf. `sql.Rows`). Documented, not enforced.
- **Credentials.** Always supplied explicitly by the consumer, as a typed value from the owning package's closed credential set; AgentKit never reads environment variables, files, or any credential store on its own. The one scoped exception is the opt-in `openai/subscription` helper (D25), which reads and maintains exactly the auth file the consumer names by path. An **absent** credential never panics and never fails construction — the operation that needs it reports which credential is missing (D38 owns the rule and its errors).
- **Dependencies.** External modules require **explicit per-case human approval**; the standard library is the default. Approved today: **`golang.org/x/net`** (the `html` package only — the Go team's HTML5 tokenizer/parser, operator-approved 2026-07-30 for `toolkit.WebFetch`'s HTML→markdown conversion, D37); nothing else. `github.com/invopop/jsonschema` was approved for struct→JSON-Schema derivation in `NewTool` and is now removed: D4 reflects into the canonical subset (D34) directly, because the library's output required undoing on every axis that mattered (`additionalProperties`, `$defs` hoisting, `$schema`, `patternProperties` for maps). Provider SDKs, the MCP `go-sdk`, and backoff libraries are **not** approved — every provider adapter and the MCP client are raw `net/http` over `internal/httpx`. Adding a module is an operator decision, never a build-loop one.
- **Toolchain.** Build/typecheck with `go build ./...`; vet with `go vet ./...`; run tests with `go test ./...`. **"The suite is green"** means `go build ./...` and `go test ./...` both exit 0 with no failing package. Requirement-id tags live in Go test files matching the glob **`*_test.go`**, as a comment carrying the bare id (e.g. `// R-DQ16-AQWE`) inside or immediately above the test that asserts that behavior. Integration tests that require live credentials are additionally gated behind the `integration` build tag and skip when their env vars are absent, so the default `go test ./...` stays hermetic and green offline.

## Layout

The design is split for addressability so the build loop never loads the whole
architecture to find the one Decision a phase realizes:

- **`project/design/INDEX.md`** — the manifest: each Decision mapped to its file and
  the Verification ids it owns, plus a sorted `R-id → Decision/file` reverse map.
  Id lookup is a grep against this file (or against the Decision files directly).
- **`project/design/DNN.md`** — one file per Decision (zero-padded; referenced in
  prose and the plan as `D<N>`). Numbering is not contiguous — there is no `D14`
  (a real gap; numbers are never reused). Each file is self-contained: the
  Decision, its public interfaces/types, the rejected alternatives, and its
  **Verification** list of `R-XXXX-XXXX` ids. The build loop reads only the
  Decision(s) its phase realizes.
- **`project/design/README.md`** (this file) — the invariant spine above: Authority, the
  *Requirement ids* convention, and *Conventions*. Static cross-cutting facts; it
  does not carry per-Decision detail.

Design is **rewritten in place**, not append-only (the construction history lives
in git): when a Decision changes, its `DNN.md` is rewritten to stay true and
`INDEX.md` is regenerated. A new Decision adds a `DNN.md` and an INDEX entry.
