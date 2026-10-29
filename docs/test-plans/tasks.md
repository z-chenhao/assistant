# 后台任务与用户进度：测试方案

[功能与验收目标](../features/tasks.md) · [技术方案](../designs/tasks.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-004](../features/tasks.md#f-004) | 验证状态转换、同 Task 单活动 Root、取消传播、旧 owner fencing 与结果回传。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-005](../features/tasks.md#f-005) | 真实长任务产生阶段摘要，UI 能读到更新；普通进度不触发 Main 推理或通知。 | 通过实际 Root 循环、真实浏览器和 checkpoint 前后杀进程证明闭环；模型质量单独评测 |

## 层级与关键路径

覆盖暂停/追加竞态、取消整棵树、once/ongoing、旧 epoch 结果拒绝和新 Root 接续。F-005 必须由真实生产路径产生摘要，不能在测试中直接调用 Progress 后当作闭环证据。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 执行记录 |
| --- | --- | --- |
| [tests/system/verify_tasks.py](../../tests/system/verify_tasks.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本轮及后续结果存于对应 evidence |
| [tests/system/verify_task_control.py](../../tests/system/verify_task_control.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本轮及后续结果存于对应 evidence |

### F-005 验收路径

- `internal/agent/sqlite/progress_test.go`：通过实际 Root 工具轮产生多阶段摘要，重复文本幂等，最终结果/Main/Child/旧冻结上下文不泄漏；空白、UTF-8 字节上限与截断响应保留上一条。注入 Task 写入失败验证 checkpoint 回滚，换 owner 恢复后复用已保存 generation，旧 claim 拒绝写入。
- 现有 Child 审批和费用授权组件检查验证 Activity 不把等待授权错误显示为工作完成；既有 Task control 检查继续覆盖 pause/cancel 的 epoch fencing。
- `verify_tasks.py`：实际 CLI/Actor 在进度事务提交前后 SIGKILL，恢复后保留摘要，完成的模型调用不重发；只生成原有最终 Root update 和 Main follow-up。现有 PTY `/status` 仍从同一 Activity 投影读取。
- `verify_channels.py` 的真实 Chromium live 场景：Root 先写一条摘要并等待冻结审批，进度页可见；拒绝后进入下一阶段并结束 ongoing 的本轮，摘要和阶段自动更新；随后取消，页面不增加任务控制按钮，Conversation 草稿及附件保留。
- 上述使用显式脚本模型检查软件协议。真实模型是否在适当阶段稳定生成有用摘要属于外部 Agent eval，本轮不声称通过该质量验收。

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

## Tasks 工作台回归

`internal/agent/sqlite/workspace_test.go` 用真实 Main/Root/Child 与 SQLite 证明详情、原始用户消息定位和非空 Main 交付，读取前后领域计数不变；65 项任务在同 created_at 下完整分页，游标不得跨搜索/视图使用，其他 Agent 的 Task 不可读。`internal/task/sqlite/reader_test.go` 验证只读池不新建或改写数据库。既有 Activity 测试继续保护终端/API 摘要契约。

既有 channels 浏览器测试打开任务详情，核对目标、审批冻结 hash、带任务上下文回 Main、移动端，以及生产 Pi peer 场景完成后的 Main 交付；不以原始 Worker result 代替用户回复。列表筛选不是新的 Task 状态；所有控制仍经 Main 和既有生命周期回归。
