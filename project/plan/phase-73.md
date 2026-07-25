# Phase 73 — `ocr`: raster normalization to a one-page PDF

*Realizes design Decision 33 (raster normalization).*

Creates the new leaf package `github.com/ikigenba/agentkit/ocr` and its first, entirely self-contained piece: the unexported `wrapImage` helper that embeds a raster image in a minimal one-page PDF, plus the content-type detection that decides which inputs need it.

Observable end state: `ocr/wrap.go` (or equivalently named file) exists in a package that builds under `go build ./...`, exporting nothing yet. `wrapImage(raw []byte) ([]byte, error)` embeds a JPEG byte-for-byte as a `/DCTDecode` image XObject, embeds a PNG losslessly as raw RGB under `/FlateDecode`, sizes the page at `pixels / 200 * 72` points, emits a structurally correct five-object PDF with a valid `xref` table and trailer, and rejects any other detected content type with an error naming that type. Standard library only — `image/png`, `image/jpeg`, `compress/zlib`, `net/http` for detection — with no new module added to `go.mod`.

No HTTP, no filesystem, no credentials, and no `agentkit.Tool` in this phase; it is pure bytes-in, bytes-out, so every id here is provable without a network or a fixture server.

**Done when** the suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions) and each id below is covered by a clearly-named test in `ocr/*_test.go` tagged with the bare id:

- R-UJJV-4KZZ — a JPEG is embedded byte-for-byte as a `/DCTDecode` stream (the source bytes appear verbatim in the output; no re-encode).
- R-UKRR-ICQO — a PNG is embedded losslessly under `/FlateDecode`: inflating the image stream yields raw RGB matching the source pixel for pixel.
- R-ULZN-W4HD — the page `/MediaBox` equals `pixels / 200 * 72` in both axes (1698×2200 → 611.28×792).
- R-UOFG-NNYR — every `xref` offset points at the byte position of its `N 0 obj` header, and the trailer `/Size` matches the object count.
- R-UPND-1FPG — an input detected as neither PDF, PNG, nor JPEG (e.g. GIF, or arbitrary non-image bytes) is rejected with an error naming the detected type.
