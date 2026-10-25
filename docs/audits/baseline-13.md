# Frozen baseline-13 audit (ERII-243)

[baseline-13.json](baseline-13.json) retains **58 exact source blocks** from
technical baseline §17.M5 original L2144 through §19 original L2234. The approved
ADR006 seal locates them at L2154–2244. **57 content identities are unchanged**;
§18.2 Task lifecycle changes original L2186 to L2196 by adding `complete`.
That row independently checks create/append/pause/cancel/complete, the four
Task states and failed Run behavior against PR45 and current tests. It inherits
no old conclusion.

The ledger records **352 sub-obligations**: 292 required-v1, 32 policy,
26 explicitly deferred choices and two table-structure explanations. All
**324 required/policy obligations remain routed**. There are 41 partial mechanical
rows, five evidence-gap rows, 10 policy rows and two background rows. A completed
disposition never certifies a requirement, milestone, semantic score or release.
The original 122-review historical ledger and earlier audit packages are unchanged.

## Source identity

| Input | Identity |
| --- | --- |
| Original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Original package digest | `933565336b762b2dec8e8dc2b9459fda3d41dc3c8515bdc8ccce9bced7729e01` |
| ADR006 manifest | 385378 bytes; SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Sealed package digest | `9d86d5732f7addecb9001ad5cf9c9b521f212373c5a57149e8c08cb95661570d` |
| Audited code base | `97b44301d6c99ceb188702c4e21cadc801470ba9` |
| Technical baseline SHA256 | `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

Task lifecycle maps old SHA256
`9602850bb1308669571ef7e86df0bf9bc91e3bee774426a3f263cb7faa21cfda` to
`1bbef426eb568fcf13f01f9caa4add3de302d98724f9f40fde377bb0cfe0824e`.
Owner authorization is `01a16e1d-3e2c-78fb-ba1d-5621d178a9bd` and coordinator
receipt is `01a16e27-6914-7c76-9607-efe9f00089c3`. Both package digests and this
mapping are pinned outside the authored ledger. Changed or ambiguous identities
reject; a moved line may only resolve by its unique section/content hash.

Each source row retains full text, original/current location, duty kinds,
observed behavior, proof limits, byte-bound current references, actual merged
PR provenance, executed observation references and per-duty remaining routes.
Merged PRs identify implementation provenance, not full acceptance of the clause.
Metadata from maintained prior audits is reused; PR13/44 metadata was read from
GitHub for this audit. Strong mode checks every merge ancestor and current blob.
An indexed maintenance test is navigation unless a native receipt records execution.

## Maintained verification

```sh
make audit-baseline-13
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load frozen source identities and current references offline.
Drive uses the existing `release_audit.py` CLI. Evidence is
`evidence/baseline-13.json`; inspect `package_audit.reviews`, kinds, remaining
routes and observations. Cleanup removes only disposable reports. Regressions
reject omissions, duplicates, extra blocks, drifted identities/locations,
stale reference/native hashes, unsafe paths, unmerged PRs, missing mandatory
routes, unexplained exemptions and altered/inherited ADR006 mapping.

