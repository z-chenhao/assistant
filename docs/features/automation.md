# 自动化与安静监控

按时间执行提醒或周期工作，监控跨阈值时才通知。

[需求主索引](../feature-map.md) · [技术方案](../designs/automation.md) · [测试方案](../test-plans/automation.md)

## 功能说明

用户可设定明确时区的 at/cron 计划，并修改或取消。

周期观察在首次进入阈值区间时通知，其余成功检查保持安静。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-015"></a>

### F-015 · 一次性与周期自动化

**需求**：用户可设定明确时区的 at/cron 计划，并修改或取消。

**当前实现**：计划、触发记录、Task/Root admission、版本去重和恢复已接通。

**边界与剩余工作**：执行需要应用或前台 worker 在运行；不自动分离守护进程。

**验收目标**：验证时区、重复触发、修改/取消竞态、停机补偿及重启不重复工作。

<a id="f-016"></a>

### F-016 · 阈值状态与安静通知

**需求**：周期观察在首次进入阈值区间时通知，其余成功检查保持安静。

**当前实现**：Root observe 写入每次 fire 的观察值，保存阈值跨越状态并控制 Reporter 通知。

**边界与剩余工作**：本需求是监控机制，不内置价格或其他在线数据源；真实数据接入由 F-025 跟踪。

**验收目标**：验证跨入、持续在内、跨出再跨入、旧 fire 及缺失观察值的行为。

## 如何使用

用户可在对话中交代明确时间或周期的工作。CLI `erii automation list` 查看计划，`erii automation manage REQUEST_KEY` 从 stdin 接受 create/update/cancel JSON。

保持正常应用、Web 服务或 `erii automation work` 前台运行。`automation tick` 只保存到期事件；监控仍需要可用工具取得实际观察值。

## 详细操作参考

