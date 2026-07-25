# Phase 78 — release v0.8.0

*Realizes no Decision — a release phase. Depends on Phase 77 (the changelog must describe the shipped `ocr` surface, not the pre-`WithModel` one).*

Documents the release in `CHANGELOG.md`, following the shape of the existing entries: a `## v0.8.0` section at the top of the list, below the intro, written in past tense from the consumer's point of view, describing observable surface rather than internal mechanics.

The entry covers what this release actually adds and changes:

- The new `ocr` subpackage: an `OCR` tool that extracts text from scanned PDFs and raster images, built on OpenRouter's `file-parser` plugin, constructed as `ocr.New(ocr.APIKey(key), opts...)` and wired with `ocr.Tool(root, cacheDir, backend)`.
- Extraction artifacts written to a consumer-named cache directory (raw provider response plus a derived `transcript.md`), a repeat call on unchanged input served without a new request, and a result that returns a bounded preview plus the transcript path so a large document never floods the conversation.
- Images normalized to a one-page PDF so PDFs and images take one extraction path.
- Failed or empty extractions surfaced as errors and never cached, including provider errors that arrive with a `200` status.
- **A behavior change to `toolkit.Read`:** it now refuses non-text files with an error naming the detected content type instead of decoding binary as text. Detection is by content, not extension. Call this out as a change in existing behavior, not only an addition, since it can affect an existing consumer.

Structural phase: it adds no code and no Verification ids, so its acceptance is a deterministic file check rather than an id list.

**Done when** all of the following hold:

- `CHANGELOG.md` contains a line matching `^## v0\.8\.0` (`grep -cE '^## v0\.8\.0' CHANGELOG.md` returns 1).
- That section mentions both `ocr` and `Read`, so neither half of the release is undocumented: `sed -n '/^## v0\.8\.0/,/^## v0\.7\.0/p' CHANGELOG.md | grep -qE '\bocr\b' && sed -n '/^## v0\.8\.0/,/^## v0\.7\.0/p' CHANGELOG.md | grep -qE '\bRead\b'`.
- The previous entry is untouched: `grep -cE '^## v0\.7\.0' CHANGELOG.md` still returns 1.
- The suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions).

Tagging is **not** part of this phase. Versions are annotated git tags cut by the operator on `main` (`git tag -a v0.8.0 -m "v0.8.0"`); the build loop never creates one.