Same-host read-only strong verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-13.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/baseline-13-strong.json
```

This mode checked the manifest's full 1588-block unique assignment, all package
identities, native bytes and Git ancestry/blobs. Without those optional inputs,
the corresponding flags remain false. `--require-complete` still exits **2**,
with `status=inconclusive`, `release_ready=false` and zero verified requirements.
Hash checks cannot judge whether the authored semantic decomposition is complete;
independent acceptance review must assess that separately.

## Native observations and limits

Complete logs and reports remain outside the worktree under the registered root:
`/Users/zhuchenhao/Projects/erii/evidence/agent-delivery/ERII-243/97b44301d6c99ceb188702c4e21cadc801470ba9/audit-20261025/`.
Receipts retain source SHA, commands, timestamps, exits, file sizes/hashes and
the locally built binary identity. Node 24.21.0 and Go 1.26.2 were used. Locked
QuickJS/Bridge packages were installed from the offline native cache. No historical
ZIP, owner credential, actual model account or Guest was accessed.

| Observation | Result and proof limit |
| --- | --- |
| Selected Go race contracts | 87 tests/subtests passed, zero skips. Real SQLite/NATS, seven event process-crash boundaries, encrypted backup/upgrade preflight, Task completion/CAS/fencing, Tool policy, frozen snapshots/Code replay and embedding/index/recall contracts. This is a selected inventory, not full Go race or a five-run series. |
| Actual QuickJS contracts | 19 passed, zero skips. Explicit output, bounded resources, Promise suspension, unawaited calls, cancellation, illegal globals/Host calls, stable keys and replay drift. This does not certify the complete Runtime event/Outcome contract or Bridge OS isolation. |
| Child CLI/PTY | Eight check groups and six actual SIGKILL windows. One Task/Root/Child tree, direct Parent results, scoped large-result Artifact, independent approval, context/capability reductions, recursive delegation and cleanup. The deterministic provider earns no semantic score. |
| 37/100 recovery CLI | Three variants: Sidecar SIGKILL, Go SIGKILL and graceful SIGTERM. At the cut there are 37 completed calls, eight accepted approval waiters and 55 unissued calls. Recovery retains invocation/Child IDs, completes 100, keeps completed provider-dispatch counts at one and releases VM/claims. Nested cancellation stays cancelled after restart. One harness run is not a protected five-run series. |

Owned PTYs/processes/peers were joined and disposable databases/effect ledgers
removed. Raw evidence is retained locally. Full race/lint/e2e, all fault windows,
20 eligible Golden Case scores, current dual-model comparisons, L2, named human
review and the final release pack were **not** executed or certified here.

## Deferred choices retain current duties

§19 delays final framework/SDK/visual choices, numeric budgets and retention
settings, script-management UI, regulated capability design and cloud/multi-Agent
product work. It does not delay the current TUI minimum, Compaction algorithm
and configuration, unknown-effect approval/never policy, Artifact references and
dry-run cleanup, conservative operations defaults, single-Agent modular product,
or the prohibitions on budget-query tools and unreviewed real trading.

The long operations block separately retains every default, TLS/credential,
upgrade, cold-backup, restore-disable/enable, source-retirement, external-result
confirmation and shutdown obligation. The security row separately lists all
three secret types against all six prohibited sinks. Final-choice exemptions
never remove those current obligations.

## Finite remaining routes

All 77 existing release children were read for semantic deduplication. Existing
owners below already have bounded inputs and independent exit conditions; no
duplicate task or product repair is added by this audit. The ledger retains each
route's exact remaining scope, exit condition and deduplication rationale.

| Existing route | Remaining bounded scope |
| --- | --- |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | 本包当前release-head逐义务实现/契约/故障/race/运维/原生CI/独立review最终join；本次选定观察不满足全套发布。 |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | 20 Case实际双臂/六变体/五轮、相同冻结条件、独立Judge评分和模型质量；缺少真实执行保持unmet。 |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | 当前SHA的真实Guest/VM、网页/登录/渠道/桌面兼容性、Artifact稳定性与具名人审；离线Fake不能代替。 |
| [ERII-335](https://multica.ai/erii-studio/issues/01a16dd9-6f96-7e35-88a3-c5f794a8e958) | Bridge没有原生Tool/Computer/领域DB权限拒绝证明；应用接口不提供这些能力不等于OS隔离。 |
| [ERII-344](https://multica.ai/erii-studio/issues/01a16ef3-47a8-7ad5-a3ef-3fb5975a05a8) | 1000项透明排队与累计Host Call默认/硬上限；pendingCalls=128耗尽会RESOURCE_LIMIT，100项通过不能外推。 |
| [ERII-346](https://multica.ai/erii-studio/issues/01a16ef3-4dfc-7c22-b841-2da0e323cd8b) | 现有功能及历史100项容量样本不构成当前模型/硬上限/成本完整校准；保持同环境原始指标与owner阈值输入。 |
| [ERII-352](https://multica.ai/erii-studio/issues/01a16f33-2755-76d6-a96f-5ac3e3e8f661) | 当前Compaction使用字节估计，需明确冻结模型token阈值映射及模型压测参数。 |
| [ERII-354](https://multica.ai/erii-studio/issues/01a16f63-9fb3-78d6-95f9-a5b3b51fdef2) | Memory当前自动降级/淘汰生命周期仍有独立有限缺口，不能由Curator存在认证全部Memory能力。 |
| [ERII-351](https://multica.ai/erii-studio/issues/01a16f1a-5c83-794f-87df-07fefac2e3c3) | ComputerProvider暂停/状态/能力职责仍需逐项合同处置，不能用idle destroy等同Pause。 |
| [ERII-347](https://multica.ai/erii-studio/issues/01a16f0e-9047-71b1-8d54-b5d5ff158cdd) | 真实Guest只读readiness和预置权限边界，不能由Fake准备状态认证。 |
| [ERII-348](https://multica.ai/erii-studio/issues/01a16f0e-934e-72d5-bb40-ac815bf49005) | 真实Guest浏览器fixture维护部署/Doctor/清理，不能把有Capability入口当生产已就绪。 |
| [ERII-363](https://multica.ai/erii-studio/issues/01a16fb4-2ec5-7050-a0f7-c4b2c4d46245) | 现有YAML/DB/固定策略与原文配置点对应需逐项批准处置；不得将硬编码当可配置。 |
| [ERII-361](https://multica.ai/erii-studio/issues/01a16f97-81c4-75b5-ac3a-f8d74cdabf14) | CodeExecution abandon状态更新没有对应Outbox；最终原子状态迁移门禁保留具体反例。 |
| [ERII-338](https://multica.ai/erii-studio/issues/01a16dfe-85d9-757b-9e52-2d0de021e380) | 现有Runtime事件生命周期/Outcome载荷与基线差异仍需有限合同处置。 |
| [ERII-359](https://multica.ai/erii-studio/issues/01a16f97-7bed-7ca3-82fc-3a585d2da78e) | Forget撤销中旧非终态Main与queued successor可共存，执行fencing通过不满足原文字面唯一性。 |
| [ERII-360](https://multica.ai/erii-studio/issues/01a16f97-7ec3-760d-a825-29978b811f82) | Probe失效后Active引用可保留degraded Profile，ready引用不变量需独立处置。 |
| [ERII-343](https://multica.ai/erii-studio/issues/01a16ef3-43c0-78cd-977f-840dce140018) | CodeExecution取消的execution_epoch原子失效与两阶段收敛差异；Task epoch测试不能代替。 |
| [ERII-356](https://multica.ai/erii-studio/issues/01a16f63-a532-743c-a2cf-034c08160575) | 模块落位与Credential消费者合同，保留已知私有组合边界差异；接口存在不等于规范职责已对齐。 |
| [ERII-364](https://multica.ai/erii-studio/issues/01a16fb4-318c-746b-afef-7dc620a43eff) | 消费者最小接口逐方法核对；原文控制面/持久化/组合职责不能由部分同名方法推断完成。 |

Final evidence must still bind the current release head, strict independent
reviews, native CI and actual eligible observations. Mechanism tests for the
release validator cannot supply product scores or human acceptance. ERII-243
completion means this frozen audit is disposed of; v1 acceptance remains open.
