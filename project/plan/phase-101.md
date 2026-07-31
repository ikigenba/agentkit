# Phase 101 — `toolkit.WebSearch`: Brave-backed web search

*Realizes design Decision 36 (`WebSearch` — Brave-backed web search).*

The `toolkit` package gains the `WebSearch(apiKey string)` constructor: a direct
raw-`net/http` client of Brave's web-search endpoint (research §19) exposed as
an `agentkit.Tool`. End state: a consumer appends `toolkit.WebSearch(key)` to a
conversation's tools and the model can search the web through the ten mirrored
Brave parameters, receiving the shaped JSON subset (web results as
`{title, url, description, extra_snippets?}` plus optional sections only when
present); an empty key panics at construction; `text_decorations=0` is always
pinned; requests carry a fixed 10-second timeout; failures — including 429 with
`Retry-After` — return as informative tool errors with no retry; results pass
through the 30k `capOutput`. `All(root)` is untouched. The base URL carries an
unexported in-package test seam; unit tests replay the measured payloads and
error shapes of research §19 from `httptest` servers — no live Brave calls in
the suite.

**Done when:**
- Its Verification ids are covered by clearly-named tagged tests:
  - R-1EKW-L5BR — `WebSearch("")` panics; a non-empty key constructs without panicking.
  - R-1FSS-YX2G — declared name `WebSearch`; `required` exactly `["query"]`; exactly the ten documented properties, each described.
  - R-1H0P-COT5 — the request wire: one `GET /res/v1/web/search` with `X-Subscription-Token`, `q`, `text_decorations=0`, supplied optionals under their Brave names (comma-joined `result_filter`), omitted optionals absent.
  - R-1I8L-QGJU — the measured 200 payload shapes to the documented JSON subset with vendor clutter absent.
  - R-1JGI-48AJ — non-2xx → tool error naming the HTTP status and Brave's `error.detail`, including `Retry-After` when present.
  - R-1KOE-I018 — an unresponsive server fails with a timeout error instead of hanging.
- `go build ./...`, `go vet ./...`, and `go test ./...` all exit 0 with no failing package.
