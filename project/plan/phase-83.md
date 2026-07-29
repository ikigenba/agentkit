# Phase 83 — the catalog's offerings: ordered routes, derived wire names, and the resolution surface

*Realizes design Decision 26 (the advisory model catalog). Depends on Phase 82 (`agentkit.ProviderID` must exist, since an offering is keyed by one).*

Reshapes `catalog.Entry` around the offering — one model as served by one provider — and rewrites the lookup surface on top of it. Data migrates mechanically: each existing entry's pricing, reasoning spec, and context move onto its offerings unchanged, so this phase restructures without re-measuring anything. Phase 84 audits the cells.

The observable end state:

- `catalog.Offering` exists, carrying `Provider agentkit.ProviderID`, `Pricing`, `Reasoning`, `Context`, and `Options`. `catalog.Entry` becomes `Model`, `Vendor VendorID`, `Offerings []Offering` (ordered, never empty), and `Embedding`. The `Provider string` and `Routes map[string]string` fields are gone.
- `catalog.VendorID` exists with the constants `VendorAnthropic`, `VendorOpenAI`, `VendorGoogle`, `VendorZAI` (`"z-ai"`), `VendorXAI` (`"x-ai"`), `VendorDeepSeek`, `VendorMoonshot` (`"moonshotai"`) — spelled as OpenRouter spells its namespaces.
- **Wire names are derived, not stored.** `Entry.WireModel(provider)` joins vendor and model with a slash for `ProviderOpenRouter` and returns the model verbatim for a direct provider. The thirteen hardcoded slugs leave `catalog/data.go` entirely.
- `Resolve(provider, model) Resolution` replaces the four-value return. `Resolution` carries `Vendor`, `Provider`, `WireModel`, `Offering`, and `Coverage`; `Coverage` is `Curated`, `Passthru`, or `Unrouted`. The trailing `ok bool` is gone from `Resolve`.
- `Offerings(model) []Offering` and `Offer(model, provider) (Offering, bool)` are new. `ListByProvider` is renamed `ListCurated`. `Check` gains a provider argument.
- Entries are re-authored in the new shape: the four `glm` models get a `z-ai` offering first and an `openrouter` offering second; the nine xAI/DeepSeek/Moonshot models get a single `openrouter` offering and a truthful `Vendor` (`x-ai`, `deepseek`, `moonshotai`) in place of the old `Provider: "openrouter"`; every direct-vendor entry gets one offering for its own provider.
- The golden reference table is regenerated to the new shape so it stays green; the id that pins it is realized by Phase 84, which settles the cells it covers.
- **The aggregator-slug invariant is retired.** It asserted that an aggregator-default entry carries a non-empty `Routes["openrouter"]` slug containing `/`, guarding a bare id reaching OpenRouter through `Resolve`'s fallback to `Entry.Model`. With the slug derived and no route map to omit, the behavior cannot fail; design no longer mints its id, so the tagged test in `catalog/catalog_test.go` is deleted along with it.
- The import-graph invariant is untouched: nothing in root or a provider package imports `catalog`.

**Done when** all of the following hold:

- Each id below is covered by a clearly-named test carrying the id verbatim as a tag:
  - `R-DMDH-5FOB` — `Lookup` returns a cataloged chat model's vendor and offerings, an embedding model's `EmbeddingInfo`, and `ok=false` for an unknown name, with no network or credentials.
  - `R-LOW2-SDWG` — every shipped entry has a non-empty `Offerings` slice returned in authored order, with `Offerings[0]` equal to what `Resolve("", model)` selects; reordering an entry's offerings changes that selection.
  - `R-LRBV-JXDU` — `Offerings(m)` returns the ordered list and `nil` for an uncataloged name; `Offer(m, p)` returns the matching offering or `ok=false`; the two agree for a multi-offering model.
  - `R-DNLD-J7F0` — `Resolve` covers all three `Coverage` states: default offering with no provider named, the named provider's own offering, `Passthru` verbatim with a zero `Offering` for a provider with no offering, and `Unrouted` with an empty `Provider` for an uncataloged model with no provider named.
  - `R-LQ3Z-65N5` — `Entry.WireModel` derives: `grok-4.5` → `x-ai/grok-4.5` and `glm-5.2` → `z-ai/glm-5.2` on OpenRouter, `glm-5.2` → `glm-5.2` on `z-ai`; `Resolution.WireModel` agrees; changing an entry's `Vendor` changes the derived name.
  - `R-LXFD-GS3B` — every `VendorID` that also names a provider package is the identical string to its `agentkit.ProviderID` (pinning `z-ai`); `x-ai`, `deepseek`, and `moonshotai` have no matching `ProviderID`.
  - `R-LTRO-BGV8` — every shipped offering's `Provider` is one of the exported `agentkit.ProviderID` constants.
  - `R-LW7H-30CM` — for every shipped entry, an offering whose `Provider` equals the entry's `Vendor` is `Offerings[0]`.
  - `R-DOT9-WZ5P` — `ListCurated(p)` returns every entry carrying an offering for `p` and nothing else; a provider with no offerings returns an empty list.
  - `R-LYN9-UJU0` — `Check(m, p, v)` validates against `p`'s offering specifically, and returns `ok=false` when the model has no offering for `p`.
  - `R-DR92-OIN3` — neither root `agentkit` nor any provider sub-package imports `agentkit/catalog`.
- The golden table stays green through the reshape (its id is Phase 84's): `go test ./catalog/...` exits 0.
- No wire slug is authored anywhere in the catalog data: `grep -nE '"[a-z0-9-]+/[a-z0-9.-]+"' catalog/data.go` returns no matches.
- The retired invariant's test is gone: no requirement id present in `catalog/catalog_test.go` is absent from `project/design/`, checked with
  `comm -13 <(grep -hoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' project/design/*.md | sort -u) <(grep -hoE 'R-[A-Z0-9]{4}-[A-Z0-9]{4}' catalog/catalog_test.go | sort -u)` returning no output.
- `go build ./...` and `go test ./...` both exit 0 (design Conventions).
- The integration suite compiles: `go vet -tags integration ./...` exits 0.
