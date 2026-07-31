# Phase 103 — The `ErrMissingCredential` sentinel

*Realizes design Decision 38 (credential convention — the sentinel slice: R-UF7D-AWAA).*

Root `agentkit` declares `ErrMissingCredential`, a bare sentinel in D7's
boundary-validation family, beside `ErrInvalidConfig` in `orchestration.go`. It
carries no payload, is matched only with `errors.Is`, and is distinct from
`ErrInvalidConfig` so the two conditions never collapse into one branch.

Nothing else changes in this phase: no constructor and no operation consults it
yet. Every later phase in this batch imports it, which is why it lands first.

**Done when:**
- R-UF7D-AWAA is covered by a test asserting that an error wrapping `ErrMissingCredential` matches `errors.Is(err, agentkit.ErrMissingCredential)`, does **not** match `errors.Is(err, agentkit.ErrInvalidConfig)`, and does **not** satisfy `errors.As(err, &agentkit.Error{})`, and that `ErrInvalidConfig` does not match `ErrMissingCredential`.
- `go build ./...` and `go test ./...` both exit 0 with no failing package.
