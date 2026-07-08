# Phase 48 — Release: minor version bump to `v0.2.0`

*Realizes — (no design Decision; a release action carries no Verification ids). Depends on Phase 46 (deferred tools) and Phase 47 (the live integration proof).*

Cut the minor release that ships the deferred-tools capability (D23) — a new backward-compatible surface (`DeferredToolGroup`, `Conversation.DeferredTools`, the synthesized `load_tools`), so the bump is minor, not patch. The version is carried by git tags per `AGENTS.md`; there is no version constant in code to edit, and `project/product/README.md`'s "starting version `v0.1.0`" is a historical contractual constant that stays untouched.

End state:

- The commit on which Phases 46 and 47 are both green is tagged `v0.2.0` (annotated, per `AGENTS.md`).
- No source change is part of this phase beyond the tag.

**Done when:** the `v0.2.0` tag exists on the green post-Phase-47 commit and the full suite is green. (This phase has no `R-XXXX-XXXX` ids; its acceptance is the tag plus a green build.)
