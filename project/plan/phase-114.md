# Phase 114 — Live-provider proof that the non-strict wire is accepted

*Realizes design Decision 22 (per-provider schema rendering), the live-substrate slice. Depends on Phase 113.*

Phase 113's fixtures prove a field changed. They cannot prove the providers stop rejecting us, because a recorded server accepts whatever it is handed. This phase adds the two checks whose substrate can actually falsify the claim, under the existing `integration` build tag so the default `go test ./...` stays hermetic and green offline.

What exists at the end of this phase:

- An integration test that attaches a large tool set to a real Anthropic conversation and completes a turn. This is the production failure the whole change exists to fix: at 146 tools the strict build returned `Too many strict tools (146). The maximum number of strict tools supported is 20.` The test asserts a returned assistant message, not merely a 200, so it proves the request ran rather than that a field was set.
- An integration test that sends a real OpenAI request carrying a tool schema with a partial `required` array and completes a turn, closing the one inference in Decision 22: research §18.2 documents the all-properties-required rule under `strict: true` and §18.4 never probed it non-strict, so Phase 113's OpenAI change currently rests on a general finding rather than a direct measurement.
- Both skip cleanly when their credentials are absent, per the design Conventions for integration tests.

**Done when:** `go build ./...` and `go test ./...` both exit 0 with no failing package (the new tests skipping or excluded by build tag), `go test -tags integration ./...` exits 0 with credentials present, and each of the following ids is carried by a clearly-named test tagged with the bare id in a `*_test.go` file:

- R-85MD-R0J3 — a live Anthropic `Send` attaching **more than 20 tools** (at least 30) completes a turn and returns an assistant message; reinstating `strict` makes this fail against the live API while every fixture test still passes.
- R-86UA-4S9S — a live OpenAI `Send` carrying a tool whose schema has a partial `required` array (at least one property in `properties` omitted from `required`) completes a turn and returns an assistant message.
