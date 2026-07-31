# Phase 110 — `toolkit` options: one constructor shape, a settable search endpoint, an injectable client

*Realizes design Decision 27 (the package-wide `Option` type and the uniform constructor shape; no Verification ids of its own — the shape is compiler-enforced), Decision 36 (`R-1D5U-0OIN`, `R-1EDQ-EG9C`, `R-1FLM-S801`, `R-1GTJ-5ZQQ`), and Decision 37 (`R-1I1F-JRHF`).*

`toolkit` gains one `Option` type (`toolkit/toolkit.go`) and two options,
`WithBaseURL` and `WithHTTPClient`. Every tool constructor in the package takes
its required arguments positionally and then `opts ...Option`, so `Bash`,
`Read`, `Write`, `Edit`, `Glob`, `Grep`, `WebSearch`, and `WebFetch` all read
alike; the six local tools honor no option today and each constructor's doc
comment names what it honors. `All(root)` is a bundle, not a tool constructor,
and its signature is unchanged.

`WebSearch` (`toolkit/websearch.go`) takes its endpoint root from `WithBaseURL`,
defaulting to Brave's `https://api.search.brave.com`, which becomes an
unexported **constant** rather than the mutable package variable it is today. A
supplied value has any trailing slash trimmed; a supplied-but-empty value fails
the call with `agentkit.ErrInvalidConfig` naming the base URL, checked after the
existing D38 key check and issuing no request. Both web tools carry their
request with the client from `WithHTTPClient`, defaulting through
`internal/httpx.Client` to `http.DefaultClient` as the rest of the module does,
replacing the two direct `http.DefaultClient.Do` calls at
`toolkit/websearch.go:111` and `toolkit/webfetch.go:52`. `WebSearch` keeps its
own 10-second deadline on top of whatever client is in force, so an injected
client shortens the bound and one with no timeout is still bounded.

The package retains no mutable state for tests: `braveWebSearchBaseURL` and
`webSearchTimeout` are gone, and the existing suite reaches its `httptest`
servers through `WithBaseURL` and `WithHTTPClient` exactly as a consumer would.
The seven `setWebSearchBaseURL` call sites and the timeout swap in
`toolkit/websearch_test.go` go with them.

**Done when:**
- `R-1D5U-0OIN` — a tool built with `WithBaseURL(srv.URL)` reaches that server and no other host, at path `/res/v1/web/search`; with no options it targets `https://api.search.brave.com`; `srv.URL+"/"` and `srv.URL` produce a byte-identical request URL.
- `R-1EDQ-EG9C` — with a non-empty key, `WithBaseURL("")` fails the call wrapping `agentkit.ErrInvalidConfig` (not `ErrMissingCredential`), naming the base URL, with zero requests recorded; with both key and base URL empty, the error wraps `ErrMissingCredential` instead.
- `R-1FLM-S801` — `WebSearch` built with `WithHTTPClient(c)` has its request carried by `c`'s custom `Transport` (exactly one recorded round trip carrying `X-Subscription-Token`), and with no options routes through `internal/httpx.Client(nil)`.
- `R-1GTJ-5ZQQ` — against a never-responding server, `WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond})` fails in well under one second; against a responsive server with `WithHTTPClient(&http.Client{Timeout: 0})`, the handler observes a request context carrying a deadline no later than ten seconds out.
- `R-1I1F-JRHF` — `WebFetch` built with `WithHTTPClient(c)` has its fetch carried by `c`'s custom `Transport` and not `http.DefaultClient`; with no options it routes through `internal/httpx.Client(nil)`; with an injected zero-`Timeout` client, a call against a never-responding server still fails within its `timeout_seconds` bound.
- `grep -rn 'http\.DefaultClient\.Do' toolkit/` returns no matches.
- `grep -rnE '^\s*(braveWebSearchBaseURL|webSearchTimeout)\s*=' toolkit/` returns no matches outside a `const` declaration.
- `go build ./...`, `go vet ./...`, and `go test ./...` all exit 0 with no failing package.
