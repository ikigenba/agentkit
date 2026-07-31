# Phase 104 — `toolkit.WebSearch`: typed credential, no construction panic

*Realizes design Decision 38 (the toolkit slice: R-UGF9-OO0Z) and Decision 36 (revised signature). Depends on Phase 103.*

`toolkit` gains `type BraveAPIKey string`, and `WebSearch` takes it in place of
the bare `string`. The construction-time panic is deleted: an absent key
constructs a tool exactly like a present one does, and the tool's invocation
returns an error wrapping `agentkit.ErrMissingCredential` whose message names
both `WebSearch` and the Brave Search API key, before any HTTP request is
built. `TestWebSearchConstruction` asserted the panic and is deleted along with
the retired requirement tag it carries, whose behavior no longer exists; the
surviving half of what it checked (a tool is returned) is subsumed by this
phase's new test.

Everything else about the tool is unchanged: schema, parameter mapping, output
reduction, error passthrough, and the 10-second bound all stay as D36 states.

**Done when:**
- R-UGF9-OO0Z is covered by a test asserting that `toolkit.WebSearch(toolkit.BraveAPIKey(""))` returns a tool without panicking (via `recover`), that invoking it returns an error satisfying `errors.Is(err, agentkit.ErrMissingCredential)` whose message names `WebSearch` and the Brave key, and that the `httptest` backend recorded **zero** requests.
- `grep -rn 'TestWebSearchConstruction' --include='*_test.go' .` produces no output — the panic test and its retired tag are gone.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
