# Phase 100 — `toolkit.WebFetch`: page fetch with in-house HTML→markdown

*Realizes design Decision 37 (`WebFetch` — page fetch to markdown).*

The `toolkit` package gains the `WebFetch()` constructor and its supporting
markdown renderer. `go.mod` takes the module's first approved external
dependency, `golang.org/x/net` (the `html` package only, per design
Conventions). End state: a consumer appends `toolkit.WebFetch()` to a
conversation's tools and the model can fetch an `http`/`https` URL and read it —
HTML converted to markdown by the in-house renderer over `x/net/html`, other
text verbatim, non-text refused by detected type, the body read capped at 5 MB,
the request bounded by `timeout_seconds` (default 10, accepted range 1–120),
redirects followed, and the result passing through the toolkit's uniform 30k
`capOutput`. Unit tests drive the tool against in-package `httptest` servers and
the renderer against crafted fragments; no live network in the suite.

**Done when:**
- Its Verification ids are covered by clearly-named tagged tests:
  - R-1LWA-VRRX — declared name `WebFetch`; `required` exactly `["url"]`; properties exactly `url` and `timeout_seconds`, each described.
  - R-1N47-9JIM — non-http(s) URL → input error, no request made.
  - R-1OC3-NB9B — HTML→markdown block structure (`#` headings, `-`/`1.` list items, fenced `pre`); `script`/`style`/`head` content absent.
  - R-1QRW-EUQP — `<a href>` → `[text](href)`; entities decoded.
  - R-1RZS-SMHE — non-HTML `text/*` returned verbatim.
  - R-1T7P-6E83 — non-text content type → error naming the detected type, no content.
  - R-1UFL-K5YS — body over 5 MB → error naming the cap; redirects followed en route.
  - R-1VNH-XXPH — `timeout_seconds` defaults to 10, rejects values outside 1–120, and a hanging server fails with a timeout error at the configured value.
- `go build ./...`, `go vet ./...`, and `go test ./...` all exit 0 with no failing package.
