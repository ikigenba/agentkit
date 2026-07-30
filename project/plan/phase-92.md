# Phase 92 — OpenAI (and the compat providers): strict mode and schema rendering

*Realizes a slice of design Decision 22 (the openai renderer: R-XUCZ-80P5, R-2W35-53BH). Depends on Phase 90.*

The `openai` adapter renders tool schemas into OpenAI's strict dialect and sends `strict: true` on each function. Because OpenAI forbids optional properties, the renderer moves **every** key of `properties` into `required` at every level and renders each canonically-optional property as a two-branch nullable union (`"type": ["T","null"]`). It sets `"additionalProperties": false` on every object, root and nested, and emits no `$schema` or `$defs`. The renderer also rewrites `oneOf` to `anyOf` (OpenAI rejects `oneOf`, research §18.2) and inlines `$ref` against `$defs`.

`internal/openaicompat` carries the same renderer so `zai` and `openrouter` inherit it unchanged: neither publishes a keyword contract, and OpenRouter's effective dialect depends on the routed upstream (research §18.5), so emitting the narrowest well-defined form is the only defensible choice.

**Done when:**
- `R-XUCZ-80P5` — against the fake-server harness, the recorded request body shows `strict: true` per tool, every `properties` key present in `required` at every level, each canonically-optional property rendered as a two-branch nullable union, `"additionalProperties": false` on every object, and no `$schema` or `$defs` key.
- `R-2W35-53BH` — a construct-complete canonical schema arrives with every subset construct present modulo the documented rewrites: all properties in `required`, optionals as nullable unions, `oneOf` as `anyOf`, `$ref` inlined, constraint keys verbatim; the identical tool payload results through `internal/openaicompat`.
- The same assertions hold for a `zai` and an `openrouter` request built through `internal/openaicompat`.
- `go build ./...` and `go test ./...` both exit 0.