- [自动化操作详情](#automation-automation-operations)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="automation-automation-operations"></a>

## Automation operations

Automation persists user intent separately from Task. The scheduler writes one
`automation_fire` and `ai.erii.automation_fired.v1` Outbox event atomically. It
never runs a model. Actor's ordinary `Next` recovery consumes authoritative fire
rows, writes its Inbox receipt, creates an ongoing Task if needed, freezes the
current model/capabilities in a new Root, and emits `run_ready` in one transaction.
Execution uses the existing RunClaim, Agent Loop, ToolInvocation, approval and
Main Reporter/Delivery path. Lost or repeated queue wakes cannot duplicate work.

Run the foreground scheduler/executor with an active, tested profile:

```sh
erii automation work
```

Keep that command running under the installation's process supervisor. It scans
at startup and at one-second intervals while idle. SIGINT/SIGTERM stops and joins
its work at existing recoverable boundaries; process exit does not cancel intent.
TUI/CLI ingress may run in another process against the same configured data root.
`chat work` also scans due Automation during its normal Actor drain.
`automation tick` only persists due events and exits; it does not execute them.
No scheduler process is implicitly detached by the application.

Create from a JSON object on stdin, with a stable request key for retries:

```sh
erii automation manage monthly-review < review.json
erii automation list
```

`review.json`:

```json
{"operation":"create","instruction":"Remind me to review my long-term goals.","triggerType":"cron","triggerSpec":"0 20 * * 0L","timezone":"Asia/Shanghai"}
```

`at` uses an RFC3339 timestamp with explicit offset. Every schedule stores an
explicit IANA timezone (`UTC` works; process-local `Local` is rejected). Cron has
five numeric fields: minute, hour, day of month, month, weekday (0=Sunday through
6=Saturday). Lists, ranges and positive steps work; `0L` through `6L` in the
weekday field select that month's last weekday. Restricted day-of-month and
weekday fields use ordinary cron OR semantics; wildcard steps retain wildcard
semantics (for example `0 8 */1 * 0` still means Sunday). See the
[crontab day-field rule](https://man7.org/linux/man-pages/man5/crontab.5.html). A new schedule fires strictly
after creation/edit time. Nonexistent DST wall minutes are skipped; both distinct
UTC occurrences of a repeated wall minute fire. Invalid calendars fail closed.

Missed occurrences coalesce to the original persisted due occurrence; cron's
next fire advances strictly past the recovery scan time. This deliberately avoids
replaying every missed day. Already accepted fires remain durable and execute
in order, with at most one live Root for the ongoing Task. A failed Root leaves
the Task active. A completed Root also leaves an Automation Task active.

Update supplies `operation: "update"`, `automationRef`, current `version` from
list, and the complete new instruction/schedule/timezone/optional threshold.
Cancel supplies only `operation: "cancel"`, `automationRef` and `version`.
Each schedule mutation is CAS; stale versions return `INVALID_ARGUMENT` without
writes. Scheduler bookkeeping and observations also advance the visible version;
read list again after a conflict. Edit increments an independent schedule version,
invalidating queued fires and suppressing stale results. Reporter and Main inbox
promotion recheck the persisted fire version, including results committed before
an edit and inbox entries queued before an edit. Current in-flight steps
keep their existing snapshots and approval boundaries. Cancel disables future
fires and uses the existing Task cancel/epoch mechanism for the current tree.
Task pause/cancel also disables its Automation. A stopped Task cannot be restarted
by a delayed fire or schedule edit; create a new Automation to establish new intent.
Memory Forget revokes prior intent as well as Task context. Creation freezes the
Agent's current privacy epoch in `intent_privacy_epoch` within the acceptance
transaction. Admission compares it with the current barrier before creating any
Task, including when the first fire was persisted before Forget. A mismatched
epoch or a linked Task predating the barrier commits the fire as `skipped` with
an Inbox receipt.
It never replaces the Task to regain its context. Fresh chat and newly created
Automations continue through the same Actor without manual cancellation of the
old schedule. A new Automation establishes fresh intent; editing the old schedule
does not restore revoked intent, even before its first Task. Receipt retries,
schedule edits and scheduler timestamps never renew the frozen privacy epoch.
Storage failures still abort admission and remain recoverable.

List uses the same privacy projection for the Main Tool and trusted CLI. A revoked
row retains only its Host-generated `automationRef`, current `version` and
`enabled` control state. `instruction`, `triggerType`, `triggerSpec` and `timezone`
are empty strings; `nextFire` and `below` are absent. This permits safe cancellation
without returning forgotten intent, schedule or threshold evidence. Fresh rows
retain their full view. Original fields remain in local audit. Edits and receipt
retries never restore visibility across the immutable intent epoch. Existing
Task search, Root admission and Reporter/follow-up privacy fences also protect
Automation-derived Task context; create/update/cancel receipts expose only the
Host reference and acceptance, not stored instructions.


Completed one-shot `at` intent retains `enabled=true, next_fire_at=NULL` for audit;
it never fires twice. Cancellation is explicit and disables it.

<a id="automation-threshold-monitoring"></a>

### Threshold monitoring

A create/update may include `below`, a nonnegative integer threshold (maximum
10^12, same unit as observations). The instruction specifies the real data source
and business units. This is not a price-fetching service: the Root uses its
existing scoped Tools to obtain evidence, then calls
`builtin__automation__observe({"value":1750})`. The Host binds the call to that
Root's persisted fire; the model cannot choose identities, dates or replay keys.
Only the linked Root may write an observation. Main and other Roots/Children
cannot update that state. Claims and schedule versions fence late writes.

The Host records one authoritative value per fire. Repeated sources returning
that value are idempotent; a different value under the same fire is
`NON_DETERMINISTIC_REPLAY`. A later observation can change the state from outside
to inside the threshold, permitting one completion notice. Further below-threshold
values remain quiet until an outside value resets it. Old/out-of-order fires do
not reset newer monitor state. No valid observation produces one `needs_input`
update rather than an invented measurement. Failures/unknown effects still use
the existing meaningful-notification path. Successful quiet observations create
no Main turn or DeliveryRequest. Workers never deliver directly to the user.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/automation.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/automation.md)。
