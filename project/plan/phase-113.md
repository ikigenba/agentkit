# Phase 113 — Drop strict from the wire and render optionality honestly

*Realizes design Decision 22 (per-provider schema rendering). Depends on Phase 112.*

The dependency is not incidental: Phase 112 must land first so the runtime validator is enforcing the schema before the wire stops carrying `strict`. Reversing the order would leave a commit range in which nothing enforces a tool's declared input shape on any provider.

What exists at the end of this phase:

- The Anthropic adapter no longer sets `strict` on a tool. The field is **absent** from the emitted tool object, not present-and-false. Everything else its renderer does is unchanged: `additionalProperties: false` injected at every object level, `$ref` inlined, `oneOf` rewritten to `anyOf`, no `$schema` or `$defs`, tools sorted by name.
- The OpenAI adapter no longer sets `strict`, and its renderer no longer rewrites optionality. A property that is optional in the canonical schema is now omitted from `required` and rendered with its plain type, instead of being moved into `required` and wrapped in a nullable union. An authored nullable union is still rendered as a type list. `zai` and `openrouter` inherit the change through `internal/openaicompat` with no per-package work.
- The Google adapter is untouched; Gemini never had a strict mode and its renderer already rendered optionality verbatim.
- The construct-complete fixture for the OpenAI renderer is updated to the new expectation. The Anthropic and Google fixtures keep asserting what they already assert.
- Every test asserting the retired behavior (`strict: true` on the wire, or OpenAI's all-properties-required rewrite) is deleted along with its id tag. Decision 22 no longer mints those ids, so the reverse-coverage check below locates them precisely: it lists exactly the tags left in the codebase that design has dropped, and each one names a test to delete.

**Done when:** `go build ./...` and `go test ./...` both exit 0 with no failing package; the reverse-coverage check

```
comm -13 <(grep -hoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/design/*.md | sort -u) \
         <(grep -rhoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' --include='*_test.go' --exclude-dir=project . | sort -u)
```

produces **empty output**, proving no test still tags an id design has retired; and each of the following ids is carried by a clearly-named test tagged with the bare id in a `*_test.go` file:

- R-80QS-7XKB — the Anthropic tool object carries **no `strict` key at all**, with `additionalProperties: false` on the root and every nested object, optional properties omitted from `required`, and no `$schema` or `$defs`.
- R-836K-ZH1P — the OpenAI tool object carries **no `strict` key at all**, with a canonically-optional property omitted from `required` and carrying its plain type (not a nullable union), `additionalProperties: false` on every object, and no `$schema` or `$defs`; the identical payload results through `internal/openaicompat` for `zai` and `openrouter`.
- R-84EH-D8SE — the OpenAI renderer fed the construct-complete canonical schema records every construct, with only `oneOf`→`anyOf` and `$ref` inlining as rewrites, the authored nullable union kept as a type list, and the canonically-optional property absent from `required` with its plain type.
