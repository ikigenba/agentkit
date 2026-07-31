# Phase 111 — Changelog and the v0.16.0 minor release

*Realizes design Decision 27, Decision 36, and Decision 37 (release record; no Verification ids of its own — the behavior is proven by Phase 110). Depends on Phase 110.*

The change is consumer-visible and breaking on two signatures, so it ships as a
**minor** version bump under the `v0` pre-stable surface: `v0.15.0` →
**`v0.16.0`**.

`CHANGELOG.md` gains a `## v0.16.0` section at the top describing, in
consumer-observable terms: that every `toolkit` tool constructor now takes the
same shape, its required arguments followed by optional `toolkit.Option` values;
that `toolkit.WithBaseURL` lets a consumer point `WebSearch` at an address other
than Brave's, so a consumer can finally drive its own search wiring to a real
result against a server it controls in its own tests, or route through a proxy;
that `toolkit.WithHTTPClient` lets a consumer supply the HTTP client either web
tool uses, putting timeouts, proxies, TLS trust, and traffic recording in the
consumer's hands and removing the module's last two uninjectable network calls;
that supplying no options preserves today's behavior exactly, Brave's address
and a bounded ten-second wait; and that an empty base URL is reported as
`ErrInvalidConfig` naming the base URL, distinct from a missing key. The two
breaking signature changes are named: `toolkit.WebSearch` and `toolkit.WebFetch`
now take variadic options, as do the six local tool constructors.

The release is then cut on `main` as an annotated tag, per the repository's
versioning rule (annotated git tags only; no `VERSION` file, no version
constant).

**Done when:**
- `grep -c '^## v0.16.0' CHANGELOG.md` returns exactly 1.
- `grep -n '^## v0' CHANGELOG.md | head -2` lists `v0.16.0`, then `v0.15.0`, in that order.
- `git tag -l v0.16.0` outputs `v0.16.0`, and `git cat-file -t "$(git rev-parse v0.16.0)"` outputs `tag` (annotated, not lightweight).
- `git tag --sort=-v:refname | head -1` outputs `v0.16.0`.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
