# Phase 88 — Document the v0.11.0 release

*Realizes design Decision — (structural/docs). Depends on Phase 87.*

`CHANGELOG.md` at the repository root gains a `## v0.11.0` section, in the
file's existing style, summarizing this release: catalog completeness (every
listed offering carries full pricing, reasoning spec, and context — the blank
secondary-offering tier is gone), OpenRouter offerings for every tracked chat
model with documented-and-confirmed reasoning vocabularies and audited rates,
and the per-offering wire-name override for the three dotted Anthropic slugs.
The operator cuts the annotated tag (`git tag -a v0.11.0 -m "v0.11.0"`) after
this phase completes; tagging is not part of the phase.

**Done when:**
- `grep -c '^## v0.11.0' CHANGELOG.md` reports exactly 1.
- The suite is green: `go build ./...` and `go test ./...` exit 0.
