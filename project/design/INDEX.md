# AgentKit — Design Index

The manifest for the split design. Each Decision lives in its own `project/design/DNN.md` (zero-padded filename; referenced in prose and the plan as `D<N>`, e.g. `D5`). `project/design/README.md` holds only the invariant spine — Authority, the *Requirement ids* convention, and *Conventions*. This index maps every Decision **and** every `R-XXXX-XXXX` Verification id to its file, so the build loop (and any reader) jumps straight to the one Decision a phase realizes without loading the whole design for `github.com/ikigenba/agentkit`. Resolve a Decision by its number below; resolve an id either by grepping this index (`grep -n R-ZWV0-CY54 project/design/INDEX.md`) or the files directly (`grep -rl R-ZWV0-CY54 project/design/`). Rewrite-in-place authority is unchanged: when a Decision changes it is rewritten in its `DNN.md`; this index is regenerated alongside. Decision numbering is not contiguous — there is no Decision 14 (a real gap; numbers are never reused).

## Decisions
One line per Decision, in number order — file, label, and the Verification ids it owns.
- **D1** `project/design/D01.md` — The consumer surface: the conversation object and the turn verb
  - ids: R-ZWV0-CY54, R-ZELD-OQNG, R-ZZAT-4HMI, R-00IP-I9D7
- **D2** `project/design/D02.md` — The consumption surface: `Stream` and the message-granular `Event` taxonomy
  - ids: R-HUZX-7N2W, R-C8UE-VJ67, R-CBA7-N2NL, R-CCI4-0UEA, R-CDQ0-EM4Z
- **D3** `project/design/D03.md` — The canonical message & block data model
  - ids: R-IKKQ-Z3B4, R-ILSN-CV1T, R-IN0J-QMSI, R-IO8G-4EJ7, R-IPGC-I69W, R-XW08-D4YL
- **D4** `project/design/D04.md` — The tool definition & registration surface
  - ids: R-WYZP-N2VB, R-XZ8K-R3NX, R-Y0GH-4VEM, R-Y1OD-IN5B, R-DIVW-07P0, R-AIWI-P5JP, R-Y2W9-WEW0, R-X07M-0UM0, R-X1FI-EMCP, R-SZH4-PB1G
- **D5** `project/design/D05.md` — Provider packaging, construction credentials, and free-flow model selection
  - ids: R-CQO3-7EE9, R-CRVZ-L64Y, R-CT3V-YXVN, R-H65D-HYXH
- **D6** `project/design/D06.md` — Generation settings and the native reasoning value
  - ids: R-P5U3-5CFZ, R-CUBS-CPMC, R-T40A-VZQ7, R-T587-9RGW, R-T6G3-NJ7L, R-DCOZ-8W8U, R-DDWV-MNZJ, R-DF4S-0FQ8, R-DGCO-E7GX, R-P9HS-ANO2, R-PBXL-275G
- **D7** `project/design/D07.md` — The error model
  - ids: R-LMGA-0UF2, R-BUR1-XAK8, R-FR35-46U7, R-BVYY-B2AX, R-BX6U-OU1M, R-BYER-2LSB, R-BZMN-GDJ0, R-I5VJ-CTXE, R-7CYE-KS40, R-6TQA-QKYI, R-6UY7-4CP7
- **D8** `project/design/D08.md` — The uniform `Usage` struct (disjoint token buckets)
  - ids: R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7
- **D9** `project/design/D09.md` — Package architecture & the provider adapter seam (SPI)
  - ids: R-LK0H-9AXO, R-01HL-I6TM, R-02PH-VYKB, R-OUE3-L8VS, R-03XE-9QB0, R-055A-NI1P, R-XR4M-U1ZT, R-OMKB-AY19, R-UJNS-PFLL, R-ZCMP-ARG8, R-DNS8-QC6Z, R-DRFX-VNF2, R-DTVQ-N6WG, R-GSIG-PT07, R-TQ77-6QLK, R-T06O-8SZX, R-CVJO-QHD1, R-CXZH-I0UF, R-4YSE-6YBS
- **D10** `project/design/D10.md` — The orchestration layer: tool loop, history, transparency, reasoning replay, cache-prefix stability
  - ids: R-VV9Y-GMKH, R-VWHU-UEB6, R-VXPR-861V, R-VYXN-LXSK, R-W05J-ZPJ9, R-W1DG-DH9Y, R-W2LC-R90N, R-XZNX-IG6O, R-Y4JJ-1J5G, R-6W63-I4FW
