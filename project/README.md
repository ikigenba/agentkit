# agentkit/project — workspace layout

Everything AgentKit (`github.com/ikigenba/agentkit`) needs to be **designed,
planned, and built** lives under `project/`. This file is the only loose file
here; everything else is in one of the folders below. Paths throughout the spec
are written relative to the **repo root**, which is also the directory the
`ralph` build loop runs from.

This repo is spec-driven: the source of truth is this `project/` tree, and the
application code (the root `agentkit` package, the provider sub-packages, and
`internal/`) is written by the unattended build loop, not by hand. To change
behavior you change the spec (via a `/*-mode` command) and append a plan phase;
the loop implements it.

## The folders

| folder | what's in it | owned by |
|---|---|---|
| `product/` | `product.md` — the *why*: problem, users, scope, user-facing promises, success criteria | `/product-mode` (rewritten in place) |
| `research/` | `research.md` — the design-informing background; non-contractual, nothing downstream reads it | `/research-mode` (rewritten in place) |
| `design/` | `design.md` (spine) + `INDEX.md` (manifest + sorted `R-id → Decision` map) + `DNN.md` (one per Decision) | `/design-mode` (rewritten in place) |
| `plan/` | `plan.md` (rules) + `STATUS.md` (the manifest — the only home of each phase's `⬜`/`✅` marker) + `phase-NN.md` (one per phase) | `/plan-mode` (append-only) |
| `prompts/` | the `ralph` build-loop prompts `gather.md`, `build.md`, `verify.md` (+ the ephemeral `brief.md`) | `/create-gather-build-verify-prompts` (generated) |
| `archive/` | superseded analysis snapshots (`audit.md`, `macro-design-eval.md`) kept as historical references | free-form (not mode-owned) |

The four **spine documents** (`product/product.md`, `research/research.md`,
`design/design.md`, `plan/plan.md`) are each singular and owned by a `/*-mode`
command — that command is the sanctioned way to change them. Don't add ad-hoc
documents to the spine folders; fold corrections and follow-ons into the existing
spine docs via the mode commands (and append a plan phase) instead. The
`archive/` notes are informal historical snapshots and are not mode-owned.

## The build loop

`ralph` is the autonomous executor. It runs **from this repo root** and is handed
the full paths to the three prompt files — the names and locations are this
project's convention (documented here); `ralph` itself assumes nothing about them:

```sh
ralph project/prompts/gather.md project/prompts/build.md project/prompts/verify.md
```

It cycles the prompts in fresh, isolated contexts — `gather → build → verify → …` —
on a **two-status** contract: each prompt ends with one JSON object whose `status`
is either `NEXT` (advance to the next prompt, wrapping `verify → gather`) or `DONE`
(stop). `CONTINUE` is unused. State lives entirely in the workspace (the git tree,
`project/plan/STATUS.md`, and the ephemeral `project/prompts/brief.md`) — never in
the agent's memory between turns.

| step | reads | writes | commits | flips marker | returns |
|---|---|---|---|---|---|
| **gather** | the big docs (STATUS, one phase, its Decisions) | `project/prompts/brief.md` | no | no | `NEXT`, or `DONE` if no `⬜` |
| **build** | `project/prompts/brief.md` only | code + co-located tests | the increment | no | `NEXT` |
| **verify** | the brief + the suite | deletes `project/prompts/brief.md` | only a marker flip (on pass) | yes (pass only) | `NEXT` |

- **gather** — the only step that opens `project/plan/` or `project/design/`. It
  greps `STATUS.md` for the first `⬜` phase; if there is none it returns `DONE`
  (the sole end of the loop). Otherwise it resolves that phase's Decision(s) via
  `project/design/INDEX.md` and writes a tiny, self-contained
  `project/prompts/brief.md`, then returns `NEXT`.
- **build** — never opens the big docs. It consumes only the brief — including the
  dependency interface signatures copied into it — does a bounded, idempotent turn
  of the remaining work, writes id-tagged tests, commits, and leaves the marker
  `⬜`.
- **verify** — the independent gate and only step that flips a marker. It re-runs
  the suite and checks that every id is covered by a genuinely-asserting test. Pass
  → flip that one `⬜ → ✅` and commit the flip. Gap → leave `⬜`, change nothing.
  **Either way it deletes the brief** and returns `NEXT`.

### Why it is human-free and converges

`verify` can neither halt the loop nor advance a phase on a gap — its only powers
are "flip this phase green" (on full proof) or "leave it red." So an incomplete or
wrongly-built phase simply stays `⬜`, and the next cycle re-gathers and re-attacks
it with a fresh context. The loop ends only when **every** phase is verified green
(`gather` finds no `⬜` and returns `DONE`) — or when a ralph budget rail
(`--max-iterations/-time/-spend/-tokens`) trips. The marker is the sole completion
signal and only verify, only on proof, ever moves it.

## The `project/prompts/brief.md` contract

The brief is the **seam** that keeps build's context tiny — the complete and only
input build and verify consume, so neither opens design or plan. It is:

- **ephemeral** — created by gather, deleted by verify; it exists only between them;
- **never committed** — `project/prompts/brief.md` is in `.gitignore`;
- **single-phase** — overwritten fresh every cycle.

### Schema

```markdown
# Brief — Phase <NN>

> Ephemeral. Written by gather, consumed by build then verify, deleted by verify.
> Never committed. Describes exactly one phase; overwritten fresh each cycle.

## Phase
<NN> — <one-line objective, copied from project/plan/phase-NN.md>

## Realizes
D<n>[, D<n>...]            (the design Decision ids this phase realizes; "—" if structural)

## Decision files
- project/design/D0N.md
[... one per realized Decision]

## Ids to cover
- R-XXXX-XXXX — <the behavior, one line>
[... one per Verification id this phase owns]
(or the literal line "(none — structural phase)" when the phase mints no ids)

## Files to touch
- <pkg>/<file>.go
- <pkg>/<file>_test.go
[... the package + test files build will create or modify]

## Dependency interfaces
The public signatures build consumes from the packages this phase depends on,
copied here so build never opens another doc. Signatures only — no bodies.

(```go ... ``` block of exported type / function signatures of dependencies)

## Done bar
<every id under "Ids to cover" tagged in an asserting *_test.go test AND the
suite green; or — for a structural phase — the build green plus the named
integration smoke.>
```

`grep -oE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/prompts/brief.md` lists the ids;
`grep -nE '^Phase .* ⬜' project/plan/STATUS.md | head -1` finds the next phase;
`grep -n R-XXXX-XXXX project/design/INDEX.md` resolves an id to its Decision file.

## Project conventions the prompts inline

These originate in design's *Conventions* (`project/design/design.md`) — the
prompts copy them verbatim so each turn is self-contained.

- **Toolchain.** Go 1.26, module `github.com/ikigenba/agentkit`. The `agentkit`
  package lives at the module **root**, built across several phases. Leaf provider
  sub-packages: `anthropic/`, `openai/`, `zai/`, `google/`. Non-importable shared
  internals under `internal/`: `internal/httpx`, `internal/sse`,
  `internal/openaicompat`, `internal/mcp`, `internal/retry`. Public symbols carry
  no package-name stutter (`agentkit.Conversation`, not `agentkit.AgentKitState`).
- **"The suite is green" means all four hold:** `go build ./...` exits 0,
  `go vet ./...` exits 0, `go test ./...` exits 0, and `gofmt -l .` prints
  **nothing**. The default `go test ./...` is offline and deterministic and spends
  no API credits; integration tests are `//go:build integration`-tagged and
  excluded by default.
- **Coverage convention.** A Verification id counts as **covered** only when it
  appears in a `// R-XXXX-XXXX` comment inside a `_test.go` file on a test that
  **genuinely asserts** that behavior (`grep -rn "R-XXXX-XXXX" --include=*_test.go`).
  Tests are **co-located** with the code they exercise and **named for the
  behavior** — never gathered into a per-phase or root-level test file. Some ids
  are **shared across phases** (the error matrix, usage mapping, the
  model/pricing/reasoning-spec registries, generation-settings mapping); a phase
  covers only its own slice, named in its `**Done when:**` line. A structural
  phase carries no ids and is proven by the green build plus any integration smoke
  it names.
- **Determinism seams (honor, do not bypass).** The single adapter test seam is
  the injected `*http.Client` + base URL via `WithBaseURL(string)` /
  `WithHTTPClient(*http.Client)`, with unit tests pointing the adapter at an
  `httptest.Server` replaying recorded fixtures; **golden SSE replay** uses
  recorded raw byte streams under each adapter's `testdata/*.sse`, asserting the
  assembled turn and `Usage` against golden JSON regenerated with a `-update` flag;
  an **injected unexported clock** makes retry backoff and JSONL timestamps
  deterministic; orchestration stays pure above the provider SPI
  (`Provider` / `Request` / `RoundTrip`); dependent packages are consumed only
  through their public interfaces; `internal/*` carries no consumer-facing surface;
  tool-call IDs stay in the strict charset.
- **Commits.** Build commits each increment (`Phase NN: …`); verify commits only
  the one-line marker flip on a pass. Both end the message with the
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` trailer.
