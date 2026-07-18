# agentkit/project — workspace map

Everything AgentKit (`github.com/ikigenba/agentkit`) needs to be **designed,
planned, and built** lives under `project/`. This file is the only loose file
here; everything else is in one of the folders below. Paths throughout the spec
are written relative to the **repo root** — the directory this `project/` tree
sits at the root of, and the directory the `ralph` build loop runs from. The
`project/` spec governs **only this codebase** (the root `agentkit` package, the
provider sub-packages, and `internal/`); it never reaches a sibling service or
shared tooling.

This repo is spec-driven: the source of truth is this `project/` tree, and the
application code is written by the unattended build loop, not by hand. To change
behavior you update the spec (product and design rewritten in place, a new plan
phase appended), then the loop implements it.

## The folders

| folder | what's in it | written by |
|---|---|---|
| `product/` | `README.md` — the *why*: problem, users, scope, user-facing promises, success criteria | `$seal-spec` (rewritten in place) |
| `research/` | `research.md` — the design-informing background; non-contractual, nothing downstream reads it | `$seal-spec` (rewritten in place) |
| `design/` | `README.md` (spine) + `INDEX.md` (manifest + sorted `R-id → Decision` map) + `DNN.md` (one per Decision) | `$seal-spec` (rewritten in place) |
| `plan/` | `README.md` (rules) + `STATUS.md` (the manifest: `Next phase` counter + the only home of each pending phase's `⬜` marker) + `phase-NN.md` (one per **pending** phase) | `$seal-spec` (appends); the build loop deletes completed phases |
| `loops/` | the installed `ralph` build-loop prompts `gather.md`, `build.md`, `verify.md` (+ the ephemeral `brief.md`), and `README.md` describing the loop | the prompt-generator workflow (`create-gather-build-verify-prompts`) |

The four **spine documents** (`product/README.md`, `research/research.md`,
`design/README.md`, `plan/README.md`) are each singular and authored by
`$seal-spec` — that is the sanctioned way to change them. Product, research,
and design are the single *current* statement, rewritten in place; the plan is
a work **queue** of pending phases only — completed phases are deleted, and
construction history lives in git. Don't add ad-hoc documents to the spine
folders; fold corrections and follow-ons into the existing spine docs and append
a plan phase.

The `loops/` prompts and `loops/README.md` are **not** spec artifacts — they are
generated from the finished spec and describe whichever loop topology is
installed. For how the build loop actually runs — the `ralph` invocation, the
`gather → build → verify` contract, and the `brief.md` seam — see
[`loops/README.md`](loops/README.md).
