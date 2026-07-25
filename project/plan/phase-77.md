# Phase 77 — `ocr`: move the model behind `WithModel`

*Realizes design Decision 32 (the construction seam). Depends on nothing pending.*

Closes the one place `ocr` departs from the repo's construction convention. Every provider package (`anthropic`, `google`, `openai`, `zai`, `openrouter`) is `New(cred, opts ...Option)` over a struct with **only unexported fields**; `ocr.Client` currently matches that except for a single exported `Model` field set after construction.

Observable end state: `Client.Model` becomes unexported and is configured by a new `WithModel(model string) Option`, so all four settings arrive through `New` and `Client` exposes no exported fields at all. A backend handed to `Tool` can no longer be reconfigured through the pointer `Tool` also holds. Behavior is unchanged: an unset model still sends `defaultModel`, a set one is still sent verbatim, so the existing model-selection test is updated to construct with the option rather than assigning the field.

Also settle the surface while here: `Do` stays exported and gains a doc comment naming it as the seam a batch or cache-repair script uses without going through the tool (D32).

**Done when** the suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions), `go vet -tags integration ./ocr/` passes, and both ids below are covered by clearly-named tests in `ocr/*_test.go` tagged with the bare id:

- R-US35-SZ6U — a client built without `WithModel` sends `google/gemini-2.5-flash-lite`; one built with `WithModel("vendor/x")` sends `vendor/x` verbatim. *(existing id, test rewritten to use the option)*
- R-GMLN-XC8W — `Client` exposes no exported fields, asserted by reflection over its type (`reflect.TypeOf(Client{})`, counting fields whose name is exported, expecting zero).
