# AgentKit — Design Index
The manifest for the split design. Each Decision lives in its own `project/design/DNN.md` (zero-padded filename; referenced in prose and the plan as `D<N>`, e.g. `D5`). `project/design/design.md` holds only the invariant spine — Authority, the *Requirement ids* convention, and *Conventions*. This index maps every Decision **and** every `R-XXXX-XXXX` Verification id to its file, so the build loop (and any reader) jumps straight to the one Decision a phase realizes without loading the whole design for `github.com/ikigenba/agentkit`. Resolve a Decision by its number below; resolve an id either by grepping this index (`grep -n R-ZWV0-CY54 project/design/INDEX.md`) or the files directly (`grep -rl R-ZWV0-CY54 project/design/`). Append-in-place authority is unchanged: when a Decision changes it is rewritten in its `DNN.md`; this index is regenerated alongside. Decision numbering is not contiguous — there is no Decision 14 (a real gap; numbers are never reused).
## Decisions
One line per Decision, in number order — file, label, and the Verification ids it owns.
- **D1** `project/design/D01.md` — The consumer surface: the conversation object and the turn verb
  - ids: R-ZWV0-CY54, R-ZELD-OQNG, R-ZZAT-4HMI, R-00IP-I9D7
- **D2** `project/design/D02.md` — The consumption surface: `Stream` and the message-granular `Event` taxonomy
  - ids: R-HUZX-7N2W, R-C8UE-VJ67, R-CBA7-N2NL, R-CCI4-0UEA, R-CDQ0-EM4Z
- **D3** `project/design/D03.md` — The canonical message & block data model
  - ids: R-IKKQ-Z3B4, R-ILSN-CV1T, R-IN0J-QMSI, R-IO8G-4EJ7, R-IPGC-I69W, R-XW08-D4YL
- **D4** `project/design/D04.md` — The tool definition & registration surface
  - ids: R-WYZP-N2VB, R-X07M-0UM0, R-X1FI-EMCP, R-X2NE-SE3E, R-SX1B-XRK2, R-SZH4-PB1G, R-X3VB-65U3, R-6ZTS-NFNZ
- **D5** `project/design/D05.md` — Provider packaging, selection, and credential placement
  - ids: R-H3PK-QFG3, R-H4XH-476S, R-H65D-HYXH, R-7GGH-BPYN
- **D6** `project/design/D06.md` — Generation settings and the native reasoning value
  - ids: R-P5U3-5CFZ, R-B7YX-J342, R-B96T-WUUR, R-T40A-VZQ7, R-T587-9RGW, R-T6G3-NJ7L, R-P89V-WVXD, R-P9HS-ANO2, R-PBXL-275G
- **D7** `project/design/D07.md` — The error model
  - ids: R-BUR1-XAK8, R-BVYY-B2AX, R-BX6U-OU1M, R-BYER-2LSB, R-BZMN-GDJ0, R-I5VJ-CTXE, R-7CYE-KS40, R-6TQA-QKYI, R-6UY7-4CP7, R-FR35-46U7
- **D8** `project/design/D08.md` — The uniform `Usage` struct (disjoint token buckets)
  - ids: R-Y810-TECF, R-Y98X-7634, R-YAGT-KXTT, R-YBOP-YPKI, R-YCWM-CHB7
- **D9** `project/design/D09.md` — Package architecture & the provider adapter seam (SPI)
  - ids: R-01HL-I6TM, R-02PH-VYKB, R-OUE3-L8VS, R-03XE-9QB0, R-055A-NI1P, R-XR4M-U1ZT, R-OMKB-AY19, R-UJNS-PFLL, R-ZCMP-ARG8, R-DNS8-QC6Z, R-DRFX-VNF2, R-DTVQ-N6WG, R-GSIG-PT07, R-TQ77-6QLK, R-T06O-8SZX, R-ELUQ-VJIQ, R-4YSE-6YBS
- **D10** `project/design/D10.md` — The orchestration layer: tool loop, history, transparency, reasoning replay, cache-prefix stability
  - ids: R-VV9Y-GMKH, R-VWHU-UEB6, R-VXPR-861V, R-VYXN-LXSK, R-W05J-ZPJ9, R-W1DG-DH9Y, R-W2LC-R90N, R-XZNX-IG6O, R-Y4JJ-1J5G, R-6W63-I4FW
- **D11** `project/design/D11.md` — Retry & backoff policy
  - ids: R-P3LQ-QY2X, R-P4TN-4PTM, R-P61J-IHKB, R-Y878-6UDJ, R-P79F-W9B0, R-P8HC-A11P, R-6XDZ-VW6L, R-6YLW-9NXA
- **D12** `project/design/D12.md` — Raw HTTP, not wrapped SDKs
  - ids: (none — structural)
