# AgentKit — Plan Status

The manifest, and a work **queue**: one line per **pending** phase, in build order — the only place a phase's status marker lives. Each phase line is a Markdown bullet beginning with `- Phase` and carries the pending-marker (U+2B1C); there is no done-marker, because **completion is deletion** — when a phase passes, the build loop removes its line here and `git rm`s its `project/plan/phase-NN.md` in the completion commit, so the plan never holds finished work and the record of what was built lives in git. The build loop finds the next work with `grep -nE '^- Phase .* ⬜' project/plan/STATUS.md | head -1` and reads only that phase's `project/plan/phase-NN.md`. The `Next phase` counter below is the number the next appended phase takes; `$seal-spec` bumps it on every append, it is never decremented, and a number is never reused — so a phase number names one phase forever, even after its files are gone. (This paragraph and the counter line deliberately carry no bare status glyph, so the anchored grep matches only phase lines.)

Next phase: 117

- Phase 116 ⬜ realizes D26 — catalog grok-4.6 and rewrite grok-4.5 cells
