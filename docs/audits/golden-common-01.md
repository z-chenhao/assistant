# Frozen Golden common-01 audit (ERII-244)

[golden-common-01.json](golden-common-01.json) retains **80 exact source blocks**
from Golden Cases §1 original/current L10 through §6 L137. The last block is the
**GC-04 overview row**. GC-05 and subsequent overview/detail blocks belong to
other packages. No content identity or line moved; no ADR reseal is needed.

The ledger contains **359 authored sub-obligations**: 252 required-v1, 50 policy
and 57 background explanations. None is explicitly deferred. All **302 mandatory
duties remain routed**, with 397 route entries because five common deterministic/
variant duties retain all 20 existing Case owners. There are 39 partial mechanical
rows, nine evidence-gap rows, 14 policy rows and 18 background rows. A completed
disposition certifies neither implementation, Golden scores nor release.

## Identity and preserved history

| Input | Identity |
| --- | --- |
| Original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Package digest | `3421f37dee8becf70cb964d7eb7a998f3a4bb19c006d0e2bcb3bbbb7cabd9d9e` |
| Audited code base | `146329d3db9d6a640443d530afb680d21f755e18` |
| Golden source SHA256 | `7f420ac86bd296d935ebe0003e2a04d2f0cf4315999aaf31ebac31e929b86f9f` |

The shared validator pins the package digest independently of the dispositions.
Each row retains full text, source/section/line/hash, current location, duty scope,
observations, proof limits, current reference bytes, actual merged PR provenance
and per-duty finite routes. There are 37 current references and 12 merged PRs.
Changed or ambiguous source identities reject; a moved block can only resolve
by its unique unchanged section/content hash. Strong Git verification now reads
the frozen package's own source file, including Golden Cases.

PR32 is **actually merged**, at `8a0d3ee67a9bab1633f92d3d8184ccb7a0d8080b`.
PR46 delivered the current unscripted entry. Their original GitHub metadata was
read for this audit; other merge metadata reuses the maintained prior package.
This does not rewrite the historical ledger's checkpoint descriptions. Its 122
partial/gap reviews and earlier package ledgers remain unchanged. A merge proves
provenance, not acceptance of every clause or an eligible model result.

## Maintained verification

```sh
make audit-golden-common-01
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load frozen identities, current references and complete routes.
Drive uses the existing `release_audit.py` CLI. Evidence is
`evidence/golden-common-01.json`; inspect `package_audit.reviews`, scope,
remaining routes and native receipts. Cleanup removes only disposable reports.
Regressions reject omissions, duplicates, added blocks, identity/location drift,
stale reference/native bytes, unsafe paths, unmerged PRs, unsupported exemptions,
missing mandatory routes and fabricated PASS. Package tests also preserve all
eight rubric rows, eight hard-failure categories, protected Case/SC repetitions,
all 20 finite Case owners and the legitimate independent-source Forget exception.

Same-host read-only strong verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/golden-common-01.json \
  --work-packages /absolute/durable/work-packages.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/golden-common-01-strong.json
```

This checks the original manifest's 1585 unique assignments, all 80 current
identities, retained native sizes/hashes and audited Git blobs/merge ancestry.
Without these optional inputs their checked flags stay false. With
`--require-complete`, the CLI still exits **2**, `status=inconclusive`,
`release_ready=false`, zero verified requirements. Byte integrity and count
checks cannot establish the semantic completeness of authored duties;
independent acceptance review must judge that separately.

## Native observations and limits

Complete raw evidence remains under the registered durable root:
`/Users/zhuchenhao/Projects/erii/evidence/agent-delivery/ERII-244/146329d3db9d6a640443d530afb680d21f755e18/audit-20261025/`.
Receipts bind commands, timestamp, source SHA, exits, file sizes/hashes and the
binary identity. Go 1.26.2 and Node 24.21.0 were used. The maintained locked setup
installed only the required Pi Bridge package from the offline npm cache.

