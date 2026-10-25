# GC-03 finance verification

`scripts/verify_gc03.py` drives the original GC-03 finance journey through the
existing CLI/TUI Main loop, Memory/Curator, read-only MCP, restricted QuickJS,
Artifact and Delivery boundaries. It uses explicit scripted capabilities and
terminal-input Fake profiles. Natural input is retained separately from driver
plans; replies from these Fakes do not prove understanding or financial advice.
`semantic_score=null`, `eligible_for_grading=false`, `release_ready=false`.
Real model, asset connector, advice quality and current L2 acceptance remain with
[ERII-223](https://multica.ai/erii-studio/issues/01a16936-335f-72a4-a7f4-618823a87151).

```sh
make build
python3 scripts/verify_gc03.py --binary bin/erii --evidence evidence/gc03.json
python3 scripts/verify_gc03.py --binary bin/erii --evidence evidence/gc03.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc03.json --evidence evidence/gc03-preview.json
python3 -m unittest discover -s scripts -p 'test_gc03.py'
```

The maintained `security` suite gate runs all six original locked `m6-v1`
wording/noise/order variants, each for five serial repetitions. `--variant-jobs`
bounds independent variants (1, 2 or 4); it never reduces rounds. Golden inputs,
scoring, repetition counts and protection rules are unchanged. The additional
lock binds the original manifest SHA256
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`
and exactly its 21 GC-03 blocks.

| Boundary | Observed proof and limit |
| --- | --- |
| Launch / Doctor | Disposable data/configuration, explicit probe/select of Fake profiles, deterministic embedding/MCP peers and real QuickJS. No online account or implicit provider. |
| Original journey | Context and all three original events reach Conversation and frozen model requests, in maintained variant order. Turn 2 passes through a real PTY TUI. Separate driver plans are visible. |
| Current data | Initial 240 USD / 200 option record is recalled through the Memory Tool. CLI correction to 250 USD / 220 supersedes that record through Curator. Calculation reads the current Memory Tool result; its script embeds neither corrected amount nor Memory ID. Natural-language extraction remains ungraded. |
| Sources and arithmetic | MCP Tool returns the exact original world. CLI-reopened `finance.json` binds its Memory reference, scenario clock, USD/CNY, strike, FX 6.73 dated 2026-09-30, household assumptions and formulas. Python Decimal independently recomputes 54,995.60 USD and 370,120.39 CNY. Cash 300,000 and index funds 220,000 remain distinct from low-liquidity options. |
| Scenarios / missing data | Annual spending 300,000 / 350,000 / 400,000 gives static liquid-asset coverage 520,000 divided by spending. Returns, inflation and income are explicit zero assumptions; no retirement date or return is promised. Tax, fees, insurance and retirement target remain unknown. Travel inclusion in annual spend is unknown; the driver never adds it twice or invents a policy. Window execution is not guaranteed. |
| Restart / replay | Real SIGKILL after committed generation and after Memory commit before acknowledgement; completed generation and one Memory receipt reused after restart. Duplicate original ingress and repeated drains preserve exact native state, Artifact and deliveries. |
| Forget / old owner | Supplemental probe freezes a dependent generation, SIGKILLs, forgets the current option lineage, then sends unrelated input. Native Recall/request source references prove dependency; revoked Run is cancelled with `memory_forgotten`. Fresh Main completes; future requests/Recall omit both references and deleted records retain audit evidence. This is a privacy fence, not a Task epoch test. |
| Security / lifecycle | Main-only short synchronous work; zero Task/Child/Automation and Approval/external-effect inventories. No trade, redemption, insurance purchase or sensitive-data submission. External unknown-outcome/approval-transfer/Task-cancel branches are inapplicable here; no invented financial action is used to exercise them. |
| Cleanup / identity | Source SHA and tracked source hashes before/after, binary SHA/build metadata, Python/Node/Go/platform identity, native requests/state/Memory/Outbox/Artifact bytes. All owned processes join, disposable roots disappear, embedding peers stop. |

`--validate` requires all 30 ordered native receipts, source/binary identity,
locks and cleanup. It recomputes every assertion instead of trusting stored flags.
Negative probes reject changed input, stale price/quantity, liquidity conflation,
fabricated tax/insurance/guaranteed returns, missing FX date, wrong scenario,
forged Tool identity, broken supersede, false restart, duplicate dispatch, external
action, corrupt/rehashed Artifact, revived old owner, deleted Recall and replay
drift. Independent exact-SHA review and a clean rerun remain necessary; receipts
are inspectable consistency evidence, not cryptographic execution attestation.

| Original frozen Golden source lines | Coverage / semantic boundary |
| --- | --- |
| 187, 189, 197 | Full original input journey and capabilities; income/insurance/retirement details absent from the frozen world stay explicitly unknown. Real-time quotes and advice quality require model/L2 evidence. |
| 191, 192, 193, 194, 195 | Current data, dated currencies/FX, separate asset classes and cash flow, static scenarios, provenance and no financial action. Explanations and suitability remain ungraded. |
| 554, 556, 572, 574, 575, 576 | Original clock/world and all three turns; six unchanged variants, no answer substitution. |
| 578, 580, 581, 582, 583, 584, 586 | Real supersede/current Recall; exact independently computed valuation; no tax certainty or liquid-stock claim; reopenable assumptions/formulas and no transaction. |

The public evidence file is a small `gc03_native_manifest`. Complete native JSON
is retained beside it under `.gc03-native/`, with relative path, byte count and
SHA256. Both validation and preview require those bytes and reject traversal,
symlink, hash and summary drift. The pinned CI uploader excludes hidden files;
cloud evidence contains only the manifest/logs/source-bound suite reports.
Builder retains full native bytes and the executable in the registered durable
local evidence root for same-host independent review. Missing native evidence
is inconclusive.

The existing `unscripted_cases.py freeze --case GC-03` entry sends the original
context/events through the production Pi Bridge with an operator-prepared profile.
It sends no driver plans, assertions, expected outputs or credentials. Its
asset/Memory journey adapter remains incomplete and its input fragments remain
ineligible for grading; see [unscripted operations](guides/unscripted-evals.md).
This mechanical delivery does not satisfy the parent release's live/model gaps.
