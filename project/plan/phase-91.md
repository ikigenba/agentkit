# Phase 91 — Anthropic: strict mode and schema rendering

*Realizes a slice of design Decision 22 (the anthropic renderer: R-XT52-U8YG, R-2UV8-RBKS). Depends on Phase 90.*

The `anthropic` adapter stops passing tool schema bytes through verbatim and renders them into its dialect at request-build time. Each tool is sent with `strict: true` as a sibling of `name`/`description`/`input_schema`. The renderer walks the whole schema and sets `"additionalProperties": false` on the root object **and every nested object** — Anthropic rejects a nested object that omits it — while emitting no `$schema` and no `$defs`. Optional properties stay omitted from `required`, which Anthropic accepts as-is. The renderer also rewrites `oneOf` to `anyOf` (Anthropic rejects `oneOf`, research §18.3) and inlines `$ref` against `$defs`.

**Done when:**
- `R-XT52-U8YG` — against the fake-server harness, the recorded request body shows `strict: true` per tool, `"additionalProperties": false` on the root and on every nested object, optional properties absent from `required`, and no `$schema` or `$defs` key at any depth.
- `R-2UV8-RBKS` — a construct-complete canonical schema arrives with every subset construct present: constraint keys verbatim, `oneOf` as `anyOf`, the nullable union as a type list, `$ref` inlined.
- `go build ./...` and `go test ./...` both exit 0.
