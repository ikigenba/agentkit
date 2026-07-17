# AgentKit — Plan

**Authority: construction order.** This document and the `project/plan/` directory own the order AgentKit is built in — a work **queue** of dependency-ordered **pending** phases only. Unlike product (`project/product/README.md`) and design (`project/design/README.md`), which are rewritten in place to stay authoritative for the current state, the plan holds only unbuilt work: **completion is deletion.** When a phase passes, the build loop removes its `STATUS.md` line and `git rm`s its `phase-NN.md` in the completion commit, so a finished phase never lingers to contradict a design that has since moved on. There is no `✅` state on disk; done is deleted. Construction history lives in **git** (the completion commits, and the deleted phase files recoverable there), never in the spec. To extend the project later, update product and design in place, then **append** a new phase here — numbered from the `STATUS.md` counter, never renumbering or reusing a number.

**One phase = one package = one build-turn context.** Each phase is a single coherent unit — almost always one package — scoped to that unit's design Decisions and the *interfaces* (not internals) of the packages it depends on, and **sized so the build loop can carry it in one fresh build-turn context**. The loop does *not* build a phase in one long accumulating context — each phase is sized to a single build turn, which is what keeps every phase the size of a small standalone tool no matter how large the project grows. Because the architecture is one large root `agentkit` package plus leaf sub-packages, the root work is split across several phases (it exceeds one context); each sub-package is its own phase. Some verification ids are table-driven or cross-provider (the error matrix, usage mapping, model registries, reasoning-`Opaque` capture, generation-settings mapping): each contributing phase names and covers its own provider's slice, and the id is fully discharged when its last contributing phase lands.

**Done bar.** A phase is **done** when every Verification id in the design Decisions it realizes (its slice of any shared id) is covered by a clearly-named test and the suite is green — measured against the per-Decision **Verification** lists in the design Decision files (minted `R-XXXX-XXXX` ids, one behavior each). This bar is deterministic — a green suite plus id-coverage, never a subjective judgment and never a self-referential check.

**Coverage invariant.** Every design Verification id *currently in* `project/design/` is either already **realized** — its id appearing verbatim as a tag in a test file that runs under the suite — or assigned to **exactly one** pending phase: no current id unassigned, none split, none duplicated across pending phases. Design (rewritten in place, the current statement) is the denominator; realized-ness is read from the **code itself** (the tagged tests), never from a ledger or a history. There is no reverse bookkeeping: when a behavior leaves design its id and tagged test are deleted with it, and a *pending* phase carrying an id design no longer mints is stale and must be fixed at authoring time. Mechanically, no current design id may be missing from the union of tagged tests and pending phases — the design-only difference must be empty:

```sh
comm -23 \
  <(grep -hoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/design/*.md | sort -u) \
  <(cat <(grep -rhoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' --include='*_test.go' --exclude-dir=project .) \
        <(grep -hoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/plan/phase-*.md 2>/dev/null) | sort -u)
```

Empty output is the pass condition. (The `--exclude-dir=project` matters — an id quoted in the spec is not a test.)

## Layout

The plan is split for addressability so the build loop never loads the whole queue to find its next unit of work:

- **`project/plan/STATUS.md`** — the manifest: the `Next phase: NN` counter plus one grep-able Markdown-bullet line per **pending** phase (`- Phase NN …`), carrying its pending marker (`⬜`) and the design Decision(s) it realizes. This is the **only** place a phase's status marker lives. The loop finds the next phase with `grep -nE '^- Phase .* ⬜' project/plan/STATUS.md | head -1`.
- **`project/plan/phase-NN.md`** — one file per **pending** phase (zero-padded; sub-phases keep their suffix, e.g. `phase-07a.md`). It holds that phase's body — the *Realizes design Decision … [Depends on …]* line, the objective and observable end state, and the *Done when* `R-XXXX-XXXX` id list. The loop reads exactly one per turn. A phase body carries no status marker of its own.
- **`project/plan/README.md`** (this file) — the invariant rules above. Static; it does not grow with the project.

**Completion is deletion, restated for this layout:** the build loop's only mutations are removing a finished phase's `STATUS.md` line together with its `phase-NN.md`; the `Next phase` counter is never decremented and never touched by the loop. New work = a new `phase-NN.md` plus a new `STATUS.md` line, both appended at the end and numbered from the counter.
