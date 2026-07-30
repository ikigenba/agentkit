# Phase 95 — `toolkit`: described parameters and honest required sets

*Realizes a slice of design Decision 27 (R-Y446-A6MP). Depends on Phase 89.*

Every `toolkit` tool's input struct gains a `jsonschema:"description=…"` tag on each field, so the model receives guidance rather than a bare type. The tools currently ship no property descriptions at all.

Required sets come out correct automatically once Phase 89's `,omitempty` rule lands — `Bash` requires `command` and not `timeout`, `Read`/`Write`/`Edit` require `file_path`, `Glob`/`Grep` require `pattern` — because the structs already mark optional fields `,omitempty`. This phase verifies that outcome rather than editing `json` tags, and fixes any struct whose tags do not match its real optionality.

**Done when:**
- `R-Y446-A6MP` — every property of all six toolkit tools carries a non-empty `description`, and each tool's `required` set names exactly its mandatory parameters (`Bash` → `command`; `Read`/`Write`/`Edit` → `file_path`; `Glob`/`Grep` → `pattern`); an empty `required` array or an undescribed property fails.
- `go build ./...` and `go test ./...` both exit 0.
