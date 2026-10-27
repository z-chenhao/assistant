# Frozen Golden common-02 audit (ERII-245)

[golden-common-02.json](golden-common-02.json) retains **80 exact source blocks**
from Golden Cases §6 L138 (GC-05 overview) through §15.产品材料 L1202 (Manus).
This package includes the remaining 16 overview rows, shared data/recording
contracts, variants, five semantic rubric rows, SC-01–13, suggested input/output
schemas, reporting metrics and M1–M7 integration. Adjacent Case datasets and
later reference links belong to other packages.

There are **476 authored duties**: 415 required-v1, 52 policy and nine background
explanations. All **467 mandatory/policy duties** retain finite routes. The 2,549
route entries preserve each affected Case's existing ownership for common duties;
these are not 2,549 new requirements. There are 41 partial mechanical rows, 26
evidence gaps, five policy rows and eight background rows. Nothing is deferred.
No requirement is certified as implemented, scored or release-ready.

## Identity and history

| Input | Identity |
| --- | --- |
| Original owner manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Canonical package digest | `8915d2e0d8b4fd82564a0f2ce88988f645d7c24ded65f280b7edaa91f73802cc` |
| Audited code base | `96411e21e541566d1197d503e8f85ef6144175f6` |
| Golden source SHA256 | `7f420ac86bd296d935ebe0003e2a04d2f0cf4315999aaf31ebac31e929b86f9f` |

The shared validator pins the package digest separately from authored reviews.
All 80 section/content hashes and original/current locations match unchanged.
No reseal or inference about changed normative text is needed. The original
manifest was read from the same-host durable cache and checked for all 1,585
unique assignments. Current source inventory has 1,588 blocks after earlier
approved changes outside this package.

Each review preserves full text, identity, all duties and scope, current
implementation/test references, merged PR provenance, observations, proof limits
and per-duty remaining owners. There are 73 current reference files and nine
actual merged PR records. PR metadata reuses the hash-checked prior Golden audit;
strong verification checks merge ancestry and every referenced blob locally.
PR32 is merged provenance, not a product acceptance certificate. The historical
ledger's 122 partial/gap reviews and every earlier package ledger remain intact.

The shared fixture maps all 13 leaf fields, including real frozen model identity
and the unresolved scenario clock boundary. The suggested input and result
protocols map all 12 and 21 leaf fields respectively. Suggested YAML layout and
sample values do not force a second file format or literal model ID. The actual
snapshot and every mandatory output still need evidence. Ungraded results keep
`final_score=null`; illustrative zero/false values never become observations.

## Maintained verification

```sh
make audit-golden-common-02
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load frozen identities, current reference bytes, dispositions and
finite routes. Drive uses the existing `release_audit.py` CLI. Evidence is
`evidence/golden-common-02.json`; inspect `package_audit.reviews`, observations,
remaining duties and gap exits. Cleanup removes only disposable reports. The
regressions reject omissions, duplicates, altered identities/locations, stale
references/native bytes, unmerged provenance, unsafe paths, missing mandatory
routes, unsupported exemptions and fabricated PASS. Package checks additionally
preserve all three field maps, SC-01–13, the two separate SC-13 five-round duties,
all reporting submetrics and concrete implementation owners.

Same-host strong read-only verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/golden-common-02.json \
  --work-packages /absolute/durable/work-packages.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/golden-common-02-strong.json
```

This checks the original manifest, all 80 current identities, native sizes/hashes,
audited Git blobs and merge ancestry. The three checked flags remain false when
the optional inputs are unavailable. `--require-complete` still exits **2**,
`status=inconclusive`, `release_ready=false`, zero verified requirements.
Mechanical integrity does not establish semantic completeness; independent
acceptance review must inspect the authored duties and routes.

## Native observations and limits

Full evidence stays under the registered durable root:
`/Users/zhuchenhao/Projects/erii/evidence/agent-delivery/ERII-245/96411e21e541566d1197d503e8f85ef6144175f6/audit-20261025/`.
Commands, timestamps, exits, source SHA, sizes/hashes and binary identity are
bound in the ledger. Go 1.26.2 and Node 24.21.0 were used. Pi Bridge and QuickJS
dependencies were installed through the maintained setup entry in offline mode.

