# Phase 83 — release v0.10.0

*Realizes no Decision — a release phase. Depends on Phase 82 (the changelog must describe the shipped catalog surface).*

Documents the release in `CHANGELOG.md`, following the shape of the existing entries: a `## v0.10.0` section at the top of the list, below the intro, written in past tense from the consumer's point of view, describing observable surface rather than internal mechanics.

The minor version is the breaking slot pre-1.0, and this release earns it: `catalog.ReasoningSpec` changes shape, so consumer code that reads a model's reasoning default no longer compiles.

- **`ReasoningSpec.Default` changed type** from `agentkit.ReasoningValue` to the new `catalog.ReasoningDefault`, which states a mode (`DefaultOff`, `DefaultFixed`, `DefaultDynamic`, or the zero `DefaultUnaudited`) and carries a value only when one honestly exists. Code that read `Default` as a `ReasoningValue` must read `Default.Mode`, and `Default.Value` only in the `DefaultFixed` case. Say why: a provider that decides per request has no value to report, and the old type forced one to be invented.
- **`ReasoningSpec` gained `CanEnable`**, because switching reasoning on and switching it off are permissions models grant separately.
- **`agentkit.EnableReasoning()` is new** — the explicit on-form, lowered to each wire's native representation. Note that it makes `grok-4.20`'s reasoning reachable for the first time, since that model reasons only when explicitly asked, and that Google and OpenAI reject it with `ErrInvalidConfig` because those wires have no bare on-form.
- **Catalog data corrections**, most visibly `kimi-k3` gaining `CanDisable: true` and `grok-4.20` being recorded as defaulting to off. Both were measured against the live APIs.

Note plainly that nothing about what AgentKit *sends* changed for existing code: an unset reasoning value still transmits no reasoning fields, so the provider's own default still applies exactly as before.

Structural phase: it adds no code and no Verification ids, so its acceptance is a deterministic file check rather than an id list.

**Done when** all of the following hold:

- `CHANGELOG.md` contains a line matching `^## v0\.10\.0` (`grep -cE '^## v0\.10\.0' CHANGELOG.md` returns 1).
- That section documents the breaking type change and the new constructor: `sed -n '/^## v0\.10\.0/,/^## v0\.9\.0/p' CHANGELOG.md | grep -qE 'ReasoningDefault' && sed -n '/^## v0\.10\.0/,/^## v0\.9\.0/p' CHANGELOG.md | grep -qE 'EnableReasoning'`.
- The previous entry is untouched: `grep -cE '^## v0\.9\.0' CHANGELOG.md` still returns 1.
- The suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions).

Tagging is **not** part of this phase. Versions are annotated git tags cut by the operator on `main` (`git tag -a v0.10.0 -m "v0.10.0"`); the build loop never creates one.
