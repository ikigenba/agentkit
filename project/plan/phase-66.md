# Phase 66 — Release: patch version bump to v0.4.1

*Realizes — (structural). Depends on Phase 65.*

The reworked login flow ships as `v0.4.1`. The changelog gains a `v0.4.1` entry documenting the change: the interactive `Login`/`LoginIO` surface removed, replaced by the value-in/value-out `BeginLogin`/`Flow.AuthorizeURL`/`Flow.Complete` flow, with the library doing no terminal IO and a clearer error for an empty or unparsable pasted redirect URL. While in the changelog, the existing mislabeled heading is corrected: the entry describing the v0.4.0 release (credential constructors, free-flow, OpenRouter, subscription auth, catalog) currently reads `## v0.3.0` and becomes `## v0.4.0`.

**Done when:** `git tag` lists `v0.4.1` and the tagged commit builds green; `grep -n "## v0.4.1" CHANGELOG.md` matches exactly once; `grep -c "## v0.3.0" CHANGELOG.md` reports 0; `grep -n "## v0.4.0" CHANGELOG.md` matches exactly once.
