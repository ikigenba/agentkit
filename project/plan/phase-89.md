# Phase 89 — AgentKit's own schema reflector, and the end of `RawTool`

*Realizes design Decision 4 (tool definition & registration surface).*

The root `agentkit` package derives a tool's input schema itself, targeting the canonical subset (D34) directly, and drops `github.com/invopop/jsonschema`.

`NewTool[In]` reflects `In` into a schema that contains only subset constructs: nested structs inlined (no `$defs`, no `$ref`), no `$schema`, and no `additionalProperties` — that key is adapter-owned (D22) and must never be authored. A field is required exactly when its `json` tag lacks `,omitempty` and its type is not a pointer. Edge Go types map by schema-library convention within the subset: `time.Time` → `{"type": "string", "format": "date-time"}`, embedded structs flattened as `encoding/json` promotes them, `json:"-"` fields omitted. `NewTool` panics — naming the offending field and construct — on an input struct the reflector cannot map into the subset: `any`, `json.RawMessage`, unmarshalable kinds (`chan`/`func`/`complex`), Go maps, recursive structs, and unrecognized or out-of-subset `jsonschema` tag keys (D4). Property metadata comes from `jsonschema` struct tags limited to the subset vocabulary (`description`, `title`, `enum`, `const`, `format`, `pattern`, `minLength`, `maxLength`, `minItems`, `default`); `required` is no longer a tag key. The derived schema stays cached and byte-stable, and returned bytes stay defensively copied.

`RawTool` and the `rawTool` type are removed from the public API, leaving `NewTool` and the MCP wrapper as the only producers of a `Tool`. Every test that used `RawTool` moves to a test-only helper in the package under test. `go.mod`/`go.sum` lose the invopop dependency and its transitive closure.

Tags on existing structs are brought in line: `ocr/tool.go`'s `jsonschema:"required"` is deleted (required-ness now comes from the `json` tag), and any test struct relying on the old marker is updated.

**Done when:**
- `R-WYZP-N2VB` — the derived schema reflects the struct's fields with each property's `description` from its `jsonschema` tag, is byte-stable across calls, and is immune to mutation of a previously returned slice.
- `R-XZ8K-R3NX` — a field is required exactly when its `json` tag lacks `,omitempty` and it is not a pointer; a struct with one plain and one `,omitempty` field yields a `required` array holding only the plain field.
- `R-Y0GH-4VEM` — subset tag keys appear verbatim in the derived schema; an out-of-subset key such as `minimum` panics at `NewTool` naming that key.
- `R-Y1OD-IN5B` — a struct with a nested struct field yields the nested shape inline, with no `$schema`, `$defs`, `$ref`, or `additionalProperties` key at any depth.
- `R-DIVW-07P0` — `time.Time` yields `{"type": "string", "format": "date-time"}`, embedded struct fields appear flattened in the parent's `properties`, and a `json:"-"` field is absent.
- `R-AIWI-P5JP` — `NewTool` panics naming the offending field and construct on an `any` or `json.RawMessage` field, a Go map field, or a recursive struct.
- `R-Y2W9-WEW0` — `RawTool` is absent from the public API and `NewTool` is the only exported constructor producing a `Tool`.
- `grep -rn "invopop" --include='*.go' --include='go.mod' --exclude-dir=project . | wc -l` reports `0`.
- `go build ./...` and `go test ./...` both exit 0.
