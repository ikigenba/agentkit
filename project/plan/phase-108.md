# Phase 108 — `anthropic`, `zai`, `openrouter`: the remaining three adapters

*Realizes design Decision 38 (the remaining-adapters slice: R-UNQN-ZAH5). Depends on Phase 103.*

The last three chat adapters take the same edit `openai` and `google` took, three
times: the credential arm of the round-trip's configuration guard splits out, so
an empty `APIKey` produces an error wrapping `agentkit.ErrMissingCredential`
whose message names that package and its API key, while nil-provider and
nil-request keep `ErrInvalidConfig`. `zai` and `openrouter` reach the wire
through `internal/openaicompat`, so the check belongs wherever each package's
guard currently sits, not duplicated into the shared adapter's own paths.

Each package's `Credential` set has one member, so no unusable-credential case
arises in any of the three.

This phase covers three packages rather than the usual one because the three
edits are the same edit and fit a single build turn together.

**Done when:**
- R-UNQN-ZAH5 is covered by a table-driven test spanning **all three** of `anthropic`, `zai`, and `openrouter`, asserting for each that `New(APIKey(""))` constructs without panicking, that a `Send` against it surfaces through `Stream.Err()` an error satisfying `errors.Is(err, agentkit.ErrMissingCredential)` whose message names that package and its API key, and that no HTTP request is issued (recorded request count = 0).
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