- **D13** `project/design/D13.md` — Testing strategy
  - ids: R-WJLM-7QRP, R-WKTI-LIIE, R-WM1E-ZA93, R-711P-17EO
- **D15** `project/design/D15.md` — Structured JSONL event log & conversation lifecycle
  - ids: R-PH7W-BVH0, R-PIFS-PN7P, R-PJNP-3EYE, R-PKVL-H6P3, R-PM3H-UYFS, R-PNBE-8Q6H, R-POJA-MHX6, R-PPR7-09NV
- **D16** `project/design/D16.md` — The model registry: pricing, cost, and reasoning introspection
  - ids: R-S6NB-RYUE, R-S7V8-5QL3, R-S934-JIBS, R-PTEW-5KVY, R-V1KQ-IKI6, R-VDY4-AP7H, R-EN2N-9B9F, R-EPIG-0UQT, R-V2SM-WC8V, R-PVUO-X4DC, R-PX2L-AW41
- **D17** `project/design/D17.md` — MCP servers as a tool source
  - ids: R-6GBE-J3SV, R-6HJA-WVJK, R-6IR7-ANA9, R-6L70-26RN, R-6MEW-FYIC, R-6NMS-TQ91, R-6OUP-7HZQ, R-6Q2L-L9QF, R-6RAH-Z1H4, R-6SIE-CT7T
- **D18** `project/design/D18.md` — The embeddings consumer surface: the `Embedder` object and the `Embed` verb
  - ids: R-Y5RV-WB3T, R-Y6ZS-A2UI, R-Y87O-NUL7, R-Y9FL-1MBW, R-YANH-FE2L, R-YBVD-T5TA, R-YD3A-6XJZ, R-YFJ2-YH1D
- **D19** `project/design/D19.md` — The `EmbeddingProvider` SPI, package architecture & adapter-owned guarantees
  - ids: R-YGQZ-C8S2, R-YHYV-Q0IR, R-YJ6S-3S9G, R-YKEO-HK05, R-YLMK-VBQU, R-YMUH-93HJ, R-YO2D-MV88
- **D20** `project/design/D20.md` — The embedding registry: usage, pricing/cost, and capability introspection
  - ids: R-YPAA-0MYX, R-YQI6-EEPM, R-YRQ2-S6GB, R-YSXZ-5Y70, R-YU5V-JPXP, R-YVDR-XHOE, R-YWLO-B9F3
- **D21** `project/design/D21.md` — The shared retry executor (`internal/retry`)
  - ids: R-IUBG-95CC, R-IWR9-0OTQ, R-IXZ5-EGKF, R-IZ71-S8B4, R-J0EY-601T
- **D22** `project/design/D22.md` — Provider-driven tool-schema limits (no provider-name dispatch in the core)
  - ids: R-SKVI-TSZQ, R-SNBB-LCH4, R-SOJ7-Z47T