- **D11** `project/design/D11.md` — Retry & backoff policy
  - ids: R-P3LQ-QY2X, R-P4TN-4PTM, R-P61J-IHKB, R-Y878-6UDJ, R-P79F-W9B0, R-P8HC-A11P, R-6XDZ-VW6L, R-6YLW-9NXA
- **D12** `project/design/D12.md` — Raw HTTP, not wrapped SDKs
  - ids: none — structural
- **D13** `project/design/D13.md` — Testing strategy
  - ids: R-WJLM-7QRP, R-WKTI-LIIE, R-WM1E-ZA93, R-711P-17EO, R-CL9K-41F1, R-CMHG-HT5Q, R-CNPC-VKWF, R-Y5C2-NYDE, R-COX9-9CN4, R-CQ55-N4DT, R-CRD2-0W4I, R-CSKY-ENV7, R-CTSU-SFLW
- **D15** `project/design/D15.md` — Structured JSONL event log & conversation lifecycle
  - ids: R-PH7W-BVH0, R-LNO6-EM5R, R-PIFS-PN7P, R-PJNP-3EYE, R-PKVL-H6P3, R-PM3H-UYFS, R-PNBE-8Q6H, R-POJA-MHX6, R-PPR7-09NV
- **D16** `project/design/D16.md` — Dollar-cost accounting: the resolution seam
  - ids: R-CZ7D-VSL4, R-D0FA-9KBT, R-D1N6-NC2I, R-V2SM-WC8V, R-PVUO-X4DC, R-PX2L-AW41
- **D17** `project/design/D17.md` — MCP servers as a tool source
  - ids: R-6GBE-J3SV, R-6HJA-WVJK, R-6IR7-ANA9, R-6L70-26RN, R-6MEW-FYIC, R-6NMS-TQ91, R-6OUP-7HZQ, R-6Q2L-L9QF, R-6RAH-Z1H4, R-6SIE-CT7T
- **D18** `project/design/D18.md` — The embeddings consumer surface: the `Embedder` object and the `Embed` verb
  - ids: R-Y5RV-WB3T, R-Y6ZS-A2UI, R-D5AV-SNAL, R-Y9FL-1MBW, R-YANH-FE2L, R-YBVD-T5TA, R-D6IS-6F1A, R-D2V3-13T7, R-D42Z-EVJW
- **D19** `project/design/D19.md` — The `EmbeddingProvider` SPI, package architecture & adapter-owned guarantees
  - ids: R-D7QO-K6RZ, R-LL8D-N2OD, R-YHYV-Q0IR, R-YJ6S-3S9G, R-YKEO-HK05, R-YLMK-VBQU, R-YO2D-MV88
- **D20** `project/design/D20.md` — Embedding usage & pricing: the data shapes
  - ids: R-YPAA-0MYX, R-D8YK-XYIO
- **D21** `project/design/D21.md` — The shared retry executor (`internal/retry`)
  - ids: R-IUBG-95CC, R-IWR9-0OTQ, R-IXZ5-EGKF, R-IZ71-S8B4, R-J0EY-601T
- **D22** `project/design/D22.md` — Per-provider schema rendering at the adapter boundary
  - ids: R-XT52-U8YG, R-XUCZ-80P5, R-XVKV-LSFU, R-XWSR-ZK6J, R-XY0O-DBX8, R-2UV8-RBKS, R-2W35-53BH, R-2XB1-IV26
- **D23** `project/design/D23.md` — Deferred tools & the built-in `load_tools` meta-tool
  - ids: R-9RQ8-9G3W, R-9SY4-N7UL, R-D5PT-82VU, R-D6XP-LUMJ, R-D85L-ZMD8, R-D9DI-DE3X, R-DALE-R5UM, R-DE93-WH2P, R-DBTB-4XLB, R-DD17-IPC0, R-B5BR-U5M1, R-B6JO-7XCQ, R-B7RK-LP3F, R-DFH0-A8TE
- **D24** `project/design/D24.md` — The `openrouter` provider
  - ids: R-DA6H-BQ9D, R-DBED-PI02, R-DCMA-39QR, R-DF22-UT85
