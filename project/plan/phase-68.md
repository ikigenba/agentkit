# Phase 68 — Release: minor version bump to v0.5.0

*Realizes — (structural). Depends on Phase 67.*

The reworked subscription credential handling ships as `v0.5.0` — a breaking release. The changelog gains a `v0.5.0` entry documenting the break: the `openai/subscription` login flow (`BeginLogin`/`Flow`/`AuthorizeURL`/`Complete`) removed with login now the job of an external OAuth login tool; the credential file format changed from the codex-CLI wrapper shape to the raw token-endpoint response, with the account id derived from the `https://api.openai.com/auth` JWT claim; refresh rewrites preserving `refresh_token`/`id_token` when a response omits them.

**Done when:** `git tag` lists `v0.5.0` and the tagged commit builds green; `grep -n "## v0.5.0" CHANGELOG.md` matches exactly once.
