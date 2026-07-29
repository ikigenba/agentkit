# Phase 85 — release v0.10.0

*Realizes no Decision — a release phase. Depends on Phase 84 (the changelog must describe the shipped catalog surface).*

Documents the release in `CHANGELOG.md`, following the shape of the existing entries: a `## v0.10.0` section at the top of the list, below the intro, written in past tense from the consumer's point of view, describing observable surface rather than internal mechanics.

The minor version is the breaking slot pre-1.0, and this release earns it several times over. Cover, in this order:

- **The catalog is organized by offering.** `catalog.Entry` no longer has `Provider` and `Routes`; it has a `Vendor` and an ordered `Offerings` slice, one per provider that can serve the model, `[0]` being the default. Rates, reasoning vocabulary, and context size now live on the offering, because the same model reached two ways is not the same deal — `glm-5.2` through OpenRouter no longer reports Z.ai's figures. Say plainly what a consumer gets: name a model and receive every route in preference order with each route's terms, in one call.
- **`Resolve` returns a `Resolution`, not four values.** The trailing `ok bool` is gone. `Coverage` states which of three things happened — `Curated`, `Passthru`, or `Unrouted` — so "the catalog has no entry for this pair" can no longer be misread as "this provider cannot serve it." Note that nothing about it gates: every state still returns a runnable model string.
- **Wire ids are derived, not stored.** A consumer never types or sees `x-ai/grok-4.5`; naming `grok-4.5`, with or without `openrouter`, is enough. `Entry.WireModel` computes it.
- **New and renamed catalog calls**: `Offerings`, `Offer`, `Entry.WireModel`, `catalog.VendorID`; `ListByProvider` is now `ListCurated`, because it reports what the catalog covers and never claimed to know what a provider can serve. `Check` takes a provider argument, since two routes to one model can accept different values.
- **Providers report an `Identity`, not a name.** `Provider.Name() string` and `EmbeddingProvider.Name() string` are now `Identity() Identity`, carrying `ProviderID` and `AuthMode` separately. `agentkit.Error` splits `Provider` into a typed `Provider` plus `Auth`, and the JSONL `turn_start` record grows an `auth` field. Anything reading a log or an error for `"openai.subscription"` now reads `provider` and `auth` independently; `Identity.String()` still renders the old form for display.
- **`zai`'s provider id is now `z-ai`.** Errors and log records carry the new spelling. It matches the vendor's namespace so the catalog's OpenRouter derivation needs no mapping; the Go package name is unchanged.
- **`ReasoningSpec.Default` changed type** from `agentkit.ReasoningValue` to `catalog.ReasoningDefault`, which states a mode (`DefaultOff`, `DefaultFixed`, `DefaultDynamic`, or the zero `DefaultUnaudited`) and carries a value only when one honestly exists. Say why: a provider that decides per request has no value to report, and the old type forced one to be invented.
- **`ReasoningSpec` gained `CanEnable`**, because switching reasoning on and switching it off are permissions models grant separately.
- **`agentkit.EnableReasoning()` is new** — the explicit on-form, lowered to each wire's native representation. Note that it makes `grok-4.20`'s reasoning reachable for the first time, since that model reasons only when explicitly asked, and that Google and OpenAI reject it with `ErrInvalidConfig` because those wires have no bare on-form.
- **Catalog data corrections**, most visibly `kimi-k3` gaining `CanDisable: true` and `grok-4.20` being recorded as defaulting to off. Both were measured against the live APIs.

Note plainly that nothing about what AgentKit *sends* changed for existing code: an unset reasoning value still transmits no reasoning fields, so the provider's own default still applies exactly as before.

Structural phase: it adds no code and no Verification ids, so its acceptance is a deterministic file check rather than an id list.

**Done when** all of the following hold:

- `CHANGELOG.md` contains a line matching `^## v0\.10\.0` (`grep -cE '^## v0\.10\.0' CHANGELOG.md` returns 1).
- That section documents the catalog reshape, the identity split, and the new constructor: with `S='sed -n "/^## v0\.10\.0/,/^## v0\.9\.0/p" CHANGELOG.md'`, each of `Offerings`, `Resolution`, `ListCurated`, `Identity`, `ReasoningDefault`, and `EnableReasoning` appears at least once in that section.
- The `z-ai` rename is called out: that same section matches `z-ai`.
- The previous entry is untouched: `grep -cE '^## v0\.9\.0' CHANGELOG.md` still returns 1.
- The suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions).

Tagging is **not** part of this phase. Versions are annotated git tags cut by the operator on `main` (`git tag -a v0.10.0 -m "v0.10.0"`); the build loop never creates one.