| Current observation | Result and limit |
| --- | --- |
| Scoring / strict release consumer | 10 / 20 tests passed. Exact thresholds, null preservation, mechanical priority, synthetic positive composition, native source/model/trial/Judge/CI/human binding rejection. Synthetic packs prove mechanism behavior only. |
| Fixtures / Judge packets / unscripted contracts | Six / three / five tests passed. Locked inputs, six variants, corruption/skip rejection, allowed packet fields and ineligible scripted fragments. No actual semantic Judge call. |
| Maintained SC conformance | One race run passed all 31 mapped test roots across real SQLite stores/executors. SC-10's mapping only names scheduler tests; the direct Follow-up/Steering contract was additionally executed below. |
| Spend and Follow-up | One race run passed 14 `TestSpend*` roots and `TestFollowupBatchWaitsForMainAndUserSteeringWins`. Covers monetary conversions, final/failed-response hold and recovery, denial, frozen prices, epoch fencing and merged follow-ups. This is one round, not five. |
| Production CLI / Pi Bridge | Ten check groups passed through actual CLI/HTTP peer: explicit model setup, native Tool/usage, restricted Judge packet, fragment rejection, plan/config/model drift, missing adapter, authentication/partial/stalled failures and cleanup. Summary and 32 raw receipts are retained. |

The actual CLI used an explicitly selected local HTTP peer, no owner credentials
or real account/model. It still exposes 12 Cases × six **input fragments** and
eight missing adapters; every execution is ineligible and ungraded. Owned
processes/listeners were joined and temporary roots removed. No historical ZIP
was transferred. Raw CLI and race evidence remains local; cloud delivery uses a
small report/manifest only.

Conversation noise at roughly 15% does not prove mail/file noise or retrieval
restraint. Reversing world keys does not prove noncritical event reordering.
Each Case owner must preserve causal/Gold identity and implement its appropriate
adversarial and surface variants. Frozen fixture context/seed does not settle
all clock/seed semantics; ERII-366 owns preparation. Model quality, full Case
journeys, all L0/L1, five protected native rounds, current Guest/L2, named human
review and full local test/lint/e2e were not executed or certified here.

## Finite remaining routes

All 79 existing parent children and their complete descriptions were read before
creating two unassigned Backlog proposals. ERII-367 owns missing metric collection,
per-submetric validation and reporting, including applicability/denominators and
unknowns. Current release receipts only check nonempty dictionaries for Durable,
Memory, Coding and resources; field presence is insufficient. ERII-346's Code Mode
performance baseline can supply observations but does not own these reporting
contracts. ERII-330 executes evaluations and ERII-332 joins final evidence;
neither implements the missing common metric mechanism.

ERII-368 owns explicit SC-13 native gate consumption. `verify_suite.py` already
schedules five spend harness rounds, but `release_decision.SYSTEMS` only accepts
SC-01–12. The new finite task must bind **both** five consecutive `verify_spend.py`
and five `TestSpend*` rounds with every monetary hard assertion. Generic safety
receipts or green CI cannot substitute. Neither new proposal starts an Agent or
establishes that its requirements are satisfied; coordinator registration and
normal admission remain necessary.

Known CodeExecution cancellation and Memory conflict/supersession duties retain
ERII-343/353. The audit does not repair product behavior or waive those conflicts.
Each route below has an independent exit in the ledger, with exact remaining
scope; common duties retain all 20 existing Case owners.

