# Phase 79 — split the OCR cache from the OCR transcript

*Realizes design Decision 31 (the `ocr` package: the document-text tool, its cache, and its transcript).*

Reworks `ocr` so the two artifacts live in two places: the raw provider response is cached durably under `cacheDir`, and the readable transcript is derived into `<root>/ocr/` on every call. The tool returns the transcript path, which is inside the confinement root by construction, so a consumer can put `cacheDir` anywhere durable without handing the model a path its own file tools refuse to open.

The observable end state in `ocr/tool.go`:

- A successful extraction writes `<cacheDir>/<stem>-<hash8>.json` (the response body byte-for-byte) and `<root>/ocr/<stem>-<hash8>.md` (the derived transcript), each written atomically by temp file plus rename. The `<stem>-<hash8>/` directory, its `os.MkdirTemp` and its directory-rename no longer exist; one file per location makes a plain file rename sufficient.
- The transcript is written on **every** call, hit or miss, so a deleted, truncated, or agent-edited transcript is restored on the next call with no provider request. `readCache`'s transcript-repair branch goes away, because derivation is now unconditional rather than a recovery path.
- A cache hit whose bytes `Transcript` rejects returns an error before any network call and leaves the file in place. It is not treated as a miss and not re-extracted.
- The result names the `<root>/ocr/` path and never the cache file. `hash8` stays 8 hex characters and the 30,000-character preview with its page-boundary cutting is untouched.
- The signature `Tool(root, cacheDir string, backend *Client)` is unchanged, `cacheDir` stays required and non-empty, and `cacheDir == root` remains legal.

Tests: three ids are new and need tests written; six have tests that assert the old single-directory layout and must be rewritten against the new one. Tests for `R-UTL6-Q86Y` in particular must use a `cacheDir` and a `root` that are disjoint temp directories, neither containing the other, or the property under test cannot fail. `ocr`'s tests stay internal to the package and do not import `toolkit`; where the transcript lands is asserted directly with the filesystem, not through another package's confined reader.

**Done when** all of the following hold:

- Every D31 Verification id is covered by a clearly-named test tagged with the bare id in a `*_test.go` file, with new tests for the three new behaviors:
  - `R-UTL6-Q86Y` — with disjoint `cacheDir` and `root`, the returned path resolves inside `root` and no `.md` exists anywhere under `cacheDir`.
  - `R-UW0Z-HROC` — a modified transcript is overwritten with the derived text on the next call, with no HTTP request.
  - `R-UX8V-VJF1` — a cached `.json` that `Transcript` rejects returns an error, issues no HTTP request, and leaves the file in place.
  and updated tests for the six whose assertions change: `R-V4A5-MOLS`, `R-V6PY-E836`, `R-V7XU-RZTV`, `R-V95R-5RKK`, `R-VE1C-OUJC`, `R-VF99-2MA1`.
- No test still asserts the removed layout: `grep -rn 'transcript\.md\|response\.json' --include='*.go' --exclude-dir=project .` returns no match (both filenames are gone from the package).
- The design-only id difference is empty (the coverage check in `project/plan/README.md`).
- The suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions).
