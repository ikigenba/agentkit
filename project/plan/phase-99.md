# Phase 99 — Changelog and the v0.13.0 minor release

*Realizes design Decision 35 (release record; no Verification ids of its own — the behavior is proven by Phases 96–98). Depends on Phase 96, Phase 97, and Phase 98.*

The behavior change is consumer-visible (a Gemini reply that previously arrived
as many `TextBlock`s now arrives as one), so it ships as a **minor** version
bump under the `v0` pre-stable surface: `v0.12.0` → **`v0.13.0`**.

`CHANGELOG.md` gains a `## v0.13.0` section at the top describing, in
consumer-observable terms: a completed assistant message's visible answer now
arrives as a single `TextBlock` on every provider, Google included, rather than
one block per SSE frame; a Gemini `thoughtSignature` split from its
`functionCall` across frames now binds correctly, so reasoning replay for that
call is no longer silently lost; and the adjacent-text invariant is now enforced
centrally for every adapter.

`CHANGELOG.md` also gains the **missing `## v0.12.0` section** — the tag exists
but the file jumps from `v0.11.0` to nothing, so the record has a hole. Its
contents are reconstructed from the commits between `v0.11.0` and `v0.12.0`
(`git log v0.11.0..v0.12.0`), written in the same consumer-facing register as
the surrounding sections. It is placed below `v0.13.0` and above `v0.11.0`.

The release is then cut on `main` as an annotated tag, per the repository's
versioning rule (annotated git tags only; no `VERSION` file, no version
constant).

**Done when:**
- `grep -c '^## v0.13.0$' CHANGELOG.md` returns exactly 1.
- `grep -c '^## v0.12.0$' CHANGELOG.md` returns exactly 1.
- `grep -n '^## v0' CHANGELOG.md | head -3` lists `v0.13.0`, then `v0.12.0`,
  then `v0.11.0`, in that order.
- `git tag -l v0.13.0` outputs `v0.13.0`, and
  `git cat-file -t "$(git rev-parse v0.13.0)"` outputs `tag` (annotated, not
  lightweight).
- `git tag --sort=-v:refname | head -1` outputs `v0.13.0`.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
