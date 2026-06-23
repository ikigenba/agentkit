# Phase 33 — Release: patch version bump to `v0.1.3`

*Realizes — (no design Decision; a release action carries no Verification ids). Depends on Phase 31 (shared retry executor) and Phase 32 (`ToolSchemaLimiter`).*

Cut the patch release that bundles the two maintenance fixes from D21 and D22. The version is carried by git tags (`v0.1.0`/`v0.1.1`/`v0.1.2` exist; `v0.1.2` is current); there is no version constant in code to edit, and `docs/product.md`'s "starting version `v0.1.0`" is a historical contractual constant that stays untouched.

End state:

- The commit on which Phases 31 and 32 are both green is tagged `v0.1.3`.
- No source change is part of this phase beyond the tag (a `CHANGELOG` entry may be added if and only if the repo already keeps one; none exists today, so none is required).

**Done when:** the `v0.1.3` tag exists on the green post-Phase-32 commit and the full suite is green. (This phase has no `R-XXXX-XXXX` ids; its acceptance is the tag plus a green build.)
