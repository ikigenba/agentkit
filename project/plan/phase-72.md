# Phase 72 — Release: minor version bump to v0.7.0

*Realizes — (structural). Depends on Phase 71.*

The coding toolkit ships as `v0.7.0` — a minor, additive release. The changelog gains a `v0.7.0` entry documenting the new `toolkit` subpackage: the six standard coding tools (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `Grep`) as ready-made `agentkit.Tool` values via per-tool constructors and `All(root)`; symlink-aware root confinement for the file tools (with the stated shell caveat); the 30,000-character result cap; `Edit`'s ambiguity refusal; `Bash`'s timeout and process-group kill; recursive `**` globbing; and `.git`/binary skipping in search.

**Done when:** `git tag` lists `v0.7.0` and the tagged commit builds green; `grep -n "## v0.7.0" CHANGELOG.md` matches exactly once.
