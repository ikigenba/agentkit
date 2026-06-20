# LOOP — the gather → build → verify build loop

This file documents how **AgentKit** (`github.com/ikigenba/agentkit`) is built unattended. It is the human/author-facing overview and the home of the `docs/brief.md` contract. The actual per-turn instructions live in three prompt files — `docs/gather.md`, `docs/build.md`, `docs/verify.md` — each read by the harness in a **fresh context** every turn. This file is **not** itself a prompt and is never read by the loop; the three prompt files are self-contained.

## The harness

The loop is driven by [`ralph`](../../ralph), which runs a **sequence** of prompt files as clean-context turns, cycling through them until a prompt reports `DONE` or a budget rail is crossed:

```sh
ralph -c sandbox_mode=workspace-write docs/gather.md docs/build.md docs/verify.md
```

`ralph` knows nothing about AgentKit. It owns only the lifecycle and the budget; the prompts own the work. The only contract is the **status** each prompt returns as the last line of its final message:

```json
{"status": "NEXT", "message": "<one short sentence>"}
```

This project uses exactly **two** of ralph's statuses:

- **`NEXT`** — advance to the next prompt in the sequence (wrapping past `verify` back to `gather`).
- **`DONE`** — the whole job is complete; stop. **Only `gather` ever returns `DONE`**, and only when no `⬜` phase remains.

`CONTINUE` is deliberately unused. Each prompt does one bounded turn and hands off; the cycle itself is the unit of progress.

## The state machine

State lives entirely in the workspace — never in the model's memory between turns:

- **`docs/plan/STATUS.md`** — the manifest. One line per phase carrying a `⬜`/`✅` marker. The **only** place a phase's status lives. A phase is `✅` only after `verify` confirms it.
- **`docs/brief.md`** — the ephemeral per-phase contract (schema below). **gitignored, never committed.** Created by `gather`, consumed by `build` then `verify`, deleted by `verify`.
- The source tree and git history — the actual built code.

| prompt | reads | writes | returns |
|---|---|---|---|
| **gather** | `STATUS.md`, one `phase-NN.md`, `INDEX.md`, the realized `DNN.md`(s), dependency interfaces | a fresh `docs/brief.md` | `NEXT` (a `⬜` phase exists) / `DONE` (none left) |
| **build** | **only** `docs/brief.md` | source + id-tagged tests; commits the code | `NEXT` (always) |
| **verify** | `docs/brief.md`, runs the suite | flips `⬜→✅` on pass; **deletes the brief** | `NEXT` (always) |

Why it is **human-free** and **converges**:

- `gather` is the **only** prompt that reads the big docs (design/plan/product). `build` and `verify` never open them — `build`'s context stays tiny because the brief carries everything it needs.
- `verify` **can never halt and can never advance a phase on a gap.** A green suite + full id coverage flips the marker to `✅`; anything short leaves it `⬜`. Either way it returns `NEXT`, wrapping to `gather`.
- `gather` re-selects the **same** `⬜` phase every cycle until `verify` flips it, then moves to the next. The loop physically cannot exit with an open gap, because the only exit (`gather → DONE`) requires zero `⬜` markers.
- An incomplete phase is therefore **never** a stop condition — it is simply a `⬜` the loop keeps returning to until the gap is closed. The run ends only when every phase is `✅`.

The only things that end a run are `gather`'s `DONE` (success) or ralph's **budget rails** — `--max-iterations` / `--max-time` / `--max-spend` / `--max-tokens` (exit 3) and `--max-retries` (exit 4). Set those deliberately at launch: they are the unattended-spend backstop so a genuinely unclosable gap cannot loop forever, **not** a logic stop.

## The `docs/brief.md` contract

The brief is the seam that keeps `build`'s context small: it is the **complete and only** input `build` and `verify` consume, so neither ever touches design or plan. It is:

- **Ephemeral** — created fresh by `gather` each cycle, deleted by `verify`. It exists only in the window between a successful `gather` and the `verify` that consumes it.
- **Never committed** — listed in `.gitignore`. Its create/delete never appears in git.
- **Single-phase** — it describes exactly one phase (the first `⬜`). It is overwritten, not appended.

`gather` writes it to **this exact shape**. `build` and `verify` parse it; the *Ids to cover* list is grep-able (`grep -oE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' docs/brief.md`).

```markdown
# Brief — Phase <NN>

> Ephemeral. Written by gather, consumed by build then verify, deleted by verify.
> Never committed. Describes exactly one phase; overwritten fresh each cycle.

## Phase
<NN> — <one-line objective, copied from docs/plan/phase-NN.md>

## Realizes
D<n>[, D<n>...]            (the design Decision ids this phase realizes; "—" if structural)

## Decision files
- docs/design/D0N.md
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
<the explicit completion condition: every id under "Ids to cover" tagged in an
asserting *_test.go test AND the suite green; or — for a structural phase — the
build green plus the named integration smoke.>
```

## Project conventions (authoritative facts the prompts inline)

These are constant across phases and are repeated inside each prompt that needs them, so each turn is self-contained. They originate in design's *Conventions* (`docs/design.md`) — do not assume, these are the real commands.

- **Language / layout:** Go 1.26; module `github.com/ikigenba/agentkit`. Package `agentkit` lives at the module **root** and is built across several phases. Leaf provider sub-packages: `anthropic/`, `openai/`, `zai/`, `google/`. Non-consumer-importable shared internals under `internal/`: `internal/httpx`, `internal/sse`, `internal/openaicompat`, `internal/mcp`. Public symbols carry no package-name stutter (`agentkit.Conversation`, not `agentkit.AgentKitState`).
- **"The suite is green" means all four hold:** `go build ./...` exits 0, `go vet ./...` exits 0, `go test ./...` exits 0, and `gofmt -l .` prints **nothing**. The default `go test ./...` is offline and deterministic and spends no API credits; integration tests are `//go:build integration`-tagged and excluded by default.
- **Coverage convention:** a Verification id counts as **covered** only when it appears in a `// R-XXXX-XXXX` comment inside a `_test.go` file on a test that **genuinely asserts** that behavior. Coverage is a grep: `grep -rn "R-XXXX-XXXX" --include=*_test.go`. Some ids are **shared across phases** (the error matrix, usage mapping, the model/pricing/reasoning-spec registries, generation-settings mapping); a phase covers only its own slice, named in its `**Done when:**` line. A structural phase carries no ids and is proven by the green build plus any integration smoke it names.
- **Determinism seams (honor, do not bypass):** the single adapter test seam is the injected `*http.Client` + base URL via `WithBaseURL(string)` / `WithHTTPClient(*http.Client)`, with unit tests pointing the adapter at an `httptest.Server` replaying recorded fixtures; **golden SSE replay** uses recorded raw byte streams under each adapter's `testdata/*.sse`, asserting the assembled turn and `Usage` against golden JSON regenerated with a `-update` flag (one mechanism, idempotent on unchanged fixtures); an **injected unexported clock** makes retry backoff and JSONL timestamps deterministic; orchestration stays pure above the provider SPI (`Provider` / `Request` / `RoundTrip`); dependent packages are consumed only through their public interfaces; `internal/*` carries no consumer-facing surface; tool-call IDs stay in the strict charset.
- **Commit trailer:** end every commit message body with
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
