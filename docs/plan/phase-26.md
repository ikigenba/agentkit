# Phase 26 — Fail-loud `default` in adapter block translation; retire the orphan verification id

*Realizes design Decision 9 (the provider adapter seam — the newly-minted fail-loud id R-4YSE-6YBS). Depends on Phase 08/09/10/11 (the four provider adapters whose outbound block-translation switches this hardens) and Phase 01 (the closed `Block` set in D3). Bundles two trivial audit corrections — Findings 17 and 2 — that share the adapter test surface.*

Two independent audit cleanups, no behavior change for any in-set block:

**Finding 17 — fail-loud `default` on an unknown block type.** Three adapter switches that lower the closed consumer `Block` set to a provider wire shape currently lack a `default`, so a future block type a `case` forgets would be **silently dropped** from the wire request (no compile error, no runtime signal — the exact silent-corruption failure mode D9's replay-shape traps already warn about). The three translation switches are:
- `anthropic/anthropic.go` — `convertMessage` (the `switch b := block.(type)` building `[]wireBlock`).
- `openai/openai.go` — the input-item builder's `switch block := block.(type)` (around `partsFromMessage`/the text-and-tool-part loop).
- `google/google.go` — `partsFromMessage` (the `switch block := block.(type)` building `[]map[string]any`).

Each gains a `default` that **panics** with a descriptive message (`fmt.Sprintf("unknown block type %T", block)` or equivalent), per D9's stance: the `Block` set is a sealed library-internal interface, so an unhandled case is a programming error to crash on loudly, not a returned `error` to thread (two of the three functions have no error channel). The non-translating estimator `anthropic/anthropic.go` `stablePrefixTokens` is **explicitly exempt** — it only approximates cache-prefix token counts, so an unrecognized block must contribute nothing rather than crash a live turn; leave its switch without a `default` (or add a no-op one), and do not add a panic there.

**Finding 2 — retire orphan id `R-P71Z-J46O`.** This id is cited as a "verification" but was never defined in the design (absent from `docs/design/INDEX.md`); the behavior its citing tests assert (generation/reasoning settings reaching the request) is already covered by the co-cited *defined* ids `R-P5U3-5CFZ`, `R-T40A-VZQ7`, `R-ELUQ-VJIQ`, so removing the citation loses no coverage. Delete the `R-P71Z-J46O` token from every artifact that cites it:
- test comments: `anthropic/anthropic_test.go`, `openai/openai_test.go`, `zai/zai_test.go`, `google/google_test.go`;
- Done-when lists: `docs/plan/phase-08.md`, `phase-09.md`, `phase-10.md`, `phase-11.md` (drop the `, R-P71Z-J46O` token only — leave every other id in those completed-phase lists untouched).

After removal, `grep -rn "R-P71Z-J46O" .` returns only this file and `docs/audit.md` (the finding's own record).

**Done when:** R-4YSE-6YBS is covered — a clearly-named test per adapter drives a synthetic out-of-set `Block` (a private type implementing the `Block` marker) through that adapter's request build and asserts it panics with the `unknown block type` message via `recover`; the three translation switches each carry the panicking `default`, while `stablePrefixTokens` does not; `R-P71Z-J46O` is removed from the four adapter test files and the four phase Done-when lists (no citation remains outside this file and `docs/audit.md`); all existing D9/D6/D16 ids remain green; `go build ./...`, `go vet ./...`, and the suite are green per design Conventions.