| Observation | Result and proof limit |
| --- | --- |
| Scoring contracts | 10 tests passed. Finite 0–4 grades, exact 80 threshold, N/A redistribution, hard-failure priority, no Fake grades, six distinct variants and locked inputs. |
| Release consumer contracts | 20 tests passed. Full synthetic positive/incomplete/rejected composition, exact source/model/trial/Judge bindings, 20×6×5 in both arms, protected rounds, null/metrics, current L2, native CI and independent human acceptance. Synthetic satisfiability is mechanism evidence only. |
| Fixture contracts | Six tests passed. Seed1701, exact 100-row gold and independent input, locked six variants, SC missing/skip/failure rejection and five-run summaries that never invent scores. No product kill/restart was performed here. |
| Judge packet contracts | Three tests passed. Allowed packet fields, exclusion of raw state/Gold, incomplete/drifted evidence rejection and scripted ineligibility. No Judge model was invoked. |
| Unscripted input contracts | Five tests passed. All 20 support dispositions, 72 supported variants, native Artifact hash/size/path and model/configuration/Tool drift rejection. |
| Production CLI / Pi Bridge | Ten check groups passed against a maintained local HTTP peer: explicit setup, native Tool/usage and restricted Judge packet, strict fragment rejection, configuration/plan/model drift, missing adapter, authentication/partial/stalled requests and cleanup. The summary and all 32 raw JSON receipts are hash-bound. |

The production CLI run exercised the real boundary with an explicitly selected
loopback profile. It used no owner credentials, account, real model or Guest.
Owned processes/listener were joined and disposable roots removed; raw receipts
remain local. No historical ZIP was downloaded or uploaded.

Current producer coverage is **12 Cases × six input variants**, all fragments;
eight Cases lack adapters. Every current execution stays ineligible and ungraded.
Fixture time is explicit context; Host leases/telemetry use real time. Provider
RNG seed is unsupported. Whether these existing controls cover the normative
clock/seed duties remains unresolved; this audit neither requires freezing all
Host time nor assumes deterministic model output. ERII-366 owns the bounded
preparation/contract reconciliation and any necessary implementation. Complete
Outbox/external state, actual role/Memory/Coding journeys,
five protected native rounds, all L0/L1, full race/lint/e2e, real model quality,
current L2 and named human review were **not** executed or certified here.
The 90-point line remains an excellence benchmark; release thresholds stay
80 per Case, mean 85, no failed mandatory assertion or hard failure.

## Finite remaining routes

All 78 existing release children were read before adding the one finite B1
preparation proposal, ERII-366. None owns the shared clock/seed preparation
contract: ERII-330 executes evaluations and ERII-332 joins reports; neither
implements the missing preparation. The ledger now routes those two duties to
ERII-366, with exact source hashes, its native contract hash, a bounded
implementation responsibility and independent exit. ERII-330/332 remain downstream
consumers. ERII-366 is unassigned Backlog, awaiting coordinator registration and
admission; its creation does not establish that the conditions are met.

The maintained package regression reproduces B1 on the previous ledger and fixes
these two duties to that preparation owner, separately from the consumers. The
80 frozen identities, 359 duties, 397 route entries, existing observations and
release thresholds are preserved. The L132 explanation now correctly ends at
the four GC01–04 overview rows; GC05 starts the next package.

Existing Case owners receive common deterministic/variant duties within their original
frozen scope; the four overview rows retain their specific domain duties.
There are 25 finite receiving tasks. The audit repairs no product behavior.

