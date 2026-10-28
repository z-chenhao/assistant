# 后台任务与用户进度

持久保存工作目标，支持追加、暂停、继续、取消和阶段进度。

[需求主索引](../feature-map.md) · [技术方案](../designs/tasks.md) · [测试方案](../test-plans/tasks.md)

## 功能说明

用户能把耗时工作交给后台，之后通过对话追加、暂停、继续、取消或结束。

用户主动查看时能了解当前阶段、摘要与子任务完成情况。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-004"></a>

### F-004 · Task 生命周期与继续工作

**需求**：用户能把耗时工作交给后台，之后通过对话追加、暂停、继续、取消或结束。

**当前实现**：Task create/append/pause/cancel/complete、once/ongoing 和 Root 恢复已接通。

**边界与剩余工作**：Run 失败不新增 failed Task 状态；继续通常建立新 Root，不复活旧 Run。

**验收目标**：验证状态转换、同 Task 单活动 Root、取消传播、旧 owner fencing 与结果回传。

<a id="f-005"></a>

### F-005 · 执行中的有意义进度

**需求**：用户主动查看时能了解当前阶段、摘要与子任务完成情况。

**当前实现**：有 Progress 存储方法、Activity 聚合及进度页；静态调用搜索未发现生产路径写入 Progress()。

**边界与剩余工作**：接通受信任的阶段进度生产者；保留独立于主动通知的只读投影。

**验收目标**：真实长任务产生阶段摘要，UI 能读到更新；普通进度不触发 Main 推理或通知。

## 如何使用

在 Conversation 交代工作，随后说“补充这个要求”“暂停这件事”“继续之前的工作”或“取消”。Main 使用 Task 工具查找和控制工作。

Web 的 Ongoing work 和终端 `/status` 可查看总体状态与 Child 聚合。当前不应期待持续更新的阶段摘要；该链路仍由 F-005 跟踪。

## 详细操作参考

