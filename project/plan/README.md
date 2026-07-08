# AgentKit — Plan

**Authority: construction order and history.** This document owns the order AgentKit is built in and the record of what has been built. Unlike product (`project/product/README.md`) and design (`project/design/README.md`), which are rewritten in place to stay authoritative for the current state, the plan is **append-only**: phases are added at the bottom and marked done as they land; completed phases are never rewritten or deleted, so the plan doubles as the construction history. To extend the project later, update product and design in place, then append a new phase here.

**One phase = one package = one build-turn context.** Each phase is a single coherent unit — almost always one package — scoped to that unit's design Decisions and the *interfaces* (not internals) of the packages it depends on, and **sized so the build loop can carry it in one fresh build-turn context**. The loop does *not* build a phase in one long accumulating context — each phase is sized to a single build turn, which is what keeps every phase the size of a small standalone tool no matter how large the project grows. Because the architecture is one large root `agentkit` package plus leaf sub-packages, the root work is split across several phases (it exceeds one context); each sub-package is its own phase. Some verification ids are table-driven or cross-provider (the error matrix, usage mapping, model registries, reasoning-`Opaque` capture, generation-settings mapping, and `R-C8UE`): each contributing phase names and covers its own provider's slice, and the id is fully discharged when its last contributing phase lands.

**Done bar.** A phase is **done** when every Verification id in the design Decisions it realizes (its slice of any shared id) is covered by a clearly-named test and the suite is green — measured against the per-Decision **Verification** lists in `project/design/README.md` (minted `R-XXXX-XXXX` ids, one behavior each). This bar is deterministic — a green suite plus id-coverage, never a subjective judgment and never a self-referential check.

**Coverage invariant.** Every design Verification id *currently in* `project/design/` appears in the build plan — realized by a phase. The plan is the denominator's superset, not its equal: verify by direction, **design ⊆ plan**, not set-equality. Because finished phases are frozen and the plan is append-only, an id minted later can only be covered by a newly appended phase, so coverage of every current id must be right at authoring time. Mechanically, the set difference *design − plan* must be empty:

```sh
comm -23 \
  <(grep -hoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/design/*.md | sort -u) \
  <(grep -hoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/plan/phase-*.md | sort -u)
```

**Known exceptions (frozen-history residue).** The reverse difference (*plan − design*) is **not** required to be empty: a frozen phase may legitimately name things that no longer exist in the current spec, because it records what was done at the time and is never rewritten. Two such residues exist today:

- **Retired ids** `R-C7MI-HRFI` and `R-P71Z-J46O` — deleted from design when their behaviors went (the D2 message-granular rework; a never-defined citation), but preserved in the frozen phases that recorded their removal (`phase-05`, `phase-24`, `phase-26`). They appear in the plan but not in design, and are excluded from the coverage check above.
- **Removed analysis docs** — frozen phases `phase-26` and `phase-34` … `phase-43` cite `project/archive/…` and `project/audit/…` as their historical provenance. Those out-of-contract folders have since been removed; the citations remain in the frozen phases as inert historical record, and the build loop never re-reads a `✅` phase.

## Layout

The plan is split for addressability so the build loop never loads the whole history to find its next unit of work:

- **`project/plan/STATUS.md`** — the manifest: one grep-able Markdown-bullet line per phase (`- Phase NN …`), carrying its status marker (`⬜`/`✅`) and the design Decision(s) it realizes. This is the **only** place a phase's status marker lives. The loop finds the next phase with `grep -nE '^- Phase .* ⬜' project/plan/STATUS.md | head -1`.
- **`project/plan/phase-NN.md`** — one file per phase (zero-padded, `phase-01.md` … `phase-48.md`). It holds that phase's body — the *Realizes design Decision … Depends on …* line, the objective and observable end state, and the *Done when* `R-XXXX-XXXX` id list. The loop reads exactly one per turn. A phase body carries no status marker of its own.
- **`project/plan/README.md`** (this file) — the invariant rules above. Static; it does not grow with the project.

**Append-only, restated for this layout:** never rewrite or delete a `phase-NN.md`; never delete a line in `STATUS.md`. The only mutation during a build is flipping one phase's `⬜ → ✅` in `STATUS.md`. New work = a new `phase-NN.md` plus a new `STATUS.md` line, both appended at the end.
