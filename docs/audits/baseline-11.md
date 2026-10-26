# Frozen baseline-11 audit (ERII-241)

[baseline-11.json](baseline-11.json) retains **80 exact source blocks** from
technical baseline §14.2 original L1881 through §16.3 original L1984. The approved
ADR006 seal locates them at L1891–1994. **79 content identities are unchanged**;
the Task row changes from original L1939 to L1949 by adding `completion_policy`.
The changed row has its own current field mapping and observations. No old audit
conclusion is inherited.

The ledger disposes of **487 sub-obligations**: 444 required-v1, 36 policy and
seven background duties. All **480 mandatory/policy duties remain routed**.
There are 62 partial mechanical rows, 11 policy rows and seven background rows.
The policy clauses explicitly exclude features from v1; they do not defer the
current limits, isolation or persistence duties mixed into those clauses.
Table headers, separators and list introductions retain explicit reasons.
No requirement is certified and no semantic score is assigned. The preserved
historical release ledger and previous audit packages remain unchanged.

## Source and evidence identity

| Input | Identity |
| --- | --- |
| Original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Original package digest | `415396f2aa95fb071574356fe6cef9f03f8b81e729e0f8c9966bf9395886b534` |
| ADR006 manifest | 385378 bytes; SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Sealed package digest | `46a87bc1728c849591bf6c45905e56fc3e83a849264dacdda83d92e3d8b04fca` |
| Audited code base | `5ff99134cbd9561b66419d111745bab358e21493` |
| Technical baseline SHA256 | `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

The Task mapping pins old SHA256
`c158433cfab75928e00c5392a666e7af064ad0362ab39e3f8ac32fe8153294f1` to new
`a719ccca98f823a521cd0bbd69518e1b8e0e959a42bea50bd72c7b00fe84fa86`.
Owner authorization is `01a16e1d-3e2c-78fb-ba1d-5621d178a9bd`; coordinator receipt
is `01a16e27-6501-716a-8a22-0487065ce998`, with admission clarification in
`01a16f94-304b-7548-840e-00c87116aad5`. Both package digests are pinned outside
the authored dispositions. Strong mode verifies the full 1588-block inventory,
unique assignment, current source hashes, original locations, native bytes,
audited Git blobs and merged PR ancestry.

The 19 minimum logical records each enumerate **every field**, including optional
fields, with a physical column or explicit unresolved expression in `field_mapping`.
Split tables and views are named. In particular, aliasing `tool_invocation_id`
to the execution identity is distinct from replacing `source_ref`, `input_ref`,
`output_ref` or `value_ref` with inline values/hashes. The latter remains a gap.

The 96 current references bind file bytes at the audited base. The 16 merged PRs
are 2/6/8/10/12/14/16/18/22/23/25/26/31/33/35/45. Metadata is reused from the
preceding retained audit; strong mode independently checks local merge ancestry
and blobs. An indexed test is navigation evidence unless an observation records
its execution. Merge provenance and green contracts do not certify a requirement.

## Maintained verification

```sh
make audit-baseline-11
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load frozen sources and current ledger offline. Drive invokes the
existing `release_audit.py` CLI. Evidence is `evidence/baseline-11.json`; inspect
`package_audit.reviews`, `field_mapping`, routes and observations. Cleanup removes
only disposable reports. Regressions reject missing/duplicate/extra/drifted blocks,
stale source/reference/native hashes, unsafe paths, unmerged provenance, missing
fields or optional-field duties, inherited conclusions and altered ADR006 mapping.
The existing failed-rerun check proves stale output is removed. `--require-complete`
still exits **2**, with `status=inconclusive`, `release_ready=false` and zero verified
requirements. Completing this audit does not mean completing release validation.