- **D25** `project/design/D25.md` — OpenAI credentials: API key and ChatGPT subscription
  - ids: R-DG9Z-8KYU, R-PV6N-URM6, R-PWEK-8JCV, R-DHHV-MCPJ, R-PXMG-MB3K, R-DL5K-RNXM, R-DJXO-DW6X
- **D26** `project/design/D26.md` — The advisory model catalog: `agentkit/catalog`
  - ids: R-DMDH-5FOB, R-LOW2-SDWG, R-LRBV-JXDU, R-DNLD-J7F0, R-E7VN-JTZ0, R-LXFD-GS3B, R-LTRO-BGV8, R-LW7H-30CM, R-E5FU-SAHM, R-EBJC-P573, R-DOT9-WZ5P, R-DQ16-AQWE, R-DHKK-RZ7M, R-DISH-5QYB, R-DK0D-JIP0, R-EABG-BDGE, R-E6NR-628B, R-DNO2-OTX3, R-LYN9-UJU0, R-DOVZ-2LNS, R-DR92-OIN3, R-4NJ4-SJ41, R-DSGZ-2ADS
- **D27** `project/design/D27.md` — The `toolkit` subpackage: standard coding tools, root confinement, output cap
  - ids: R-LQ1Y-XASG, R-LR9V-B2J5, R-Y446-A6MP, R-LSHR-OU9U, R-LTPO-2M0J, R-LUXK-GDR8, R-LW5G-U5HX
- **D28** `project/design/D28.md` — `toolkit`: `Read`, `Write`, `Edit` semantics
  - ids: R-LXDD-7X8M, R-LYL9-LOZB, R-LZT5-ZGQ0, R-M28Y-R07E, R-M3GV-4RY3, R-M4OR-IJOS, R-M5WN-WBFH, R-M74K-A366, R-VGH5-GE0Q, R-VHP1-U5RF, R-VIWY-7XI4, R-VK4U-LP8T
- **D29** `project/design/D29.md` — `toolkit`: `Bash` semantics
  - ids: R-M8CG-NUWV, R-M9KD-1MNK, R-MAS9-FEE9, R-MC05-T64Y, R-MD82-6XVN
- **D30** `project/design/D30.md` — `toolkit`: `Glob` and `Grep` semantics
  - ids: R-MEFY-KPMC, R-MFNU-YHD1, R-MGVR-C93Q, R-MJBK-3SL4, R-MKJG-HKBT, R-MLRC-VC2I, R-MMZ9-93T7, R-MO75-MVJW, R-MPF2-0NAL
- **D31** `project/design/D31.md` — The `ocr` package: the document-text tool, its cache, and its transcript
  - ids: R-V1UC-V54E, R-V329-8WV3, R-V4A5-MOLS, R-UTL6-Q86Y, R-V6PY-E836, R-V7XU-RZTV, R-V95R-5RKK, R-UW0Z-HROC, R-UX8V-VJF1, R-VADN-JJB9, R-VBLJ-XB1Y, R-VCTG-B2SN, R-VE1C-OUJC, R-VF99-2MA1
- **D32** `project/design/D32.md` — OpenRouter document parsing: the request, the response contract, and `Transcript`
  - ids: R-UQV9-F7G5, R-US35-SZ6U, R-GMLN-XC8W, R-UTB2-6QXJ, R-UUIY-KIO8, R-UVQU-YAEX, R-UWYR-C25M, R-UY6N-PTWB, R-UZEK-3LN0, R-V0MG-HDDP
- **D33** `project/design/D33.md` — Raster normalization: images become one-page PDFs
  - ids: R-UJJV-4KZZ, R-UKRR-ICQO, R-ULZN-W4HD, R-UOFG-NNYR, R-UPND-1FPG
- **D34** `project/design/D34.md` — The canonical tool-schema subset, owned by root
  - ids: R-XLTO-JMIA, R-XRX6-GH7R, R-XN1K-XE8Z, R-XO9H-B5ZO, R-XQPA-2PH2, R-ZPPN-6FV9, R-6QNT-WR7C, R-U3C5-4A1V
- **D35** `project/design/D35.md` — The assembled message is transport-shape-independent
  - ids: R-QUWY-MLCD, R-QW4V-0D32, R-QXCR-E4TR, R-QYKN-RWKG, R-QZSK-5OB5, R-R10G-JG1U

