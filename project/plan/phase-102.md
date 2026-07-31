# Phase 102 — Changelog and the v0.14.0 minor release

*Realizes design Decisions 36 and 37 (release record; no Verification ids of its own — the behavior is proven by Phases 100 and 101). Depends on Phase 100 and Phase 101.*

The change is consumer-visible (two new toolkit tools and the module's first
external dependency), so it ships as a **minor** version bump under the `v0`
pre-stable surface: `v0.13.0` → **`v0.14.0`**.

`CHANGELOG.md` gains a `## v0.14.0` section at the top describing, in
consumer-observable terms: the toolkit's new `WebSearch` tool (web search via
the Brave Search API on a consumer-supplied key, with count/freshness/locale/
safe-search/result-type controls and clean, bounded JSON results) and `WebFetch`
tool (fetching an `http`/`https` page as readable markdown, plain text verbatim,
binaries refused, per-call adjustable timeout); that neither joins `All`, which
still returns exactly the six local coding tools; and that the module now
depends on `golang.org/x/net` for HTML parsing.

The release is then cut on `main` as an annotated tag, per the repository's
versioning rule (annotated git tags only; no `VERSION` file, no version
constant).

**Done when:**
- `grep -c '^## v0.14.0' CHANGELOG.md` returns exactly 1.
- `grep -n '^## v0' CHANGELOG.md | head -2` lists `v0.14.0`, then `v0.13.0`, in that order.
- `git tag -l v0.14.0` outputs `v0.14.0`, and `git cat-file -t "$(git rev-parse v0.14.0)"` outputs `tag` (annotated, not lightweight).
- `git tag --sort=-v:refname | head -1` outputs `v0.14.0`.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