## Verification ids → Decision
Every minted id, sorted, mapped to its Decision and file (grep target for id lookup).
R-00IP-I9D7  D1  project/design/D01.md
R-01HL-I6TM  D9  project/design/D09.md
R-02PH-VYKB  D9  project/design/D09.md
R-03XE-9QB0  D9  project/design/D09.md
R-055A-NI1P  D9  project/design/D09.md
R-4YSE-6YBS  D9  project/design/D09.md
R-6GBE-J3SV  D17  project/design/D17.md
R-6HJA-WVJK  D17  project/design/D17.md
R-6IR7-ANA9  D17  project/design/D17.md
R-6L70-26RN  D17  project/design/D17.md
R-6MEW-FYIC  D17  project/design/D17.md
R-6NMS-TQ91  D17  project/design/D17.md
R-6OUP-7HZQ  D17  project/design/D17.md
R-6Q2L-L9QF  D17  project/design/D17.md
R-6RAH-Z1H4  D17  project/design/D17.md
R-6SIE-CT7T  D17  project/design/D17.md
R-6TQA-QKYI  D7  project/design/D07.md
R-6UY7-4CP7  D7  project/design/D07.md
R-6W63-I4FW  D10  project/design/D10.md
R-6XDZ-VW6L  D11  project/design/D11.md
R-6YLW-9NXA  D11  project/design/D11.md
R-6ZTS-NFNZ  D4  project/design/D04.md
R-711P-17EO  D13  project/design/D13.md
R-7CYE-KS40  D7  project/design/D07.md
R-7GGH-BPYN  D5  project/design/D05.md
R-B7YX-J342  D6  project/design/D06.md
R-B96T-WUUR  D6  project/design/D06.md
R-BUR1-XAK8  D7  project/design/D07.md
R-BVYY-B2AX  D7  project/design/D07.md
R-BX6U-OU1M  D7  project/design/D07.md
R-BYER-2LSB  D7  project/design/D07.md
R-BZMN-GDJ0  D7  project/design/D07.md
R-C8UE-VJ67  D2  project/design/D02.md
R-CBA7-N2NL  D2  project/design/D02.md
R-CCI4-0UEA  D2  project/design/D02.md
R-CDQ0-EM4Z  D2  project/design/D02.md
R-DNS8-QC6Z  D9  project/design/D09.md
R-DRFX-VNF2  D9  project/design/D09.md
R-DTVQ-N6WG  D9  project/design/D09.md
R-ELUQ-VJIQ  D9  project/design/D09.md
R-EN2N-9B9F  D16  project/design/D16.md
R-EPIG-0UQT  D16  project/design/D16.md
R-FR35-46U7  D7  project/design/D07.md
R-GSIG-PT07  D9  project/design/D09.md
R-H3PK-QFG3  D5  project/design/D05.md
R-H4XH-476S  D5  project/design/D05.md
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
R-OMKB-AY19  D9  project/design/D09.md
R-OUE3-L8VS  D9  project/design/D09.md
R-P3LQ-QY2X  D11  project/design/D11.md
R-P4TN-4PTM  D11  project/design/D11.md
R-P5U3-5CFZ  D6  project/design/D06.md
R-P61J-IHKB  D11  project/design/D11.md
R-P79F-W9B0  D11  project/design/D11.md
R-P89V-WVXD  D6  project/design/D06.md
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
R-PTEW-5KVY  D16  project/design/D16.md
R-PVUO-X4DC  D16  project/design/D16.md
R-PX2L-AW41  D16  project/design/D16.md
R-S6NB-RYUE  D16  project/design/D16.md
R-S7V8-5QL3  D16  project/design/D16.md
R-S934-JIBS  D16  project/design/D16.md
R-SKVI-TSZQ  D22  project/design/D22.md
R-SNBB-LCH4  D22  project/design/D22.md
R-SOJ7-Z47T  D22  project/design/D22.md
R-SX1B-XRK2  D4  project/design/D04.md
R-SZH4-PB1G  D4  project/design/D04.md
R-T06O-8SZX  D9  project/design/D09.md
R-T40A-VZQ7  D6  project/design/D06.md
R-T587-9RGW  D6  project/design/D06.md
R-T6G3-NJ7L  D6  project/design/D06.md
R-TQ77-6QLK  D9  project/design/D09.md
R-UJNS-PFLL  D9  project/design/D09.md
R-V1KQ-IKI6  D16  project/design/D16.md
R-V2SM-WC8V  D16  project/design/D16.md
R-VDY4-AP7H  D16  project/design/D16.md
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
R-X2NE-SE3E  D4  project/design/D04.md
R-X3VB-65U3  D4  project/design/D04.md
R-XR4M-U1ZT  D9  project/design/D09.md
R-XW08-D4YL  D3  project/design/D03.md
R-XZNX-IG6O  D10  project/design/D10.md
R-Y4JJ-1J5G  D10  project/design/D10.md
R-Y5RV-WB3T  D18  project/design/D18.md
R-Y6ZS-A2UI  D18  project/design/D18.md
R-Y810-TECF  D8  project/design/D08.md
R-Y878-6UDJ  D11  project/design/D11.md
R-Y87O-NUL7  D18  project/design/D18.md
R-Y98X-7634  D8  project/design/D08.md
R-Y9FL-1MBW  D18  project/design/D18.md
R-YAGT-KXTT  D8  project/design/D08.md
R-YANH-FE2L  D18  project/design/D18.md
R-YBOP-YPKI  D8  project/design/D08.md
R-YBVD-T5TA  D18  project/design/D18.md
R-YCWM-CHB7  D8  project/design/D08.md
R-YD3A-6XJZ  D18  project/design/D18.md
R-YFJ2-YH1D  D18  project/design/D18.md
R-YGQZ-C8S2  D19  project/design/D19.md
R-YHYV-Q0IR  D19  project/design/D19.md
R-YJ6S-3S9G  D19  project/design/D19.md
R-YKEO-HK05  D19  project/design/D19.md
R-YLMK-VBQU  D19  project/design/D19.md
R-YMUH-93HJ  D19  project/design/D19.md
R-YO2D-MV88  D19  project/design/D19.md
R-YPAA-0MYX  D20  project/design/D20.md
R-YQI6-EEPM  D20  project/design/D20.md
R-YRQ2-S6GB  D20  project/design/D20.md
R-YSXZ-5Y70  D20  project/design/D20.md
R-YU5V-JPXP  D20  project/design/D20.md
R-YVDR-XHOE  D20  project/design/D20.md
R-YWLO-B9F3  D20  project/design/D20.md
R-ZCMP-ARG8  D9  project/design/D09.md
R-ZELD-OQNG  D1  project/design/D01.md
R-ZWV0-CY54  D1  project/design/D01.md
R-ZZAT-4HMI  D1  project/design/D01.md
