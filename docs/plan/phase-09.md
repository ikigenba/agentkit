# Phase 9 — OpenAI adapter (Responses API)
*Realizes design Decision 9, Decision 7, Decision 8, Decision 16, Decision 3, Decision 6, and Decision 13 (the OpenAI slice). Depends on Phases 5 through 8 (reuses `internal/sse`, `internal/httpx`, and the harness).*

The `openai` sub-package implements the SPI over the Responses API on raw `net/http`: every request carries `store:false` and `include:["reasoning.encrypted_content"]` (fixed, never a consumer knob), the returned `encrypted_content` populates `ReasoningBlock.Opaque` and replays verbatim, `call_id` is normalized into the neutral ID, SSE parse with central assembly, usage mapping (`InputUncached`=prompt−cached, `Output`=output−reasoning, `ReasoningOutput` populated, native total asserted == bucket sum), error classification, and the registry + pricing (gpt-5.5-pro with `CacheReadInput`==`InputUncached`, tiered gpt-5.5 and gpt-5.4, gpt-5.4-mini, gpt-5.4-nano) with exported constants.

**Done when:** (OpenAI slices): R-H3PK-QFG3, R-XR4M-U1ZT, R-BUR1-XAK8, R-BX6U-OU1M, R-BYER-2LSB, R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7, R-VDY4-AP7H, R-V1KQ-IKI6, R-XW08-D4YL, R-055A-NI1P, R-P5U3-5CFZ, R-P71Z-J46O, R-C8UE-VJ67 (OpenAI assembly slice), R-V2SM-WC8V (gpt tiered slice) are covered; suite green.

