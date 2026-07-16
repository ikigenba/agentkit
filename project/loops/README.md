# agentkit build loop — `ralph` gather / build / verify

The prompts in this folder are the **installed build loop**: they are generated
from the finished `project/` spec and describe the loop topology that drives the
unattended build. They are not spec artifacts — the spec is authoritative for
*what* to build; these prompts are the machinery that builds it. This README
documents the installed loop; `gather.md`, `build.md`, and `verify.md` are the
prompts themselves.

## Running it

Run the loop from the repo root with the executable wrapper:

```sh
project/loops/run
```

`project/loops/run` is a one-line wrapper around the autonomous executor `ralph`;
its contents are exactly:

```sh
#!/bin/bash

exec ralph project/loops/gather.md project/loops/build.md project/loops/verify.md
```

`ralph` runs **from the repo root** (its working directory), so every workspace
path the prompts reference is repo-root-relative (`project/…`). The prompt names
and locations are this project's convention (documented here); `ralph` itself
assumes nothing about them.

## The status contract

`ralph` cycles the prompts in fresh, isolated contexts — `gather → build →
verify → gather → …` — on a **two-terminal-status** contract. The **final**
message of each turn carries a `status`:

- `NEXT` — **terminal**: advance to the next prompt, wrapping `verify → gather`.
- `DONE` — **terminal**: stop the loop. **Only `gather`** ever reports it, and
  only when no `⬜` phase remains. `build` and `verify` **never** report `DONE`:
  even closing a phase's last gap with a green suite is still `NEXT` for them.
- `CONTINUE` — **non-terminal**: the status a streaming model (e.g. gpt-5.5
  under codex, which coerces *every* streamed message into the schema) tags the
  progress messages it emits **before** its terminal message. It never
  terminates a turn or drives the loop; `ralph` reads only the last message and
  advances on the terminal `NEXT`/`DONE` alone.

`ralph` injects the `{status, message}` schema per backend (codex via
`--output-schema` writing the final message to `-o`; claude via `--json-schema`
surfaced as a synthetic `StructuredOutput` tool) and reads it back itself — the
prompts describe only the contract, never a transport. State lives entirely in
the workspace (the git tree, `project/plan/STATUS.md`, and the ephemeral
`project/loops/brief.md`), never in the agent's memory between turns.

## Per-step reads / writes / commits / flips

| step | reads | writes | commits | flips marker | returns |
|---|---|---|---|---|---|
| **gather** | the big docs (STATUS, one phase, its Decisions) — only when authoring a fresh brief | the brief's **contract region** (or nothing, if a brief for this phase is already in flight) | no | no | `NEXT`, or `DONE` if no `⬜` |
| **build** | `project/loops/brief.md` only (contract + feedback) | code + co-located tests | the increment | no | `NEXT` |
| **verify** | the brief + the suite | the brief's **feedback region** (on a gap), or deletes the brief (on pass/stall) | only a marker flip (on pass) | yes (pass only) | `NEXT` |

- **gather** — the only step that opens `project/plan/` or `project/design/`. It
  greps `STATUS.md` for the first `⬜` phase; if there is none it returns `DONE`
  (the sole end of the loop). **If a brief for that same phase already exists it
  leaves it untouched** (the phase is mid-flight — its contract and any `verify`
  feedback are preserved) and returns `NEXT` without opening a big doc. Only when
  no brief exists, or the existing one is for a different (now-`✅`) phase, does it
  read that one `phase-NN.md`, resolve its Decision(s) via `INDEX.md`, read only
  those `DNN.md`, and write a fresh self-contained brief.
- **build** — never opens the big docs. It consumes only the brief — including
  the design prose and dependency interface signatures copied into it, and the
  `## Verify feedback` region (open gaps first). It does a bounded, idempotent
  turn of the remaining work — ideally the whole phase — writes id-tagged tests
  co-located with the code, commits, and leaves the marker `⬜`.
- **verify** — the independent gate and only step that flips a marker. It
  re-derives current truth from scratch, re-runs the suite, and checks that every
  id is covered by a genuinely-asserting, actually-reachable test. **Pass** → flip
  that one `⬜ → ✅`, commit the flip, and delete the brief. **Gap** → leave `⬜`,
  change no source, and overwrite the brief's feedback region with only the
  currently-open gaps (so the brief persists for the next `build`). **Stall** →
  after 3 consecutive no-progress attempts on the same gaps, log it and discard
  the brief so `gather` rebuilds it fresh.

## The brief lifecycle

`project/loops/brief.md` is the **seam** that keeps build's context tiny — the
complete and only input build and verify consume, so neither opens design or
plan. It is **phase-scoped, not per-cycle**:

- **gather** authors the contract region **once**, when a phase first becomes the
  active `⬜` phase, and **no-ops** (leaves the file untouched) on every later
  cycle while that phase stays `⬜`.
- **build** consumes it every cycle, addressing any feedback gaps first, and
  never writes it.
- **verify** either **passes** (flip `⬜ → ✅`, delete the brief) or records
  **gaps** (overwrite the feedback region, keep the brief). The brief therefore
  **persists across cycles** until the phase passes or a stall reset discards it.

