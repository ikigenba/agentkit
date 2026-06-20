# AgentKit — Plan

**Authority: construction order and history.** This document owns the order AgentKit is built in and the record of what has been built. Unlike product (`docs/product.md`) and design (`docs/design.md`), which are rewritten in place to stay authoritative for the current state, the plan is **append-only**: phases are added at the bottom and marked done as they land; completed phases are never rewritten or deleted, so the plan doubles as the construction history. To extend the project later, update product and design in place, then append a new phase here.

**One phase = one package = one accumulating context.** Each phase is a single coherent unit — almost always one package — built in one accumulating context against product and design, reading only that package's design Decisions and the *interfaces* (not internals) of the packages it depends on. This is what keeps every phase the size of a small standalone tool no matter how large the project grows. Because the architecture is one large root `agentkit` package plus leaf sub-packages, the root work is split across several phases (it exceeds one context); each sub-package is its own phase. Some verification ids are table-driven or cross-provider (the error matrix, usage mapping, model registries, reasoning-`Opaque` capture, generation-settings mapping, and `R-C8UE`): each contributing phase covers its own provider's slice, and the id is fully discharged when its last contributing phase lands.

**Done bar.** A phase is **done** when every Verification id in the design Decisions it realizes (its slice of any shared id) is covered by a clearly-named test and the suite is green — measured against the per-Decision **Verification** lists in `docs/design.md` (minted `R-XXXX-XXXX` ids, one behavior each).

## Layout

The plan is split for addressability so the build loop never loads the whole history to find its next unit of work:

- **`docs/plan/STATUS.md`** — the manifest: one grep-able line per phase, carrying its status marker (`⬜`/`✅`) and the design Decision(s) it realizes. This is the **only** place a phase's status marker lives. The loop finds the next phase by grepping for the first `⬜`.
- **`docs/plan/phase-NN.md`** — one file per phase (zero-padded, `phase-01.md` … `phase-23.md`). It holds that phase's body — the *Realizes design Decision … Depends on …* line, the objective and observable end state, and the *Done when* `R-XXXX-XXXX` id list. The loop reads exactly one per turn.
- **`docs/plan.md`** (this file) — the invariant rules above. Static; it does not grow with the project.

**Append-only, restated for this layout:** never rewrite or delete a `phase-NN.md`; never delete a line in `STATUS.md`. The only mutation during a build is flipping one phase's `⬜ → ✅` in `STATUS.md`. New work = a new `phase-NN.md` plus a new `STATUS.md` line, both appended at the end.
