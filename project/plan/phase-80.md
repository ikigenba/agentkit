# Phase 80 — release v0.9.0

*Realizes no Decision — a release phase. Depends on Phase 79 (the changelog must describe the shipped split-cache surface).*

Documents the release in `CHANGELOG.md`, following the shape of the existing entries: a `## v0.9.0` section at the top of the list, below the intro, written in past tense from the consumer's point of view, describing observable surface rather than internal mechanics.

The minor version is the breaking slot pre-1.0, and this release earns it twice over even though `ocr.Tool`'s signature is unchanged and existing code still compiles:

- **The returned transcript path moved.** `ocr.Tool` now returns a path under `<root>/ocr/`, not one under the consumer's cache directory. A consumer that stored, logged, or post-processed the old path sees a different value.
- **The cache layout changed and every existing entry is invalidated.** Entries are now a single `<stem>-<hash8>.json` per document rather than a `<stem>-<hash8>/` directory holding `response.json` and `transcript.md`. Old entries are not read and not migrated; the first call for each document after upgrading re-extracts and re-bills it. Say this plainly, since it costs money.
- **What the change buys:** the cache directory can now sit outside the agent's working directory and stay durable across runs, while the transcript still lands somewhere the agent's own file tools can open. Under the old layout those two were mutually exclusive.
- **A cached response that no longer derives is an error, not a silent re-extraction**, so a corrupt or unreadable cache entry surfaces instead of quietly re-billing the document.

Structural phase: it adds no code and no Verification ids, so its acceptance is a deterministic file check rather than an id list.

**Done when** all of the following hold:

- `CHANGELOG.md` contains a line matching `^## v0\.9\.0` (`grep -cE '^## v0\.9\.0' CHANGELOG.md` returns 1).
- That section documents both halves of the break: `sed -n '/^## v0\.9\.0/,/^## v0\.8\.0/p' CHANGELOG.md | grep -qE '\bocr\b' && sed -n '/^## v0\.9\.0/,/^## v0\.8\.0/p' CHANGELOG.md | grep -qiE 'cache'`.
- The previous entry is untouched: `grep -cE '^## v0\.8\.0' CHANGELOG.md` still returns 1.
- The suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions).

Tagging is **not** part of this phase. Versions are annotated git tags cut by the operator on `main` (`git tag -a v0.9.0 -m "v0.9.0"`); the build loop never creates one.
