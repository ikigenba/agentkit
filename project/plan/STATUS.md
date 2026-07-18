# AgentKit — Plan Status

The manifest, and a work **queue**: one line per **pending** phase, in build order — the only place a phase's status marker lives. Each phase line is a Markdown bullet beginning with `- Phase` and carries the pending-marker (U+2B1C); there is no done-marker, because **completion is deletion** — when a phase passes, the build loop removes its line here and `git rm`s its `project/plan/phase-NN.md` in the completion commit, so the plan never holds finished work and the record of what was built lives in git. The build loop finds the next work with `grep -nE '^- Phase .* ⬜' project/plan/STATUS.md | head -1` and reads only that phase's `project/plan/phase-NN.md`. The `Next phase` counter below is the number the next appended phase takes; `$seal-spec` bumps it on every append, it is never decremented, and a number is never reused — so a phase number names one phase forever, even after its files are gone. (This paragraph and the counter line deliberately carry no bare status glyph, so the anchored grep matches only phase lines.)

Next phase: 65

- Phase 59  ⬜  realizes D25                          — openai subscription auth + openai/subscription (R-DG9Z-8KYU, R-DHHV-MCPJ, R-DIPS-04G8, R-DL5K-RNXM)
- Phase 60  ⬜  realizes D26                          — the advisory catalog package (R-DMDH-5FOB, R-DNLD-J7F0, R-DOT9-WZ5P, R-DQ16-AQWE, R-DR92-OIN3, R-DSGZ-2ADS)
- Phase 61  ⬜  realizes D18,D20                      — embeddings root seam: free-flow, supplied pricing, dimension verification (R-D5AV-SNAL, R-D6IS-6F1A, R-D2V3-13T7, R-D42Z-EVJW, R-D8YK-XYIO)
- Phase 62  ⬜  realizes D19                          — embedding adapters to the dumb layer (R-D7QO-K6RZ)
- Phase 63  ⬜  realizes D13,D24,D25                  — live integration: uniform happy-path suite + OpenRouter reported cost + subscription round trip (D13 slice: R-CL9K-41F1, R-CMHG-HT5Q, R-CNPC-VKWF, R-COX9-9CN4, R-CQ55-N4DT, R-CRD2-0W4I, R-CSKY-ENV7, R-CTSU-SFLW; R-DF22-UT85, R-DJXO-DW6X)
- Phase 64  ⬜  realizes —                            — Release: minor version bump to v0.3.0
