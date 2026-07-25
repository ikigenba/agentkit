# Phase 74 — `ocr`: the OpenRouter request and the `Transcript` deriver

*Realizes design Decision 32 (OpenRouter document parsing). Depends on Phase 73.*

Adds the wire half of `ocr`: the `OpenRouter` backend struct, the request it builds, and the pure exported `Transcript` function that turns a raw response body into Markdown. This is where every fragile wire assumption lives, and it is deliberately separated from HTTP and the filesystem so it can be proven against recorded real responses.

Observable end state: `ocr` exports `OpenRouter` (with `APIKey`, `Model`, `BaseURL`, `HTTP` fields) and `Transcript(response []byte) (string, error)`. The request carries the document as a `file` block with a `data:` URI, no text content part, `max_tokens: 1`, and the `file-parser` plugin pinned to `mistral-ocr`; images are routed through Phase 73's `wrapImage` first, so a PNG or JPEG is sent as a one-page PDF. `Transcript` strips the `<file …>`/`</file>` sentinels by pattern, keeps only `type == "text"` items as pages in order, separates them with `<!-- page N -->`, drops every `image_url` item, and errors on an `error` body or on an empty extraction. Retry goes through the shared executor (`internal/retry`, D21) with body-derived classification, since the HTTP status cannot be trusted.

Testing substrate: unit tests run `Transcript` against **recorded real response bodies** committed as testdata (including the observed HTTP-200-carrying-`code: 413` body and a response whose `image_url` item exceeds 20,000 characters); request-shape assertions capture the outbound body at an `httptest` server pointed at by `BaseURL`. One id is a live integration test, gated behind the `integration` build tag and skipped when its credential env var is absent, so the default suite stays hermetic.

**Done when** the suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions) and each id below is covered by a clearly-named test in `ocr/*_test.go` tagged with the bare id:

- R-UQV9-F7G5 — the outbound request has no text content part, `max_tokens: 1`, the `file-parser`/`mistral-ocr` plugin, and a `data:` URI file block (captured at `httptest`).
- R-US35-SZ6U — an empty `Model` sends `google/gemini-2.5-flash-lite`; a set `Model` is sent verbatim.
- R-UTB2-6QXJ — `Transcript` returns pages in order, sentinels removed, separated by `<!-- page N -->` numbered from 1.
- R-UUIY-KIO8 — `Transcript` omits every `image_url` item, so no base64 payload appears in its output.
- R-UVQU-YAEX — a blank page keeps its slot: its marker is emitted and later page numbers do not shift.
- R-UWYR-C25M — sentinels are matched by pattern: a page whose text begins with `<file` is kept as a page.
- R-UY6N-PTWB — absent `annotations`, an empty array, or zero text characters after filtering each return an error, never an empty success.
- R-UZEK-3LN0 — a body carrying an `error` object returns an error naming its `code` and `message`, asserted against the recorded HTTP-200 + `code: 413` response, without consulting the status.
- R-V0MG-HDDP — *(integration tag, live OpenRouter)* a real call with the image-only scanned PDF fixture returns a transcript containing the substring `SPECIMEN4242` (substring, never a hash — the engine is not byte-reproducible).