## Verification ids → Decision
R-00IP-I9D7  D1  project/design/D01.md
R-01HL-I6TM  D9  project/design/D09.md
R-02PH-VYKB  D9  project/design/D09.md
R-03XE-9QB0  D9  project/design/D09.md
R-055A-NI1P  D9  project/design/D09.md
R-2UV8-RBKS  D22  project/design/D22.md
R-2W35-53BH  D22  project/design/D22.md
R-2XB1-IV26  D22  project/design/D22.md
R-4NJ4-SJ41  D26  project/design/D26.md
R-4YSE-6YBS  D9  project/design/D09.md
R-6GBE-J3SV  D17  project/design/D17.md
R-6HJA-WVJK  D17  project/design/D17.md
R-6IR7-ANA9  D17  project/design/D17.md
R-6L70-26RN  D17  project/design/D17.md
R-6MEW-FYIC  D17  project/design/D17.md
R-6NMS-TQ91  D17  project/design/D17.md
R-6OUP-7HZQ  D17  project/design/D17.md
R-6Q2L-L9QF  D17  project/design/D17.md
R-6QNT-WR7C  D34  project/design/D34.md
R-6RAH-Z1H4  D17  project/design/D17.md
R-6SIE-CT7T  D17  project/design/D17.md
R-6TQA-QKYI  D7  project/design/D07.md
R-6UY7-4CP7  D7  project/design/D07.md
R-6W63-I4FW  D10  project/design/D10.md
R-6XDZ-VW6L  D11  project/design/D11.md
R-6YLW-9NXA  D11  project/design/D11.md
R-711P-17EO  D13  project/design/D13.md
R-7CYE-KS40  D7  project/design/D07.md
R-9RQ8-9G3W  D23  project/design/D23.md
R-9SY4-N7UL  D23  project/design/D23.md
R-AIWI-P5JP  D4  project/design/D04.md
R-B5BR-U5M1  D23  project/design/D23.md
R-B6JO-7XCQ  D23  project/design/D23.md
R-B7RK-LP3F  D23  project/design/D23.md
R-BUR1-XAK8  D7  project/design/D07.md
R-BVYY-B2AX  D7  project/design/D07.md
R-BX6U-OU1M  D7  project/design/D07.md
R-BYER-2LSB  D7  project/design/D07.md
R-BZMN-GDJ0  D7  project/design/D07.md
R-C8UE-VJ67  D2  project/design/D02.md
R-CBA7-N2NL  D2  project/design/D02.md
R-CCI4-0UEA  D2  project/design/D02.md
R-CDQ0-EM4Z  D2  project/design/D02.md
R-CL9K-41F1  D13  project/design/D13.md
R-CMHG-HT5Q  D13  project/design/D13.md
R-CNPC-VKWF  D13  project/design/D13.md
R-COX9-9CN4  D13  project/design/D13.md
R-CQ55-N4DT  D13  project/design/D13.md
R-CQO3-7EE9  D5  project/design/D05.md
R-CRD2-0W4I  D13  project/design/D13.md
R-CRVZ-L64Y  D5  project/design/D05.md
R-CSKY-ENV7  D13  project/design/D13.md
R-CT3V-YXVN  D5  project/design/D05.md
R-CTSU-SFLW  D13  project/design/D13.md
R-CUBS-CPMC  D6  project/design/D06.md
R-CVJO-QHD1  D9  project/design/D09.md
R-CXZH-I0UF  D9  project/design/D09.md
R-CZ7D-VSL4  D16  project/design/D16.md
R-D0FA-9KBT  D16  project/design/D16.md
R-D1N6-NC2I  D16  project/design/D16.md
R-D2V3-13T7  D18  project/design/D18.md
R-D42Z-EVJW  D18  project/design/D18.md
R-D5AV-SNAL  D18  project/design/D18.md
R-D5PT-82VU  D23  project/design/D23.md
R-D6IS-6F1A  D18  project/design/D18.md
R-D6XP-LUMJ  D23  project/design/D23.md
R-D7QO-K6RZ  D19  project/design/D19.md
R-D85L-ZMD8  D23  project/design/D23.md
R-D8YK-XYIO  D20  project/design/D20.md
R-D9DI-DE3X  D23  project/design/D23.md
R-DA6H-BQ9D  D24  project/design/D24.md
R-DALE-R5UM  D23  project/design/D23.md
R-DBED-PI02  D24  project/design/D24.md
R-DBTB-4XLB  D23  project/design/D23.md
R-DCMA-39QR  D24  project/design/D24.md
R-DCOZ-8W8U  D6  project/design/D06.md
R-DD17-IPC0  D23  project/design/D23.md
R-DDWV-MNZJ  D6  project/design/D06.md
R-DE93-WH2P  D23  project/design/D23.md
R-DF22-UT85  D24  project/design/D24.md
R-DF4S-0FQ8  D6  project/design/D06.md
R-DFH0-A8TE  D23  project/design/D23.md
R-DG9Z-8KYU  D25  project/design/D25.md
R-DGCO-E7GX  D6  project/design/D06.md
R-DHHV-MCPJ  D25  project/design/D25.md
R-DHKK-RZ7M  D26  project/design/D26.md
R-DISH-5QYB  D26  project/design/D26.md
R-DIVW-07P0  D4  project/design/D04.md
R-DJXO-DW6X  D25  project/design/D25.md
R-DK0D-JIP0  D26  project/design/D26.md
R-DL5K-RNXM  D25  project/design/D25.md
R-DMDH-5FOB  D26  project/design/D26.md
R-DNLD-J7F0  D26  project/design/D26.md
R-DNO2-OTX3  D26  project/design/D26.md
R-DNS8-QC6Z  D9  project/design/D09.md
R-DOT9-WZ5P  D26  project/design/D26.md
R-DOVZ-2LNS  D26  project/design/D26.md
R-DQ16-AQWE  D26  project/design/D26.md
R-DR92-OIN3  D26  project/design/D26.md
R-DRFX-VNF2  D9  project/design/D09.md
R-DSGZ-2ADS  D26  project/design/D26.md
R-DTVQ-N6WG  D9  project/design/D09.md
R-E5FU-SAHM  D26  project/design/D26.md
R-E6NR-628B  D26  project/design/D26.md
R-E7VN-JTZ0  D26  project/design/D26.md
R-EABG-BDGE  D26  project/design/D26.md
R-EBJC-P573  D26  project/design/D26.md
R-FR35-46U7  D7  project/design/D07.md
R-GMLN-XC8W  D32  project/design/D32.md
R-GSIG-PT07  D9  project/design/D09.md
R-H65D-HYXH  D5  project/design/D05.md
R-HUZX-7N2W  D2  project/design/D02.md
R-I5VJ-CTXE  D7  project/design/D07.md
R-IKKQ-Z3B4  D3  project/design/D03.md
R-ILSN-CV1T  D3  project/design/D03.md
R-IN0J-QMSI  D3  project/design/D03.md
R-IO8G-4EJ7  D3  project/design/D03.md
R-IPGC-I69W  D3  project/design/D03.md
R-IUBG-95CC  D21  project/design/D21.md
R-IWR9-0OTQ  D21  project/design/D21.md
R-IXZ5-EGKF  D21  project/design/D21.md
R-IZ71-S8B4  D21  project/design/D21.md
R-J0EY-601T  D21  project/design/D21.md
R-LK0H-9AXO  D9  project/design/D09.md
R-LL8D-N2OD  D19  project/design/D19.md
R-LMGA-0UF2  D7  project/design/D07.md
R-LNO6-EM5R  D15  project/design/D15.md
R-LOW2-SDWG  D26  project/design/D26.md
R-LQ1Y-XASG  D27  project/design/D27.md
R-LR9V-B2J5  D27  project/design/D27.md
R-LRBV-JXDU  D26  project/design/D26.md
R-LSHR-OU9U  D27  project/design/D27.md
R-LTPO-2M0J  D27  project/design/D27.md
R-LTRO-BGV8  D26  project/design/D26.md
R-LUXK-GDR8  D27  project/design/D27.md
R-LW5G-U5HX  D27  project/design/D27.md
R-LW7H-30CM  D26  project/design/D26.md
R-LXDD-7X8M  D28  project/design/D28.md
R-LXFD-GS3B  D26  project/design/D26.md
R-LYL9-LOZB  D28  project/design/D28.md
R-LYN9-UJU0  D26  project/design/D26.md
R-LZT5-ZGQ0  D28  project/design/D28.md
R-M28Y-R07E  D28  project/design/D28.md
R-M3GV-4RY3  D28  project/design/D28.md
R-M4OR-IJOS  D28  project/design/D28.md
R-M5WN-WBFH  D28  project/design/D28.md
R-M74K-A366  D28  project/design/D28.md
R-M8CG-NUWV  D29  project/design/D29.md
R-M9KD-1MNK  D29  project/design/D29.md
R-MAS9-FEE9  D29  project/design/D29.md
R-MC05-T64Y  D29  project/design/D29.md
R-MD82-6XVN  D29  project/design/D29.md
R-MEFY-KPMC  D30  project/design/D30.md
R-MFNU-YHD1  D30  project/design/D30.md
R-MGVR-C93Q  D30  project/design/D30.md
R-MJBK-3SL4  D30  project/design/D30.md
R-MKJG-HKBT  D30  project/design/D30.md
R-MLRC-VC2I  D30  project/design/D30.md
R-MMZ9-93T7  D30  project/design/D30.md
R-MO75-MVJW  D30  project/design/D30.md
R-MPF2-0NAL  D30  project/design/D30.md
R-OMKB-AY19  D9  project/design/D09.md
R-OUE3-L8VS  D9  project/design/D09.md
R-P3LQ-QY2X  D11  project/design/D11.md
R-P4TN-4PTM  D11  project/design/D11.md
R-P5U3-5CFZ  D6  project/design/D06.md
R-P61J-IHKB  D11  project/design/D11.md
R-P79F-W9B0  D11  project/design/D11.md
R-P8HC-A11P  D11  project/design/D11.md
R-P9HS-ANO2  D6  project/design/D06.md
R-PBXL-275G  D6  project/design/D06.md
R-PH7W-BVH0  D15  project/design/D15.md
R-PIFS-PN7P  D15  project/design/D15.md
R-PJNP-3EYE  D15  project/design/D15.md
R-PKVL-H6P3  D15  project/design/D15.md
R-PM3H-UYFS  D15  project/design/D15.md
R-PNBE-8Q6H  D15  project/design/D15.md
R-POJA-MHX6  D15  project/design/D15.md
R-PPR7-09NV  D15  project/design/D15.md
R-PV6N-URM6  D25  project/design/D25.md
R-PVUO-X4DC  D16  project/design/D16.md
R-PWEK-8JCV  D25  project/design/D25.md
R-PX2L-AW41  D16  project/design/D16.md
R-PXMG-MB3K  D25  project/design/D25.md
R-QUWY-MLCD  D35  project/design/D35.md
R-QW4V-0D32  D35  project/design/D35.md
R-QXCR-E4TR  D35  project/design/D35.md
R-QYKN-RWKG  D35  project/design/D35.md
R-QZSK-5OB5  D35  project/design/D35.md
R-R10G-JG1U  D35  project/design/D35.md
R-SZH4-PB1G  D4  project/design/D04.md
R-T06O-8SZX  D9  project/design/D09.md
R-T40A-VZQ7  D6  project/design/D06.md
R-T587-9RGW  D6  project/design/D06.md
R-T6G3-NJ7L  D6  project/design/D06.md
R-TQ77-6QLK  D9  project/design/D09.md
R-U3C5-4A1V  D34  project/design/D34.md
R-UJJV-4KZZ  D33  project/design/D33.md
R-UJNS-PFLL  D9  project/design/D09.md
R-UKRR-ICQO  D33  project/design/D33.md
R-ULZN-W4HD  D33  project/design/D33.md
R-UOFG-NNYR  D33  project/design/D33.md
R-UPND-1FPG  D33  project/design/D33.md
R-UQV9-F7G5  D32  project/design/D32.md
R-US35-SZ6U  D32  project/design/D32.md
R-UTB2-6QXJ  D32  project/design/D32.md
R-UTL6-Q86Y  D31  project/design/D31.md
R-UUIY-KIO8  D32  project/design/D32.md
R-UVQU-YAEX  D32  project/design/D32.md
R-UW0Z-HROC  D31  project/design/D31.md
R-UWYR-C25M  D32  project/design/D32.md
R-UX8V-VJF1  D31  project/design/D31.md
R-UY6N-PTWB  D32  project/design/D32.md
R-UZEK-3LN0  D32  project/design/D32.md
R-V0MG-HDDP  D32  project/design/D32.md
R-V1UC-V54E  D31  project/design/D31.md
R-V2SM-WC8V  D16  project/design/D16.md
R-V329-8WV3  D31  project/design/D31.md
R-V4A5-MOLS  D31  project/design/D31.md
R-V6PY-E836  D31  project/design/D31.md
R-V7XU-RZTV  D31  project/design/D31.md
R-V95R-5RKK  D31  project/design/D31.md
R-VADN-JJB9  D31  project/design/D31.md
R-VBLJ-XB1Y  D31  project/design/D31.md
R-VCTG-B2SN  D31  project/design/D31.md
R-VE1C-OUJC  D31  project/design/D31.md
R-VF99-2MA1  D31  project/design/D31.md
R-VGH5-GE0Q  D28  project/design/D28.md
R-VHP1-U5RF  D28  project/design/D28.md
R-VIWY-7XI4  D28  project/design/D28.md
R-VK4U-LP8T  D28  project/design/D28.md
R-VV9Y-GMKH  D10  project/design/D10.md
R-VWHU-UEB6  D10  project/design/D10.md
R-VXPR-861V  D10  project/design/D10.md
R-VYXN-LXSK  D10  project/design/D10.md
R-W05J-ZPJ9  D10  project/design/D10.md
R-W1DG-DH9Y  D10  project/design/D10.md
R-W2LC-R90N  D10  project/design/D10.md
R-WJLM-7QRP  D13  project/design/D13.md
R-WKTI-LIIE  D13  project/design/D13.md
R-WM1E-ZA93  D13  project/design/D13.md
R-WYZP-N2VB  D4  project/design/D04.md
R-X07M-0UM0  D4  project/design/D04.md
R-X1FI-EMCP  D4  project/design/D04.md
R-XLTO-JMIA  D34  project/design/D34.md
R-XN1K-XE8Z  D34  project/design/D34.md
R-XO9H-B5ZO  D34  project/design/D34.md
R-XQPA-2PH2  D34  project/design/D34.md
R-XR4M-U1ZT  D9  project/design/D09.md
R-XRX6-GH7R  D34  project/design/D34.md
R-XT52-U8YG  D22  project/design/D22.md
R-XUCZ-80P5  D22  project/design/D22.md
R-XVKV-LSFU  D22  project/design/D22.md
R-XW08-D4YL  D3  project/design/D03.md
R-XWSR-ZK6J  D22  project/design/D22.md
R-XY0O-DBX8  D22  project/design/D22.md
R-XZ8K-R3NX  D4  project/design/D04.md
R-XZNX-IG6O  D10  project/design/D10.md
R-Y0GH-4VEM  D4  project/design/D04.md
R-Y1OD-IN5B  D4  project/design/D04.md
R-Y2W9-WEW0  D4  project/design/D04.md
R-Y446-A6MP  D27  project/design/D27.md
R-Y4JJ-1J5G  D10  project/design/D10.md
R-Y5C2-NYDE  D13  project/design/D13.md
R-Y5RV-WB3T  D18  project/design/D18.md
R-Y6ZS-A2UI  D18  project/design/D18.md
R-Y810-TECF  D8  project/design/D08.md
R-Y878-6UDJ  D11  project/design/D11.md
R-Y98X-7634  D8  project/design/D08.md
R-Y9FL-1MBW  D18  project/design/D18.md
R-YAGT-KXTT  D8  project/design/D08.md
R-YANH-FE2L  D18  project/design/D18.md
R-YBOP-YPKI  D8  project/design/D08.md
R-YBVD-T5TA  D18  project/design/D18.md
R-YCWM-CHB7  D8  project/design/D08.md
R-YHYV-Q0IR  D19  project/design/D19.md
R-YJ6S-3S9G  D19  project/design/D19.md
R-YKEO-HK05  D19  project/design/D19.md
R-YLMK-VBQU  D19  project/design/D19.md
R-YO2D-MV88  D19  project/design/D19.md
R-YPAA-0MYX  D20  project/design/D20.md
R-ZCMP-ARG8  D9  project/design/D09.md
R-ZELD-OQNG  D1  project/design/D01.md
R-ZPPN-6FV9  D34  project/design/D34.md
R-ZWV0-CY54  D1  project/design/D01.md
R-ZZAT-4HMI  D1  project/design/D01.md
