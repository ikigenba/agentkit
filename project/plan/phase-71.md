# Phase 71 — `toolkit`: `Bash`, `Glob`, and `Grep`

*Realizes design Decision 29 (`Bash` semantics) and 30 (`Glob`/`Grep` semantics). Depends on Phase 70.*

The remaining three tools reach their full D29/D30 semantics on the Phase-70 skeleton: `Bash` with combined output, the `[exit status N]` normal-result contract, the millisecond timeout defaulting to 120,000, and process-group kill; `Glob` with plain and recursive `**` matching, sorted base-relative JSON results, and `.git` exclusion; `Grep` with regexp line matching over a file or walked directory, base-name glob filtering, `.git` exclusion, and NUL-heuristic binary skipping. All three flow through Phase 70's confinement (Glob/Grep paths) and output cap.

**Done when:** the suite is green (per design Conventions) and each of these ids is covered by a clearly-named tagged test:

- R-M8CG-NUWV — `bash -c` from the root with combined stdout+stderr.
- R-M9KD-1MNK — nonzero exit yields output plus `[exit status N]` as a normal result.
- R-MAS9-FEE9 — a timed-out command is killed and the result reports the timeout with captured output.
- R-MC05-T64Y — the timeout kill takes the whole process group (a spawned background child dies).
- R-MD82-6XVN — a blank command errors without spawning.
- R-MEFY-KPMC — plain glob: sorted, base-relative JSON matches.
- R-MFNU-YHD1 — `**` glob matches at every depth.
- R-MGVR-C93Q — glob walk skips `.git`, not `.github`.
- R-MJBK-3SL4 — no matches yields `[]`, not an error.
- R-MKJG-HKBT — grep returns sorted `file:line:text` JSON matches.
- R-MLRC-VC2I — grep single-file `path` and base-name `glob` filtering.
- R-MMZ9-93T7 — grep skips `.git` contents.
- R-MO75-MVJW — grep skips NUL-detected binary files.
- R-MPF2-0NAL — an invalid regexp errors.