- [任务操作详情](#tasks-task-delegation-and-root-recovery)
- [任务停止与准入模型](../designs/tasks.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="tasks-task-delegation-and-root-recovery"></a>

## Task delegation and Root recovery

The M2 slices add Main-only Task discovery, create/append/pause/cancel/complete,
independent Root execution and durable Main follow-ups. Use a selected, probed
model through the existing TUI or `chat send` / `chat work` commands. There is
one shared Agent Loop and one ToolInvocation pipeline.

A self-contained delegation brief belongs in `task_manage(create).task`. Main
must include necessary background, constraints, completion criteria and expected
outputs. For continuing work, Main can explicitly create with
`completionPolicy: "ongoing"`; omission or `"once"` creates a one-shot Task.
The policy is fixed at creation. For example:

```json
{"operation":"create","task":"Keep the wedding checklist current as guest counts and dates change; preserve confirmed facts and return the next dependencies to resolve.","completionPolicy":"ongoing"}
```

The Host generates an opaque TaskRef and a title; the receipt means
accepted, not finished. Short synchronous questions remain in Main.

| Canonical tool | Input | Output | Visibility / permission | Recovery |
| --- | --- | --- | --- | --- |
| `builtin__task__search` v1 | Optional `query`, `scope`, `status[]`, `limit` | `TaskSummary[]` | Main, `read_only` | `safe`; bounded 64 KiB result |
| `builtin__task__manage` v1 | Discriminated `create/append/pause/cancel/complete` union | `{taskRef,status,accepted:true}` | Main, `task_control` | `safe`; transactional receipt dedup |

The exact schemas and strict validators live in `internal/task/task.go`.
Unknown fields, nulls, duplicate fields, wrong union arms, invalid limits and
unknown statuses return `INVALID_ARGUMENT`. Missing or inaccessible Task or
Artifact references return `NOT_FOUND`. Responses expose no Run/invocation IDs,
credentials, SQL, budgets or internal traces. Infrastructure write failures stop
at a recoverable boundary instead of becoming a misleading Tool success.

[Artifact validation](artifacts-media.md), [external effects and approvals](tools-approvals.md),
and [delivery](../designs/channels.md) use the shared Tool pipeline. Workers can use
[Direct Child delegation](../designs/delegation.md); [Code Mode](../designs/codemode.md) supplies
JavaScript and fixed-source Durable Replay with frozen snapshots.

<a id="tasks-controlled-stop-and-continuation"></a>

### Controlled stop and continuation

| Current Task | pause | cancel | append | complete |
| --- | --- | --- | --- | --- |
| active | paused, epoch +1 | cancelled, epoch +1 | Stable Steering if a Root is active; otherwise new Root, epoch +1 | completed if current epoch has no live Run or unresolved Tool; otherwise INVALID_ARGUMENT |
| paused | Same receipt state, no epoch change | cancelled, epoch +1 | active, new Root, epoch +1 | INVALID_ARGUMENT |
| completed | INVALID_ARGUMENT | INVALID_ARGUMENT | active, new Root, epoch +1 | INVALID_ARGUMENT |
| cancelled | INVALID_ARGUMENT | Same receipt state, no epoch change | active, new Root, epoch +1 | INVALID_ARGUMENT |

Stop acceptance atomically persists Task status/epoch, Run and invocation desired
state, receipt and Outbox. This immediately closes business-write and new-work
admission. The owner observes intent every 100 ms, cancels its safe provider/Tool
context, joins the current step, then settles the old Run as `cancelled` with
`task_paused` or `task_cancelled`. Already committed exchanges, Tool results,
instructions and progress remain available to the new Root as framed Task data.
An in-flight response arriving after acceptance is rejected, including its usage
write. Stop/progress events do not wake Main with a Worker business result.

Cancellation covers all current Task tree members and their invocations. The
scanner settles leaves before parents; live owners retain only lease/stop
authority, while expired or unclaimed work settles without a business claim.
Every claim-bound business write checks both ownership and the Task epoch.
An append racing stop waits outside the database transaction for the old tree
boundary, then atomically saves instruction/reactivation/new Root. The database
retains its one nonterminal Root constraint. A call interrupted during admission
remains recoverable under its frozen ToolInvocation and creates no partial append.
The private [bounded state model](../designs/tasks.md) maps these transactions,
records safety and recovery-suffix checks, and retains a fencing counterexample
with a real SQLite regression.

Only the same terminal Root's result is reported after its last descendant ends.
`completion_policy` defaults to `once`; explicit create `completionPolicy: "ongoing"`
keeps the Task active when one Root ends. `append` retains that policy. An idle
ongoing Task does not start another Root on its own: Main must append work, or
an existing [Automation](automation.md) must fire. Ordinary Root success is still reported once
to Main. Pause/cancel, unknown-effect handling and epoch fencing keep their
existing behavior.

Main can explicitly end idle active work with
`{"operation":"complete","taskRef":"<existing TaskRef>"}`. The Host rejects any
nonterminal Root/Child or queued/running/waiting/unknown ToolInvocation in the
current Task epoch with non-retryable `INVALID_ARGUMENT`. Complete does not cancel,
drain or wait. Paused work must be reactivated through append before completion;
completed/cancelled Tasks reject a fresh complete request. A repeated accepted
source or invocation returns its original receipt.

Completion stores status, timestamp and receipt atomically under Main ownership
and Task version/epoch fencing. It changes neither epoch nor completion policy,
and emits no Worker TaskUpdate or Main follow-up. Append can later reactivate
that completed Task with a new Root and epoch. See [ADR 006](../designs/tasks.md#decision-006-006--explicit-task-completion-policy).

No storage migration is needed: schema v9 already stores the immutable policy
and migrates prior Tasks to `once`. New Main Runs freeze the extended schema.
A pre-extension Main capability cannot run with the changed Tool Spec: registry
selection fails `MODEL_INCOMPATIBLE`. Finish that Run with its original binary
before upgrading; do not rewrite snapshots. Omitted and explicit once normalize
to the prior request shape, retaining receipt hashes. Completed Tool results
remain durable. Automation continues using its existing Host policy.
The original M2 tests retain stored topology and conservative unsafe-stop
fixtures. The M3 Tool harness verifies approvals and external reconciliation;
the [Child harness](../designs/delegation.md) exercises actual admission, recursive execution
and cancellation. No external exactly-once guarantee is made.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/tasks.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/tasks.md)。