Same-host read-only strong verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-11.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/baseline-11-strong.json
```

Only this mode claims manifest/native-byte/full-Git checks. Shallow CI cannot
claim unavailable local evidence. Independent review must assess the authored
semantic decomposition; hash checks alone cannot do that.

## Native observations and limits

Raw logs, commands, source SHA, timestamps, exits, binary hash, sizes and SHA256
remain in the registered durable root under
`ERII-241/5ff99134cbd9561b66419d111745bab358e21493/audit-20261025/`.
Locked offline package setup reused native caches. No historical ZIP, real model
account, owner credential or Guest was accessed.

| Observation | Observed result and limit |
| --- | --- |
| Selected Go race contracts | 79 passing tests/subtests, zero skips across five packages. Covers fresh/prior/future migrations, schema equivalence and real kill/restart, rollback, Profile/CAS/snapshot/credential cleanup, Child atomic admission, Task completion, Code migration, Artifact and Conversation/Inbox boundaries. This is a selected inventory, not full race or a protected five-run series. |
| Invariant contracts | 14 passing tests/subtests, zero skips. Additional tests exercise queued/live-owner Forget successors, failed/cancelled retest, missing authentication, atomic activation and Code Store/event failure. Passing the existing degraded/fencing behavior exposes the exact normative differences; it does not approve them. |
| Production schema readback | Real `erii status` creates isolated state v26, Conversation v3 and Memory v2 databases; all use WAL. Read-only schema/table/index inspection captures field owners. Go DSN source supplies per-connection foreign keys/busy timeout; Python's independent read connection cannot prove those Go connection settings. |
| Artifact CLI | Maintained CLI/PTY harness checks bytes, hash/scope/forgery/traversal/tombstone/CAS, Task visibility, selection/publication, Tool result atomicity and real kill/restart/query recovery using an independent Fake effect ledger. It does not establish OCR, actual model semantics or the full multimodal case. |
| Task control CLI | Real PTY Main→Task control→generation cancellation→Root settlement, plus three actual SIGKILL windows. Continue appends into a new Root/epoch and old followup/terminal results do not replay. The model is deterministic and has no semantic score. |

CLI harnesses join owned PTYs/children/peers and remove disposable state. Schema
readback deletes its isolated databases and starts no service. No whole release
suite, full race inventory, current L2, five-run protected series or named human
review is claimed. Audit completeness is separate from product conformance.

## Finite gap routes

Four new unassigned Backlog proposals retain precise source hashes and independent
exit conditions. Deduplication read all 70 existing parent children; no other
Agent was dispatched. Existing semantic owners are reused only within their
frozen responsibilities. The audit PR changes no product state or protocol.

| Route | Remaining scope |
| --- | --- |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | 本包294项mandatory/policy remaining的当前release-head实现/迁移/CAS/race/restart/安全审批、保护五轮、原生CI与独立review最终join；已知实现差异另有有限路由。 Exit: 最终按本包remaining逐条join当前代码/契约/已合并PR/原生test-race与五轮/restart/CI及独立review；未观察义务保留unmet并另路由有限修复，不从本包完成推发布通过。 |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | 本包实际模型的Memory证据判断、稳定性/行为验证与召回质量、Main异常解释、进度意义，以及本地/ChatGPT Provider兼容。 Exit: 在原冻结双臂评测中，保留相关原文义务的实际Main/Root/Child/Tool/Artifact/Delivery状态与独立评分；缺Profile/预算/Judge保持inconclusive。 |
| [ERII-335](https://multica.ai/erii-studio/issues/01a16dd9-6f96-7e35-88a3-c5f794a8e958) | Bridge进程对领域DB/Artifact/Computer/Tool的原生权限拒绝、独立凭据所有权与实际Host-Push Secret envelope对应原文Secret RPC；不认证已隔离。 Exit: 仅对无秘密自建fixture经生产launcher记录允许/拒绝与敏感材料清理；保留失败，必要最小修复或批准ADR及独立review后才满足，不触碰用户数据。 |
| [ERII-345](https://multica.ai/erii-studio/issues/01a16ef3-4aea-7e6d-93a7-bf158e28d3ff) | §6.6.8列举的文件/目录和codemode到Agent的依赖方向与当前agent/sqlite组合不同。 Exit: 逐列项职责和owner映射、依赖检查；变更规范先owner ADR/旧新hash/coordinator重封存；不加空包装或第二管线。 |
| [ERII-351](https://multica.ai/erii-studio/issues/01a16f1a-5c83-794f-87df-07fefac2e3c3) | Provider四方法没有独立Pause；state/Provider能力职责的私有实现对应尚需核对，禁止默认为与idle destroy等价。 Exit: 逐项映射原文生命周期责任与实际状态owner/入口；差异经批准处置及必要最小修复/ADR重封存，维护契约/CAS/race/restart、exact-SHA CI和独立review。 |
| [ERII-357](https://multica.ai/erii-studio/issues/01a16f68-a44e-7b55-99e4-6ea2a800addc) | 当前只有本地startup/database spans，没有跨Go/Bridge/Sidecar的trace context链。 Exit: 有限实现或owner批准精确解释，保留Run/Tool关联、边界传递与脱敏、失败/取消/重启/并发证据，exact-SHA CI及独立review。 |
| [ERII-358](https://multica.ai/erii-studio/issues/01a16f97-78f9-73b8-b93b-448bacbf792e) | code_manifest.source/input_json 和 code_store.value_json 保存内联数据，缺 source_ref/input_ref/value_ref 的对应引用；不能从hash和数据仍存活推断引用语义已满足。tool_invocation_id=execution_id、code_call_state joins 等可追踪别名应逐字段保留，不加空字段冒充完整。 CodeCall.output_ref当前映射到result_ref哈希与inline result_json，没有独立输出引用。 Exit: 只核对这三个源块中的每个最小逻辑字段、当前表/视图/读写者、引用解析/完整性/作用域/保留语义；必要最小修复或owner ADR及coordinator旧新hash重封存。保持旧Run固定源码、Store replay、unknown不重试与单一Tool管线。一个有限PR，空/旧库/rollback、CAS/race、跨作用域拒绝、真实kill/restart、维护入口/feature map、exact-SHA CI及独立review。 |
| [ERII-359](https://multica.ai/erii-studio/issues/01a16f97-7bed-7ca3-82fc-3a585d2da78e) | migrations/020_main_successor.sql 的 one_main 限定 desired_state=running；Forget 后旧 Main 仍非终态而新 queued Main 可共存。Claim会等待旧owner收敛，但16.2字面要求每Agent只有一个非终态Main。不是双重执行的证明。 Exit: 准确记录合法/非法状态与旧Run收敛、新输入持久化的时序。解决此单条唯一性规范与生产隐私fencing/后继Main的差异；必要owner ADR、兼容与coordinator重封存，或有限修复。不可丢弃新输入、重用旧隐私上下文、提前终态或放松fence。维护Forget/queued/active successor、并发与旧owner、空/旧迁移/CAS/race/kill-restart及exact-SHA CI和独立review。 |
| [ERII-360](https://multica.ai/erii-studio/issues/01a16f97-7ec3-760d-a825-29978b811f82) | agent_config_select_ready只约束setup_status=ready的选择；InvalidateProbe及账号失效将Profile置unavailable/reauth_required并保留active_model_profile_id、setup_status=degraded。16.2字面要求该引用必须ready；5.x允许降级/重认证，不能静默选择解释。 Exit: 只处置Active Profile的ready约束与现有degraded/重认证/恢复合同，保留本行Snapshot不可变和查询索引的已映射证据。先取得具体owner解释/ADR及必要重封存或有限实现；不默选其他模型、不改已冻结Run、不清理owner凭据。维护Probe失败、logout/refresh、冷启动、并发CAS、迁移、原子事件和replay，exact-SHA CI及独立review。 |
| [ERII-361](https://multica.ai/erii-studio/issues/01a16f97-81c4-75b5-ac3a-f8d74cdabf14) | codeBridge.claim/release调用runEvent，但abandon只UPDATE code_execution_state至suspended后提交，无对应EventOutbox；Runs.write/Transaction不自动发事件。已持久化状态可被扫描恢复不等于满足任意迁移+Outbox的原文。 Exit: 只以该abandon路径为首个固定反例核对CodeExecution状态迁移与原子EventOutbox责任，建立状态/事件表，必要最小修复或owner ADR及旧新hash重封存。不把Broker发布纳入事务，不建第二调度器。维护event enqueue失败全回滚、重复/迟到/old epoch、断连与恢复、unknown不重试、CAS/race和真实kill-restart；一个有限PR、feature map、exact-SHA CI和独立review。 |
| [ERII-344](https://multica.ai/erii-studio/issues/01a16ef3-47a8-7ad5-a3ef-3fb5975a05a8) | 单CodeMode缺可配置累计HostCall默认10000/硬上限50000；128 pending限制使正常1000 Promise不能据100项通过推定。 Exit: 分别验证累计计数与有界透明排队；100/1000、重放不重计、旧epoch/拒绝/内存/race/restart；规范变化先owner ADR及重封存。 |

The Main uniqueness row records a narrower SQL predicate (`desired_state=running`)
than the unconditional normative constraint. Its current privacy successor
behavior has a regression test and fences execution; this audit does not infer
simultaneous execution. The Active Profile row likewise records the difference
between selection-time `ready` validation and retaining an active pointer while
`degraded`. These require explicit contract resolution, not silent compatibility.

The Outbox finding is specifically `codeBridge.abandon`: unlike `claim` and
`release`, it updates `code_execution_state` to suspended without `runEvent`.
Recoverability by scanning does not itself meet the unconditional atomic-event
clause. The finite task starts from that exact counterexample, without inventing
a second execution or scheduling pipeline.
