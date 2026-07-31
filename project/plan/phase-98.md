# Phase 98 — Live Gemini proof of the single-block answer

*Realizes design Decision 35 (transport-shape independence), the live slice: R-R10G-JG1U. Depends on Phase 96 and Phase 97.*

Phases 96 and 97 prove the adapter handles the frame shape we believe Gemini
sends. This phase proves that belief is still true against the real API, which
no fixture can do: the fixtures encode our model of the wire, so a change in
Gemini's framing would leave them green while the assembled message degraded.

`google/google_integration_test.go` gains an `integration`-tagged test that
makes one real `RoundTrip` against a Gemini model with a prompt eliciting a
multi-sentence reply, and asserts the returned `Message` carries exactly one
`TextBlock` with non-empty text. It skips when the credential env var is absent,
so the default `go test ./...` stays hermetic and green offline, per design
Conventions.

**Done when:**
- `R-R10G-JG1U` — the live call returns a `Message` whose `TextBlock` count is
  exactly 1 and whose text is non-empty. The assertion is on a completed call's
  returned blocks, not on any configured request value.
- The id appears verbatim as a `// R-R10G-JG1U` comment on the test asserting
  it, in `google/google_integration_test.go`.
- `go build ./...` and `go test ./...` both exit 0 with the credential absent
  (the test skips, the suite stays green).
- `go test -tags integration -run R10G ./google/` exits 0 with the credential
  present, and its output does not contain `SKIP`.
