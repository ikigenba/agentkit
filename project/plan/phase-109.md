# Phase 109 — Changelog and the v0.15.0 minor release

*Realizes design Decision 38 (release record; no Verification ids of its own — the behavior is proven by Phases 103–108). Depends on Phase 103, Phase 104, Phase 105, Phase 106, Phase 107, and Phase 108.*

The change is consumer-visible and breaking on two signatures, so it ships as a
**minor** version bump under the `v0` pre-stable surface: `v0.14.0` →
**`v0.15.0`**.

`CHANGELOG.md` gains a `## v0.15.0` section at the top describing, in
consumer-observable terms: that anything needing a credential can now be built
without one and reports which credential is missing when it is used, rather than
panicking or failing at wiring time, so a tool set no longer changes shape with
whichever keys are set; that a credential which is real but cannot serve its
operation — a ChatGPT subscription handed to embeddings — is reported as exactly
that instead of as a missing credential; that `agentkit.ErrMissingCredential` is
the new sentinel for the first case and `ErrInvalidConfig` continues to serve the
second; and the two breaking signature changes,
`toolkit.WebSearch(toolkit.BraveAPIKey)` in place of a bare `string`, and
`openai.NewEmbedder` no longer panicking on a subscription credential.

The release is then cut on `main` as an annotated tag, per the repository's
versioning rule (annotated git tags only; no `VERSION` file, no version
constant).

**Done when:**
- `grep -c '^## v0.15.0' CHANGELOG.md` returns exactly 1.
- `grep -n '^## v0' CHANGELOG.md | head -2` lists `v0.15.0`, then `v0.14.0`, in that order.
- `git tag -l v0.15.0` outputs `v0.15.0`, and `git cat-file -t "$(git rev-parse v0.15.0)"` outputs `tag` (annotated, not lightweight).
- `git tag --sort=-v:refname | head -1` outputs `v0.15.0`.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
