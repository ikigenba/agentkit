# Phase 82 — provider identity: the package id, the auth mode, and the two fields that carry them

*Realizes design Decision 9 (the provider SPI), Decision 7 (the error model), Decision 15 (the JSONL log), and Decision 19 (the embedding SPI). Depends on no pending phase.*

Replaces the single `Name() string` label on both SPIs with a typed `Identity` carrying two facts, and splits every place that label landed into two fields. The observable end state:

- `agentkit.ProviderID`, `agentkit.AuthMode`, and `agentkit.Identity` exist in the root package, both ids defined `string` types so a third-party adapter can still name itself and both serialize as plain JSON strings. `Identity.String()` renders the dotted display form (`openai.subscription`).
- The exported constants are `ProviderAnthropic`, `ProviderOpenAI`, `ProviderGoogle`, `ProviderZAI`, `ProviderOpenRouter`, and `AuthAPIKey`, `AuthSubscription`. **`ProviderZAI` is `"z-ai"`, not `"zai"`** — the Go package keeps its name, the id follows the vendor spelling so the catalog's OpenRouter join needs no mapping table (D26).
- `agentkit.Provider` (`orchestration.go`) and `agentkit.EmbeddingProvider` both replace `Name() string` with `Identity() Identity`. Every adapter follows: `anthropic`, `google`, `zai`, `openrouter`, `internal/openaicompat`, `openai` chat, `openai` embedding, `google` embedding.
- `openai`'s chat provider reports `{ProviderOpenAI, AuthSubscription}` or `{ProviderOpenAI, AuthAPIKey}` from the same credential branch that produced the dotted labels at `openai/openai.go:120`; its embedding provider reports `{ProviderOpenAI, AuthAPIKey}`, closing the asymmetry where the chat side said `openai.apikey` and the embedding side said bare `openai`.
- `agentkit.Error` replaces `Provider string` with `Provider ProviderID` and gains `Auth AuthMode`. MCP failures leave both empty and continue to set `MCPServer`.
- `LogRecord` (`log.go`) types `Provider` as `ProviderID` and gains `Auth AuthMode` with tag `json:"auth,omitempty"`; the `turn_start` emitter at `orchestration.go:229` fills both from the conversation provider's `Identity`.

Nothing about routing, credentials, or the wire changes: this is the label surface only. The `zai` → `z-ai` id change is user-visible in error values and log output, which is why it rides this release rather than a later one.

**Done when** all of the following hold:

- Each id below is covered by a clearly-named test carrying the id verbatim as a tag:
  - `R-LK0H-9AXO` — every shipped chat provider's `Identity()` returns its `ProviderID` and the `AuthMode` its credential selects; both `openai` credential modes return the same `ProviderID`; `Identity.String()` renders `openai.subscription` and `z-ai.apikey`; no shipped provider returns the zero `AuthMode`.
  - `R-LL8D-N2OD` — `openai.NewEmbedder` reports `{ProviderOpenAI, AuthAPIKey}` (identical `ProviderID` to its chat sibling) and `google.NewEmbedder` reports `{ProviderGoogle, AuthAPIKey}`.
  - `R-LMGA-0UF2` — a provider `*Error` carries `Provider` and `Auth` as two typed fields matching the failing handle; an MCP failure leaves both empty with `MCPServer` set; the two serialize as separate `provider` and `auth` JSON strings.
  - `R-LNO6-EM5R` — a `turn_start` record emits `provider` and `auth` as two JSON string fields, asserted on the raw log bytes, with no dotted composite in `provider` for either OpenAI mode and `"provider":"z-ai"` for a Z.ai turn.
- No dotted provider label survives outside the display renderer: `grep -rnE '"(openai|z-ai)\.(apikey|subscription)"' --include='*.go' . | grep -v _test.go` matches only `Identity.String()`'s implementation, or nothing at all.
- `go build ./...` and `go test ./...` both exit 0 (design Conventions).
- The integration suite compiles: `go vet -tags integration ./...` exits 0.
