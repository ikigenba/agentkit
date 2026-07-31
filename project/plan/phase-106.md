# Phase 106 — `openai`: chat and embeddings report their own missing credential

*Realizes design Decision 38 (the openai slice: R-UIV2-G7ID, R-ULAV-7QZR). Depends on Phase 103.*

`openai.RoundTrip` currently folds a nil provider, a nil request, and an absent
credential into one bare `ErrInvalidConfig`. The credential arm splits out: an
empty `APIKey` and a `Subscription` with a nil token source each produce an
error wrapping `agentkit.ErrMissingCredential`, naming the API key and the
ChatGPT subscription token source respectively — two distinct messages. The nil
provider and nil request arms keep `ErrInvalidConfig` exactly as they are.

`NewEmbedder` keeps its `Credential` parameter and loses its panic. An empty
`APIKey` constructs, and `Embed` reports the missing API key. A `Subscription`
credential also constructs, and `Embed` rejects it with `ErrInvalidConfig` —
deliberately *not* `ErrMissingCredential`, because a credential was supplied —
stating that a ChatGPT subscription cannot serve embeddings and an API key is
required. That is the operation-relative case the convention exists for: the
subscription backend speaks only `chatgpt.com/backend-api/codex/responses`, and
OAuth tokens are rejected by `api.openai.com/v1/*` (research §15.2).

**Done when:**
- R-UIV2-G7ID is covered by a test asserting that `openai.New(openai.APIKey(""))` and `openai.New(openai.Subscription(nil))` each construct without panicking, that a `Send` against each surfaces through `Stream.Err()` an error satisfying `errors.Is(err, agentkit.ErrMissingCredential)` with two distinct messages naming the API key and the subscription token source, that neither issues an HTTP request, and that a nil `*Provider` and a nil `*Request` still surface `ErrInvalidConfig`.
- R-ULAV-7QZR is covered by a test asserting that `openai.NewEmbedder(openai.APIKey(""))` constructs without panicking and its `Embed` fails matching `ErrMissingCredential` naming the API key; that `openai.NewEmbedder(openai.Subscription(ts))` constructs without panicking and its `Embed` fails matching `ErrInvalidConfig` and **not** `ErrMissingCredential`, with a message stating a subscription cannot serve embeddings; and that neither issues an HTTP request.
- `grep -c 'embeddings require an APIKey credential' openai/openai.go` returns 0 — the construction panic is gone (the block-translation `default` panic of D9 stays and is not touched).
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
