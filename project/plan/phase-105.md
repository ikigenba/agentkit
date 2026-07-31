# Phase 105 — `ocr`: separate a missing credential from a bad configuration

*Realizes design Decision 38 (the ocr slice: R-UHN6-2FRO). Depends on Phase 103.*

`ocr.Client.Do` today rejects `c == nil || c.apiKey == "" || c.baseURL == ""`
with one message, `ocr: invalid OpenRouter configuration`, which names neither
the condition nor the credential. The check splits: an empty `APIKey` returns an
error wrapping `agentkit.ErrMissingCredential` naming the OpenRouter API key,
while a nil client or an empty base URL keeps `agentkit.ErrInvalidConfig` with a
message naming what is actually missing. Both paths return before any HTTP
request is issued, as they do now.

`ocr.New` is unchanged — it already accepts an absent key without panicking, and
it is the reference behavior this convention generalizes.

**Done when:**
- R-UHN6-2FRO is covered by a test asserting that a client built with `ocr.APIKey("")` fails a tool call with an error satisfying `errors.Is(err, agentkit.ErrMissingCredential)` and naming the OpenRouter API key; that a client with a real key and an empty base URL fails instead with `errors.Is(err, agentkit.ErrInvalidConfig)` naming the base URL; that the two messages differ; and that neither issues an HTTP request (recorded request count = 0).
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