| Route | Remaining bounded responsibility |
| --- | --- |
| [ERII-247](https://multica.ai/erii-studio/issues/01a1693a-5fd4-7668-8d4e-f00ef195b132) | GC01完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-248](https://multica.ai/erii-studio/issues/01a1693a-9350-72b5-a785-4e7ac8894434) | GC02完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-249](https://multica.ai/erii-studio/issues/01a1693a-df22-792e-adf5-b82e777486ce) | GC03完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-250](https://multica.ai/erii-studio/issues/01a1693a-fa93-789b-a6d4-aca9072ad95e) | GC04完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-251](https://multica.ai/erii-studio/issues/01a1693b-131f-74fc-9d7c-09ebdf3b3add) | GC05完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-252](https://multica.ai/erii-studio/issues/01a1693b-310e-74c2-a76a-aec77a81464a) | GC06完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-253](https://multica.ai/erii-studio/issues/01a1693b-5c67-79f4-b7e7-503aab53c9fa) | GC07完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-254](https://multica.ai/erii-studio/issues/01a1693b-7bcb-763c-bec3-66a6266bcb7d) | GC08完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-255](https://multica.ai/erii-studio/issues/01a1693b-9023-7dd9-bdc5-f524bea6131d) | GC09完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-256](https://multica.ai/erii-studio/issues/01a1693b-ad30-79e0-bd4f-5a82e873b962) | GC10完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-257](https://multica.ai/erii-studio/issues/01a1693b-d1f2-72c3-95c5-e3ff6994db17) | GC11完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-258](https://multica.ai/erii-studio/issues/01a1693b-fd12-70c1-ba8e-4befddcb8664) | GC12完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-259](https://multica.ai/erii-studio/issues/01a1693c-158b-76b6-b878-f6d365909cb7) | GC13完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-323](https://multica.ai/erii-studio/issues/01a16bd0-7c19-7394-a44e-2e43a2857380) | GC14完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-324](https://multica.ai/erii-studio/issues/01a16bd0-9154-79c1-80f5-f10aa1d58d15) | GC15完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-325](https://multica.ai/erii-studio/issues/01a16bd0-a5a8-7b07-9f3e-7e13e9dd5433) | GC16完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-326](https://multica.ai/erii-studio/issues/01a16bd0-bbda-7866-affe-d69fe2dcbd4e) | GC17完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-327](https://multica.ai/erii-studio/issues/01a16bd0-d496-78f4-97f2-804d1b060d81) | GC18完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-328](https://multica.ai/erii-studio/issues/01a16bd0-e84f-7117-8c46-d40ad1b8904a) | GC19完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-329](https://multica.ai/erii-studio/issues/01a16bd0-fd2e-7889-9580-841a082c340e) | GC20完整机械/无脚本领域适配；以原冻结§8合同为准。 |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | 本包模型目标完成、证据/推断/未知、长期连续性、低打扰、八维评分、六变体五轮、pass@1/pass^5与已准备比较条件仍需实际双臂和独立Judge。时钟/seed准备/合同核对由ERII-366承担，不由本执行任务修复。 |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | 本包L1真实macOS VM/application/screenshot与L2鲜活搜索、只读/测试账号、兼容性、延迟、体验、采样时间及具名人审无当前完整原生证明。 |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | 本包完整L0/L1、Run/Task/Tool/Approval/Outbox/Artifact/外部状态、已准备冻结条件、全部硬失败/保护序列和当前CI的最终逐义务join仍缺；维护测试存在不等于完整发布证据。时钟/seed准备/合同核对由ERII-366承担。 |
| [ERII-343](https://multica.ai/erii-studio/issues/01a16ef3-43c0-78cd-977f-840dce140018) | CodeExecution取消的execution_epoch原子失效与两阶段收敛差异；Task epoch测试不能代替。 |
| [ERII-353](https://multica.ai/erii-studio/issues/01a16f33-2af9-73ad-af32-b0c29774632d) | 不同文本的自动事实缺冲突关联；双城市CLI已复现，不能用指纹去重或显式supersedes冒充完整后台语义处置。 |
| [ERII-366](https://multica.ai/erii-studio/issues/01a17025-2ae4-7e46-a945-443bc4599f4a) | 仅承接Golden §3 L66固定时钟与L68固定随机种子的准备/实现合同：准确区分fixture时钟/seed、Host实时计时与Provider随机控制，完成批准处置、必要最小准备机制及维护原生证据；不默选新规范含义。 |
| [ERII-367](https://multica.ai/erii-studio/issues/01a17044-e7e4-71ae-b4b1-c1e52cb22035) | §13交互/效率/Durable/Memory/Coding各子指标的原生采集、适用性、逐字段校验及可重算报告，不能仅检查非空dict。 |
| [ERII-368](https://multica.ai/erii-studio/issues/01a17044-eac3-7820-9353-52c408cdabdb) | §11 SC-13两类五轮原生收据的专用消费者；现有发布SYSTEMS只接受01..12。 |

The strict release consumer, current eligible observations and final owner
acceptance remain required. This audit being Done cannot certify v1 release.
