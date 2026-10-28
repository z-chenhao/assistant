# 后台任务与用户进度：测试方案

[功能与验收目标](../features/tasks.md) · [技术方案](../designs/tasks.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-004](../features/tasks.md#f-004) | 验证状态转换、同 Task 单活动 Root、取消传播、旧 owner fencing 与结果回传。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-005](../features/tasks.md#f-005) | 真实长任务产生阶段摘要，UI 能读到更新；普通进度不触发 Main 推理或通知。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |

## 层级与关键路径

覆盖暂停/追加竞态、取消整棵树、once/ongoing、旧 epoch 结果拒绝和新 Root 接续。F-005 必须由真实生产路径产生摘要，不能在测试中直接调用 Progress 后当作闭环证据。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_tasks.py](../../tests/system/verify_tasks.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_task_control.py](../../tests/system/verify_task_control.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

- **F-005 剩余覆盖**：接通受信任的阶段进度生产者；保留独立于主动通知的只读投影。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="task-control-model-counterexample-and-implementation-evidence"></a>

### Counterexample and implementation evidence

`test_one_shot_completion_uses_root_outcome_after_child_drains` and
`TestTaskControlModelCompletionUsesRootOutcome` pair eight traces: both Root
outcomes, both Child outcomes, and both ending orders. For example,
`claim root0 → admit child owner1 → claim child → completed root0 → failed child`
must complete the Task and publish one completed Root update. The Go regression
uses the existing synthetic Child admission fixture, the real Agent Loop for
success, `Runs.Fail` for failure, and SQLite readback. Admission itself is not
proven by this fixture. A failed Root stays active even if its Child succeeds.
The unsafe-effect slot still belongs only to Root 0: an unknown effect in the
current epoch prevents completion; an old epoch's unknown does not prevent a
fresh Root from completing.

The unsafe model finds `claim root0 → commit pause → accept old-owner result`.
The Task epoch changes while the old owner still has a live lease. Checking only
ownership would accept that result. The normal model rejects it.
`TestTaskControlModelCounterexampleRejectsOldOwner` repeats this trace through
real model-result persistence, `Progress` and `ToolStore.Accept`, for pause and
cancel. It proves no stale progress, invocation or Child survives rejection.
The earlier `TestTaskControlPauseFencesLateWritesAndContinueNewRoot` also checks
late generation/usage rollback, reply, terminal and trace writes after stop.

For mutation evidence, use a disposable Go overlay removing only the Task
predicate from `fence` and run the new regression. It must fail with
`model counterexample admitted stale write`; rerun without the overlay and keep
both logs. Do not commit the overlay or a weakened production predicate.
Process-level evidence remains the maintained `task-control`, `children`,
`tools` and `code-recovery` gates; repetitions are explicitly selected through
the current runner rather than assumed from the historical schedule.

<a id="tasks-migration-and-verification"></a>

### Migration and verification

State schema v9 extends v8 with immutable completion policy, invocation stop
intent and legal stop/epoch transitions, without rewriting existing Runs,
snapshots or Conversation. Frozen capabilities retain their exact schemas.
Forward tests cover empty and every v1–v8 database, including v8 rollback;
existing identity/config/event checks remain in place. Future schemas fail fast.

`make test`, `make test-race`, `make lint`, and the system runner run the deterministic
checks. `tests/system/verify_tasks.py` owns isolated launch, doctor, CLI/PTY drive,
SQLite evidence and cleanup. It kills actual processes before/after Task,
result, Reporter and follow-up commits, then reclaims work and checks receipts,
model reuse, one completion per Root and delivery dedup. `evidence/tasks.json`
contains before/after state and terminal output. `evidence/local-provider.json`
also drives Task creation, independent Root execution and runtime-framed Main
follow-up through the actual Node/Pi Bridge and owned HTTP peer. `verify_task_control.py` adds live
PTY cancellation and three real SIGKILL boundaries around stop acceptance and
settlement. Repeat count and output directories are explicitly selected through the system runner. Go contracts cover transition rejection,
duplicate and concurrent append/pause/cancel, stale results/claims, synthetic
descendant propagation, retained results, one-shot/ongoing policy and shutdown.
Scripted fragments receive no semantic score. Live-model quality,
online accounts and full product Golden Case gates remain unexecuted.

Shutdown joins the currently owned work at a stable boundary. SIGKILL recovery
waits for the five-second lease to expire. Re-run `chat work` or restart the TUI;
never edit the database to revive terminal Runs. No background child is required
to survive the command or test harness.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
