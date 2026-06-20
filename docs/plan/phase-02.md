# Phase 2 — Error model
*Realizes design Decision 7 (the error model). Depends on Phase 1.*

The thirteen sentinel category vars, the rich `*Error` struct (all fields), and its `Error()`/`Is`/`Unwrap` methods exist, so `errors.Is(err, ErrX)` is the single branching idiom over provider failures. The per-provider classification matrix, the verbatim-`Raw` capture, MCP attribution, and the `*Error`-versus-bare-sentinel distinction are proven later, where the producing code and the other sentinel families exist.

**Done when:** R-BVYY-B2AX is covered (`errors.Is` returns true for a matching sentinel and false for a non-matching one); suite green.

