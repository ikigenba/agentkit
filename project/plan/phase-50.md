# Phase 50 — Release: patch version bump to `v0.2.1`

*Realizes — (no design Decision; a release action carries no Verification ids). Depends on Phase 49 (group-name loading).*

Cut the patch release that ships group-name acceptance in `load_tools` (the amended D23) — a backward-compatible widening of accepted input to an existing tool, so the bump is patch. The version is carried by git tags per `AGENTS.md`; there is no version constant in code to edit.

End state:

- The commit on which Phase 49 is green is tagged `v0.2.1` (annotated, per `AGENTS.md`).
- No source change is part of this phase beyond the tag.

**Done when:** the `v0.2.1` tag exists on the green post-Phase-49 commit and the full suite is green. (This phase has no `R-XXXX-XXXX` ids; its acceptance is the tag plus a green build.)
