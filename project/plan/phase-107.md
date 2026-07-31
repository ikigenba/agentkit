# Phase 107 — `google`: chat and embeddings report their own missing credential

*Realizes design Decision 38 (the google slice: R-UMIR-LIQG). Depends on Phase 103.*

`google.RoundTrip` folds a nil provider, a nil request, an empty base URL and an
absent key into one bare `ErrInvalidConfig`. The credential arm splits out the
same way `openai`'s does: an empty `APIKey` produces an error wrapping
`agentkit.ErrMissingCredential` naming the Google API key, while the other arms
keep `ErrInvalidConfig`. The embedding provider's round-trip splits identically,
so `NewEmbedder(google.APIKey(""))` constructs and `Embed` reports the missing
key.

`google.Credential` has one member, so there is no unusable-credential case here.

**Done when:**
- R-UMIR-LIQG is covered by a test asserting that `google.New(google.APIKey(""))` constructs without panicking and a `Send` against it surfaces through `Stream.Err()` an error satisfying `errors.Is(err, agentkit.ErrMissingCredential)` naming the Google API key; that `google.NewEmbedder(google.APIKey(""))` constructs without panicking and its `Embed` fails matching the same sentinel; and that neither issues an HTTP request (recorded request count = 0).
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
