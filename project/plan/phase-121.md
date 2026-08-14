# Phase 121 — Add `gemini-3.7-flash` to the catalog

*Realizes design Decision 26 (advisory catalog). No new `R-` id: a tracked-model row is catalog **data** realizing D26's existing shape, and its correctness is pinned by D26's already-realized data invariants (golden R-DQ16-AQWE, completeness R-E5FU-SAHM, router coverage R-EBJC-P573, fixed-default R-DHKK-RZ7M / R-DISH-5QYB, provider-set R-LTRO-BGV8, direct-first R-LW7H-30CM), which now range over the new row. Source values: research §6.5 (native + OpenRouter rates), §7.1 (native reasoning vocabulary), §14.7 (OpenRouter descriptor + resolved offering).*

Add one chat entry, `gemini-3.7-flash`, to `catalog/data.go`. Its OpenRouter rate diverges from native (half), so it is authored with an **explicit** `ProviderOpenRouter` alternative offering rather than the native-rate one `chatEntry` auto-appends:

```go
"gemini-3.7-flash": chatEntry(
    "gemini-3.7-flash", VendorGoogle, agentkit.ProviderGoogle, 1_048_576,
    pricing(agentkit.RateTier{InputUncached: 750, CacheReadInput: 75, Output: 3750}),
    enumReasoning("thinking level", []string{"low", "medium", "high"}, "medium", false),
    Offering{
        Provider: agentkit.ProviderOpenRouter, WireName: openRouterWireName("gemini-3.7-flash"),
        Pricing:   pricing(agentkit.RateTier{InputUncached: 375, CacheReadInput: 38, Output: 1875}),
        Reasoning: enumReasoning("thinking level", []string{"low", "medium", "high"}, "medium", false),
        Context:   1_048_576,
    },
),
```

Native offering: intro pricing 750 / 75 / 3750 nano-USD (flat, single tier), context 1,048,576, reasoning enum `low`/`medium`/`high` default `medium`, mandatory (`CanDisable` false). OpenRouter offering: audited rates 375 / 38 / 1875, same context and reasoning spec. Both `Default` modes are `DefaultFixed(medium)`, which the spec accepts. Then regenerate the golden reference digest (`recordedReference` in `catalog/data_test.go`) so R-DQ16-AQWE matches the new shipped data — the digest is the *only* expected edit to a test file, and no other `_test.go` is touched.

**Done when:**
- `grep -q '"gemini-3.7-flash"' catalog/data.go` (the entry ships).
- `go test ./catalog/` is green — the regenerated golden (R-DQ16-AQWE) matches, and the new row satisfies every shipped-data invariant (R-E5FU-SAHM completeness, R-EBJC-P573 router coverage, R-DHKK-RZ7M / R-DISH-5QYB fixed-default consistency, R-LTRO-BGV8, R-LW7H-30CM).
- `go build ./...` and the full `go test ./...` suite are green.

(The integration-gated live checks R-4NJ4-SJ41 and R-DOVZ-2LNS enumerate their subjects from the catalog, so the new OpenRouter offering is exercised automatically when the suite runs with `-tags integration` and OpenRouter credentials — verified out-of-band 2026-08-13: the route serves and its off-form is rejected. No hermetic-suite gate depends on credentials.)
