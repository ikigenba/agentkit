# Phase 70 — `toolkit` package: skeleton, confinement, output cap, and the file tools

*Realizes design Decision 27 (`toolkit` subpackage: constructors, confinement, cap) and 28 (`Read`/`Write`/`Edit` semantics).*

The new leaf subpackage `toolkit/` exists at the module root with the six per-tool constructors and `All(root)` (Bash/Glob/Grep may be stubs that construct and declare correctly but whose full semantics land in Phase 71 — their D29/D30 behaviors are not this phase's bar). Fully working in this phase: root resolution (`""` = process cwd), symlink-aware hard confinement shared by all file tools, the uniform 30,000-character result cap with its truncation marker, and the complete `Read`, `Write`, and `Edit` tools per D28 — including the ambiguous-edit refusal.

**Done when:** the suite is green (per design Conventions) and each of these ids is covered by a clearly-named tagged test:

- R-LQ1Y-XASG — `All(root)` returns the six tools, correctly named, in order.
- R-LR9V-B2J5 — each per-tool constructor's tool carries its matching name.
- R-LSHR-OU9U — relative escapes are rejected by every file tool with no effect outside the root.
- R-LTPO-2M0J — symlink escapes are rejected.
- R-LUXK-GDR8 — empty root resolves against the process working directory.
- R-LW5G-U5HX — results over 30,000 characters are cut with the truncation marker; smaller results pass unmarked.
- R-LXDD-7X8M — `Read` returns exact contents.
- R-LYL9-LOZB — `Read` offset/limit line slicing.
- R-LZT5-ZGQ0 — `Read` of a missing file errors.
- R-M28Y-R07E — `Write` creates parents and the file with exact content.
- R-M3GV-4RY3 — `Edit` replaces a unique occurrence exactly.
- R-M4OR-IJOS — `Edit` errors when `old_string` is absent, file unchanged.
- R-M5WN-WBFH — `Edit` errors on ambiguous `old_string` without `replace_all`, file unchanged.
- R-M74K-A366 — `Edit` `replace_all` replaces every occurrence and reports the count.
