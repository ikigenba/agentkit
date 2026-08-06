# Phase 112 — Runtime tool-argument validation in root

*Realizes design Decision 39 (runtime tool-argument validation).*

The root `agentkit` package gains `validateToolArguments(schema, args json.RawMessage) error`, a pure first-violation validator over the D34 canonical subset, and the tool loop calls it before dispatching any tool.

What exists at the end of this phase:

- A validator in root, beside `validateToolSchema` (which owns what a tool may *declare*, as this one owns what the model may *send*). It enforces the root-must-be-object rule, `type` including two-branch nullable unions, `required` presence, unknown-property rejection, `enum`, `const`, `properties` and `items` recursively, `anyOf`/`oneOf` branch matching, `$ref` resolution against `$defs`, `minLength`, `maxLength`, `minItems`, and `pattern` compiled with `regexp`. It accepts `format` without checking it and never applies `default`. It never mutates, coerces, or reorders the arguments. Its error text names the violated constraint and the offending location as a JSON Pointer. Standard library only, no new module dependency.
- The orchestration tool loop validates arguments ahead of `Tool.Call` for **every** tool, in-process and MCP alike. A violation means the tool is not invoked at all (and for an MCP tool, no `tools/call` request leaves the process), and the loop feeds back `ToolResultBlock{IsError: true}` carrying the reason, exactly as it already does for an unknown tool name. The turn continues; the existing `MaxToolIterations` bound is what stops a model that never corrects.
- No new sentinel error, no new event type, no `Stream` warning, and no opt-out switch anywhere in the public surface.

**Done when:** `go build ./...` and `go test ./...` both exit 0 with no failing package, and each of the following ids is carried by a clearly-named test tagged with the bare id in a `*_test.go` file:

- R-7M3Z-MONZ — a call omitting a `required` property is not dispatched and returns `IsError: true` naming the missing property; the turn continues.
- R-7NBW-0GEO — a wrong declared `type` (a number for a `"type": "string"` property) is rejected naming the property and both types; the tool is not invoked.
- R-7OJS-E85D — a property absent from `properties` is rejected naming the unknown property.
- R-7PRO-RZW2 — a value outside a string `enum`, and a value unequal to a `const`, are each rejected; a member and a matching `const` dispatch.
- R-7QZL-5RMR — `minLength`, `maxLength` and `minItems` are enforced at their exact boundary (2 rejected / 3 accepted for `minLength: 3`, 4 rejected / 3 accepted for `maxLength: 3`, empty rejected / one element accepted for `minItems: 1`).
- R-7S7H-JJDG — a string violating `pattern` is rejected naming the pattern; a matching string passes; an RE2-valid pattern compiles rather than erroring.
- R-7TFD-XB45 — `anyOf`/`oneOf` accepts a value matching at least one branch and rejects one matching none; a `$ref` is resolved against `$defs` before its target applies; a nullable union accepts both `null` and the non-null type.
- R-7UNA-B2UU — `"format": "email"` with value `"not-an-email"` dispatches; an omitted optional property carrying a `default` is **absent** from the arguments the tool receives.
- R-7VV6-OULJ — a valid call reaches `Call` with byte-identical arguments, asserted by a tool capturing its raw input.
- R-7X33-2MC8 — an MCP tool call violating its server-supplied schema is rejected and **no** JSON-RPC `tools/call` reaches the stub server, which records requests.
- R-7YAZ-GE2X — a violation nested inside an object or array element is caught and its location reported as a JSON Pointer (e.g. `/filters/2/name`).
- R-7ZIV-U5TM — arguments that are not a JSON object (bare array, string, `null`, malformed JSON) are rejected naming the received shape; the turn continues.
