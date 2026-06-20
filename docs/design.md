# AgentKit — Design

**Authority: shape and its proof.** This document owns *how* AgentKit is built — its seams, public interfaces, naming, struct/type definitions, data model — and *how each behavior is proven* by tests. `docs/product.md` owns the *why*, the users, the scope, and the user-facing promises; this document never re-declares the why. Design *uses* the product's contractual constants by value (module path `github.com/ikigenba/agentkit`, starting version `v0.1.0`, minimum Go 1.26) but does not own them. This is the single, current statement of the architecture: when a decision changes, this doc is rewritten in place to stay true — stale decisions are removed, not stacked. The history of how the design got here lives in the plan.

## Requirement ids

- Each Decision ends with a **Verification** list: the concrete behaviors that decision requires.
- Every Verification item carries a **minted id** of the form `R-XXXX-XXXX`, minted with `idgen -p R` — never hand-written or reused.
- One id, one behavior, in exactly one place. The ids live inline in these Verification lists and nowhere else — there is no separate requirements document.
- When the design is rewritten in place, existing ids are never renumbered; a newly added behavior gets a fresh id, and a removed behavior takes its id with it.

## Conventions

- **Language/module.** Go 1.26; module `github.com/ikigenba/agentkit`; package `agentkit` at the module root. Public symbols are named so their purpose is clear from the name alone, with no package-name stutter (`agentkit.Conversation`, not `agentkit.AgentKitState`).
- **Concurrency stance.** A `*Conversation` is one conversation owned by one goroutine; it is not safe for concurrent use and does no internal locking (cf. `sql.Rows`). Documented, not enforced.
- **Credentials.** Always supplied explicitly by the consumer; AgentKit never reads environment variables, files, or any credential store on its own.

## Layout

The design is split for addressability so the build loop never loads the whole
architecture to find the one Decision a phase realizes:

- **`docs/design/INDEX.md`** — the manifest: each Decision mapped to its file and
  the Verification ids it owns, plus a sorted `R-id → Decision/file` reverse map.
  Id lookup is a grep against this file (or against the Decision files directly).
- **`docs/design/DNN.md`** — one file per Decision (zero-padded; referenced in
  prose and the plan as `D<N>`). Numbering is not contiguous — there is no `D14`
  (a real gap; numbers are never reused). Each file is self-contained: the
  Decision, its public interfaces/types, the rejected alternatives, and its
  **Verification** list of `R-XXXX-XXXX` ids. The build loop reads only the
  Decision(s) its phase realizes.
- **`docs/design.md`** (this file) — the invariant spine above: Authority, the
  *Requirement ids* convention, and *Conventions*. Static cross-cutting facts; it
  does not carry per-Decision detail.

Design is **rewritten in place**, not append-only (the construction history lives
in the plan): when a Decision changes, its `DNN.md` is rewritten to stay true and
`INDEX.md` is regenerated. A new Decision adds a `DNN.md` and an INDEX entry.
