# Phase 117 — Extract the shared Responses core; keep openai green

*Realizes design Decision 40 (xAI / Responses core) — slice R-DE1J-AWJ6, R-DF9F-OO9V, R-DP0M-QU7F.*

Move the OpenAI Responses request build, SSE assemble, `store:false` / `instructions` / encrypted-reasoning include, and tool-item replay out of `openai/` into an unexported `internal/responses` package parameterized by host, path, extra headers, bearer func, identity, error classifier, and cost extractor (D40). `openai.New(APIKey)` and `openai.New(Subscription)` become thin wrappers over that core and keep today's public surface. No `xai` package yet. No consumer-importable `responses` package.

**Done when:**
- R-DE1J-AWJ6 — `openai.New(openai.APIKey(k))` against `httptest` still posts to `/v1/responses` with `Authorization: Bearer <k>`, `store:false`, non-empty `instructions`, and `include:["reasoning.encrypted_content"]`.
- R-DF9F-OO9V — `openai.New(openai.Subscription(ts))` against `httptest` still posts to the Codex backend path with the D25 subscription headers, and the API-key path still carries none of them.
- R-DP0M-QU7F — `go list github.com/ikigenba/agentkit/responses` fails (no public package); `openai` imports `github.com/ikigenba/agentkit/internal/responses`.
- `go build ./...` and `go test ./...` exit 0; `gofmt -l .` prints nothing.