| Existing route | Bounded remaining scope |
| --- | --- |
| [ERII-247](https://multica.ai/erii-studio/issues/01a1693a-5fd4-7668-8d4e-f00ef195b132) | 本包公共确定性/变体义务及适用总览能力仍需GC01在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-248](https://multica.ai/erii-studio/issues/01a1693a-9350-72b5-a785-4e7ac8894434) | 本包公共确定性/变体义务及适用总览能力仍需GC02在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-249](https://multica.ai/erii-studio/issues/01a1693a-df22-792e-adf5-b82e777486ce) | 本包公共确定性/变体义务及适用总览能力仍需GC03在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-250](https://multica.ai/erii-studio/issues/01a1693a-fa93-789b-a6d4-aca9072ad95e) | 本包公共确定性/变体义务及适用总览能力仍需GC04在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-251](https://multica.ai/erii-studio/issues/01a1693b-131f-74fc-9d7c-09ebdf3b3add) | 本包公共确定性/变体义务及适用总览能力仍需GC05在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-252](https://multica.ai/erii-studio/issues/01a1693b-310e-74c2-a76a-aec77a81464a) | 本包公共确定性/变体义务及适用总览能力仍需GC06在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-253](https://multica.ai/erii-studio/issues/01a1693b-5c67-79f4-b7e7-503aab53c9fa) | 本包公共确定性/变体义务及适用总览能力仍需GC07在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-254](https://multica.ai/erii-studio/issues/01a1693b-7bcb-763c-bec3-66a6266bcb7d) | 本包公共确定性/变体义务及适用总览能力仍需GC08在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-255](https://multica.ai/erii-studio/issues/01a1693b-9023-7dd9-bdc5-f524bea6131d) | 本包公共确定性/变体义务及适用总览能力仍需GC09在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-256](https://multica.ai/erii-studio/issues/01a1693b-ad30-79e0-bd4f-5a82e873b962) | 本包公共确定性/变体义务及适用总览能力仍需GC10在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-257](https://multica.ai/erii-studio/issues/01a1693b-d1f2-72c3-95c5-e3ff6994db17) | 本包公共确定性/变体义务及适用总览能力仍需GC11在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-258](https://multica.ai/erii-studio/issues/01a1693b-fd12-70c1-ba8e-4befddcb8664) | 本包公共确定性/变体义务及适用总览能力仍需GC12在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-259](https://multica.ai/erii-studio/issues/01a1693c-158b-76b6-b878-f6d365909cb7) | 本包公共确定性/变体义务及适用总览能力仍需GC13在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-323](https://multica.ai/erii-studio/issues/01a16bd0-7c19-7394-a44e-2e43a2857380) | 本包公共确定性/变体义务及适用总览能力仍需GC14在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-324](https://multica.ai/erii-studio/issues/01a16bd0-9154-79c1-80f5-f10aa1d58d15) | 本包公共确定性/变体义务及适用总览能力仍需GC15在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-325](https://multica.ai/erii-studio/issues/01a16bd0-a5a8-7b07-9f3e-7e13e9dd5433) | 本包公共确定性/变体义务及适用总览能力仍需GC16在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-326](https://multica.ai/erii-studio/issues/01a16bd0-bbda-7866-affe-d69fe2dcbd4e) | 本包公共确定性/变体义务及适用总览能力仍需GC17在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-327](https://multica.ai/erii-studio/issues/01a16bd0-d496-78f4-97f2-804d1b060d81) | 本包公共确定性/变体义务及适用总览能力仍需GC18在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-328](https://multica.ai/erii-studio/issues/01a16bd0-e84f-7117-8c46-d40ad1b8904a) | 本包公共确定性/变体义务及适用总览能力仍需GC19在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-329](https://multica.ai/erii-studio/issues/01a16bd0-fd2e-7889-9580-841a082c340e) | 本包公共确定性/变体义务及适用总览能力仍需GC20在原冻结范围完成完整机械/无脚本适配。 |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | 本包模型目标完成、证据/推断/未知、长期连续性、低打扰、八维评分、六变体五轮、pass@1/pass^5与已准备比较条件仍需实际双臂和独立Judge。时钟/seed准备/合同核对由ERII-366承担，不由本执行任务修复。 |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | 本包L1真实macOS VM/application/screenshot与L2鲜活搜索、只读/测试账号、兼容性、延迟、体验、采样时间及具名人审无当前完整原生证明。 |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | 本包完整L0/L1、Run/Task/Tool/Approval/Outbox/Artifact/外部状态、已准备冻结条件、全部硬失败/保护序列和当前CI的最终逐义务join仍缺；维护测试存在不等于完整发布证据。时钟/seed准备/合同核对由ERII-366承担。 |
| [ERII-343](https://multica.ai/erii-studio/issues/01a16ef3-43c0-78cd-977f-840dce140018) | 本包Pause/Cancel迟到epoch要求保留CodeExecution取消原子execution_epoch及两阶段收敛的已知差异。 |
| [ERII-366](https://multica.ai/erii-studio/issues/01a17025-2ae4-7e46-a945-443bc4599f4a) | Golden §3 L66固定时钟、L68固定随机种子的准备合同核对与批准处置、必要最小实现及原生拒绝/readback；不得只转回执行/汇总任务。 |

The ledger retains each route's exact remaining scope, original independent exit
and deduplication rationale. Case Done, an evaluation run ending or this audit's
acceptance cannot individually certify v1 release. The strict consumer and final
owner acceptance remain mandatory.
