# Phase 93 — Google: the allowlist renderer

*Realizes a slice of design Decision 22 (the google renderer: R-XVKV-LSFU) and a slice of Decision 13 (google's live tool-call cell: R-Y5C2-NYDE). Depends on Phase 90.*

`google`'s `convertSchema` is rewritten as a strict allowlist over Gemini's documented `Schema` fields. Gemini 400s on any key that is not a proto field (research §18.1), so an unrecognized construct must be structurally unreachable rather than filtered after the fact.

The renderer inlines `$ref` against `$defs`, maps `oneOf` to `anyOf`, rewrites `const: v` to `enum: [v]`, renders a nullable property as a single `type` plus `nullable: true` (Gemini rejects type unions), and emits no `additionalProperties`, `$schema`, `$defs`, or `$ref` at any depth. The fabrications the old converter performed are removed: it no longer synthesizes `items: {"type":"STRING"}` for an array without `items`, and no longer collapses an unrecognized node to `{"type":"OBJECT"}` — a schema reaching the renderer is already subset-valid, so those paths are unreachable and their absence is the point.

Google's live tool-call cell in D13's uniform suite, previously filled by the deleted `R-9UK4-JI3L`, is restored as the standard presence-gated live tool-call test.

**Done when:**
- `R-XVKV-LSFU` — against the fake-server harness, the recorded `parameters` contains no `additionalProperties`, `$schema`, `$defs`, or `$ref` key at any depth; `$ref` targets appear inlined; `oneOf` appears as `anyOf`; `const: v` appears as `enum: [v]`; a nullable property appears as a single `type` plus `nullable: true`.
- `R-Y5C2-NYDE` — the `integration`-tagged live test gated on `GEMINI_API_KEY`/`GOOGLE_API_KEY` completes a full tool-call round trip to a non-error assembled message, and skips cleanly when the key is absent.
- `go build ./...` and `go test ./...` both exit 0.