It is **never committed** — `project/loops/brief.md` is in `.gitignore` — and
**single-phase**: it only ever describes one phase at a time.

## Why the loop converges

`verify` can neither halt the loop nor advance a phase on a gap — its only powers
are "flip this phase green" (on full, reachable proof) or "leave it `⬜`, with the
open gaps recorded." So an incomplete phase simply stays `⬜`, and the next cycle
re-attacks it — now with `verify`'s grounded feedback in front of `build`, and
without `gather` re-reading the big docs (it no-ops on the in-flight brief). The
persisted feedback also gives `verify` cross-cycle memory: it distinguishes *slow
convergence* (the open-gap id set shrinking or changing) from a *true stall* (the
**same** gap ids unsatisfied for ≥3 consecutive attempts with **no new build
commit**). On a true stall it does a **trajectory reset** — discards the brief so
`gather` rebuilds the contract fresh — which stays inside the "verify never halts,
never advances on a gap" invariant. The only exit is still `gather → DONE`, which
requires zero `⬜` markers — so the run ends only when every phase is verified
green, or a ralph budget rail (`--max-iterations/-time/-spend/-tokens`) trips. The
marker is the sole completion signal, and only `verify`, only on proof, ever moves
it.

## The `project/loops/brief.md` schema

Two regions, **one writer each**: the contract region (gather-owned) and the
`## Verify feedback` region (verify-owned). Neither writer touches the other's.

````markdown
# Brief — Phase <NN>

> Ephemeral. Contract written by gather; consumed by build then verify. The
> `## Verify feedback` region is written by verify. Persists across cycles while
> the phase stays ⬜; deleted by verify on pass or on a stall reset. Never
> committed (it is gitignored). Describes exactly one phase.

## Phase
<NN> — <one-line objective, copied from project/plan/phase-NN.md>

## Realizes
D<n>[, D<n>...]            (or "—" if structural)

## Decision files
- project/design/D0N.md
[... one per realized Decision]

## Design prose
### D<n> — <title>
<the realized Decision's full design prose — Decision statement, shape/signatures,
and Rejected alternatives — copied verbatim from its DNN.md, with that Decision's
Verification list omitted>
[... one block per realized Decision]

## Ids to cover
R-XXXX-XXXX — <full requirement text copied verbatim from the Decision's Verification list>
[... one id per line; id at line-start, an em-dash, then the requirement prose]
(or the single literal line "(none — structural phase)")

## Files to touch
- <pkg>/<file>.go
- <pkg>/<file>_test.go
[... unit tests co-located with the code they exercise; never a per-phase or
root-level test file]

## Dependency interfaces
```go
<exported type / function signatures of the packages this phase depends on>
```

## Done bar
<every id above tagged in a genuinely-asserting, actually-reachable *_test.go test
AND the suite green; each id's proving suite invocation named (default offline
`go test ./...`, or `go test -tags integration ./...` with credentials for a
live id); a structural phase → green build + the named integration smoke>

## Verify feedback — attempt <N>
- Build commit observed: <sha or none>
- Stall streak: <k>
- Open gaps:
  - R-XXXX-XXXX — <exact failing command> → <observed output> (file:line)
  [... only the currently-open gaps; "(none — not yet verified)" in a fresh brief]
````

Useful greps:

- `grep -nE '^- Phase .* ⬜' project/plan/STATUS.md | head -1` — the next phase.
- `grep -oE '^R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/loops/brief.md` — the id
  denominator (anchored at line-start, so ids quoted in prose don't miscount).
- `grep -n R-XXXX-XXXX project/design/INDEX.md` — resolve an id to its Decision.
- `grep -rn "R-XXXX-XXXX" --include=*_test.go` — the coverage check.

## Project conventions the prompts inline

These originate in design's *Conventions* (`project/design/README.md`) and this
project's layout — the prompts copy them verbatim so each turn is self-contained.

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
  excluded by default, run under `go test -tags integration ./...` with provider
  credentials when a phase's done bar names a live id.
- **Coverage convention.** A Verification id counts as **covered** only when it
  appears in a `// R-XXXX-XXXX` comment inside a `_test.go` file on a test that
  **genuinely asserts** that behavior **and actually runs under the suite's real
  invocation** — a test held out by an unset build tag or an unsatisfiable skip,
  or one that turns a real failure into a `SKIP`, is **uncovered**. A skip is
  never acceptable green for a requirement. Tests are **co-located** with the code
  they exercise and **named for the behavior** — never gathered into a per-phase
  or root-level test file; the few cross-package integration tests live only in
  their designated `//go:build integration` home. Some ids are **shared across
  phases** (the error matrix, usage mapping, the model/pricing/reasoning-spec
  registries, generation-settings mapping); a phase covers only its own slice,
  named in its `**Done when:**` line. A structural phase carries no ids and is
  proven by the green build plus any integration smoke it names.
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
- **Commits.** Build commits each increment (`Phase NN — …`); verify commits only
  the one-line marker flip on a pass (`Phase NN — verified`). Both end the message
  with the `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
  trailer. `gather` commits nothing.

`project/README.md` (the workspace map) only points here; loop mechanics and the
brief schema live nowhere but this file. `project/loops/brief.md` is listed in
`.gitignore`.
