# 诊断、备份恢复与费用

检查运行状态、安全备份恢复，并观察和约束模型费用。

[需求主索引](../feature-map.md) · [技术方案](../designs/operations.md) · [测试方案](../test-plans/operations.md)

## 功能说明

操作员能检查健康、导出受控诊断、备份并隔离恢复数据。

操作员能查看用量，并按明确价格元数据控制账户费用。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-028"></a>

### F-028 · 本地诊断与加密恢复

**需求**：操作员能检查健康、导出受控诊断、备份并隔离恢复数据。

**当前实现**：doctor/status/ops、日志轮转、加密备份、恢复验证与显式切换已实现。

**边界与剩余工作**：备份不包含凭据、Guest 镜像等外部资源；不能同时运行同身份的两个恢复副本。

**验收目标**：验证备份认证、坏包拒绝、隔离恢复、兼容升级及激活 fencing。

<a id="f-029"></a>

### F-029 · 用量记账与配置型费用授权

**需求**：操作员能查看用量，并按明确价格元数据控制账户费用。

**当前实现**：provider_usage、费用快照、预留和授权已接入模型 admission。

**边界与剩余工作**：无价格时不能声称零费用；不是供应商账单或按任务动态模型路由。

**验收目标**：验证价格冻结、并发预留、usage 缺失和授权等待/拒绝的执行边界。

## 如何使用

运行 `collie doctor` 检查安装；`collie status` 查看模型就绪；`collie ops health` 查看运行健康；`collie spend status` 查看费用状态。

备份恢复按现有指南提供 32 字节密钥的 SecretStore 引用，先恢复到隔离数据根。现有 `collie ops trace RUN_ID` 只提供简化轨迹；完整可视化由 F-031 跟踪。

## 详细操作参考

- [运维操作详情](#operations-production-recovery-and-operations)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="operations-production-recovery-and-operations"></a>

## Production recovery and operations

The runtime state is authoritative. JetStream is an at-least-once wakeup bus.
Restarts reuse accepted model results, frozen Model/Capability Snapshots,
ToolInvocation identities and fixed-source Code Mode ledgers. There is no
exactly-once guarantee for external effects. `unknown` stays query-only or
requires an operator decision; a backup cannot reveal effects performed after
its recovery point.

<a id="operations-run-and-diagnose"></a>

### Run and diagnose

`collie -config CONFIG web serve`, `tui`, and `automation work` own the
foreground production lifecycle. Setup remains available without a usable
profile. The broker and durable consumer initialize before the Tool Registry,
ComputerProvider and Sidecar handshake. The existing Outbox/Inbox and recovery
scan start before execution and work ingress. Runtime publication uses an atomic
core reference, so a Setup UI never captures an obsolete Actor after activation.
Interactive model selection wakes startup immediately and waits for actual
runtime readiness before completing the Web response or printing the TUI input
prompt. The wait is bounded at 15 seconds and stops during shutdown.

The scan is the same `Runs.Recover` used by `Next`: due Automation admission,
Code execution claim/wake reconciliation, Approval expiry, query-only unknown
outcomes, controlled stops, immutable conversation/result intents and Main
follow-ups. Run scheduling reclaims expired leases through the existing CAS and
epoch fences. Artifact startup reconciliation and channel delivery recovery
retain their existing boundaries. See [replay](../designs/codemode.md),
[Computer](../designs/computer-use.md), [channels](channels.md) and [delivery](../designs/channels.md).

SIGTERM stops ingress and joins accepted executions at durable boundaries. It
stops Memory maintenance, drains internal wakeups with a two-second bound, closes
the consumer/broker, then closes the Provider and databases. It never cancels a
Task. Sidecar executions remain per invocation; the startup handshake process
receives `runtime.shutdown` and is joined before ingress. No idle VM is retained
by the handshake. A broker disconnect retains intents, closes new work ingress,
and reconnects without failing accepted work. Setup/progress endpoints remain
available. Explicit one-shot CLI commands (`chat work`, `delivery work`, etc.)
can inspect/recover the local authoritative state without a running broker.
Only `-test-bridge` selects a Fake; `-test-events` additionally drives the real
NATS lifecycle in isolated CI.

```sh
collie -config CONFIG ops health
collie -config CONFIG ops broker
collie -config CONFIG ops trace RUN_ID
collie -config CONFIG ops report > bug-report.json
```

`health` contains counts of live/expired claims, pending Code wakes, conversation
intents, due Automation/Approval, pending/unknown delivery and Tool outcomes,
active Root queue ages/service tickets, circuit breakers and configured scheduler
limits. Root detail is limited to 100 rows; traces return the latest 1000 points.
`broker` reads retained messages/bytes, consumer pending/redelivery and DLQ count.
These are operator commands; they are never model Tools, Web progress data,
notifications or model-visible budget discovery. Querying them never drives an
Agent or sends a message.

`report` is a bounded allowlist of counts, limits and opaque correlation IDs.
`trace` includes Run/Parent/Root IDs, sequence, event kind and Tool call ID; it
omits the event text. Never attach raw SQLite, logs, full RunTrace, prompts,
Tool arguments/results, endpoints, account/destination identifiers, config,
Artifacts or archives to a bug report. Reports remain local until an operator
explicitly shares them. Credentials are resolved only by SecretStore. Structured
logs contain fixed messages/codes and correlation metadata. App/trace files each
rotate at 16 MiB with two backups (96 MiB total by default); rotation is serialized
across CLI processes. Raw domain audit rows are retained, not silently pruned.

<a id="operations-broker-authentication-bounds-and-dlq"></a>

### Broker authentication, bounds and DLQ

Use a dedicated `stream_prefix` per data root. Literal loopback can use an
unauthenticated development broker. Every remote broker requires `tls://` with
normal certificate verification and `token_ref` resolved through SecretStore.
URLs reject embedded credentials, query strings and fragments. Never place the
token itself in YAML, command arguments, SQLite or reports. Missing/invalid
credentials fail closed with a constant error, without the token or provider
error. Rotate the referenced token in SecretStore and restart the service.

Defaults: event Stream `_events_v1`, File storage, DiscardNew, seven days,
1 GiB, 1,000,000 events, 1 MiB/event. Durable consumers permit 32 unacknowledged
messages and pull one at a time. AckWait defaults to 30 seconds (10 ms–5 min),
retry interval to one second (1 ms–1 min). Ingress/recovery does not depend on
broker deduplication or global event ordering.

Provision broker storage for both configured Stream reservations plus operating
headroom. Reconnect reads and strictly validates existing Stream configuration;
it does not request another reservation or silently alter retention or access.

After five unsuccessful consumer deliveries, the adapter durably writes a
content-free quarantine receipt before acknowledging the original message. A
crash between those writes can duplicate the receipt; its consumer/stream
sequence is its stable identity. The receipt contains only consumer, stream
sequence, attempt count, body SHA-256 and a fixed failure code. Raw bodies and
headers stay in the original protected stream. The independent `_dlq_v1` Stream
retains receipts for 30 days, at most 64 MiB/10,000 entries/4 KiB per entry.
Configure `poison_attempts` (2–100) for both consumption and non-transient Outbox
publication failures. Malformed stored envelopes quarantine immediately. Healthy
Outbox rows are served before previously attempted rows. Broker outages do not
increment poison failures or exhaust their threshold. No DLQ event notifies the
user. The local structured warning and `ops broker`/`ops health` are the developer
alert surface. A full broker/DLQ retains authoritative intent and requires
operator remediation; it does not authorize dropping records or replaying effects.

```sh
# Requeue a valid internal wakeup using its displayed attempt count as a fence.
collie -config CONFIG ops retry-event EVENT_ID ATTEMPTS
```

This operation cannot retry a Tool or channel side effect and refuses malformed
envelopes and stale decisions. Broker receipt inspection is a privileged broker
operation using the displayed stream sequence, not a user channel. Diagnose the
handler first; authoritative recovery scans continue independently.

Existing unbounded v1 Streams fail validation instead of silently acquiring new
retention policy. Stop Collie processes, back up state, and explicitly run:

```sh
collie -config CONFIG ops upgrade-broker
```

The command verifies the owned subject/storage/retention contract and refuses
bounds that would immediately discard retained messages by count/bytes/age.
It preserves messages, subjects and durable positions. Set a supported larger
retention window or settle/export old wakeups before retrying. Normal `broker`
and serving commands validate policy; they never silently alter existing bounds.

<a id="operations-cold-encrypted-backup-and-isolated-restore"></a>

### Cold encrypted backup and isolated restore

Stop **every** process using the data root, including temporary CLI commands and
external database writers. Every Collie process holds a shared OS lock. Backup
requires the exclusive lock and refuses a live instance; process death releases
the lock. Do not copy live WAL files. The backup verifies all three databases,
checkpoints their WALs, checks foreign keys and live Artifact hashes, then streams
a consistent archive directly into encryption. No plaintext staging archive is
created. Symlinks, hard links and special files in the snapshot are refused.

Create/escrow a dedicated 32-byte encryption key in SecretStore first. A reference
may use `env:NAME` for a controlled deployment, or the OS credential store. The
archive never contains key material, and is useless if its key is lost. The
archive uses a fresh 256-bit salt, HMAC-SHA256 key derivation and AES-256-GCM
records, with authenticated order/length and a final marker. Truncation, append,
corruption and a wrong key fail closed. Maximum snapshot size is 64 GiB/65,536
files, with an 8 MiB manifest. The private manifest records version, database
schemas, sizes and SHA-256 hashes. Its CLI response is local operational data,
not the shareable bug report.

```sh
collie -config SOURCE_CONFIG ops backup /private/backups/collie.collie KEY_REF
collie -config ISOLATED_CONFIG ops restore /private/backups/collie.collie KEY_REF
# Inspect/retire the source and reconcile post-backup external outcomes first.
collie -config ISOLATED_CONFIG ops activate source-retired-and-outcomes-reconciled
collie -config ISOLATED_CONFIG status
```

The archive includes `state.db`, `conversation.db`, `memory.db` and immutable
`artifacts/` bytes, including tombstone/claim/approval/replay state. It excludes
credentials, logs, auth locks, configuration, workspace/Skill directories,
Computer VM/checkpoint storage and external provider ledgers. Back up those
resources separately using their owners' policy. A new machine must provision
the same credential references and Memory decryption key separately; missing
credentials enter repairable setup, never a paid/Fake fallback. Memory's existing
encrypted material remains encrypted. Sensitive Conversation/Artifact material
is encrypted in the archive; its restored at-rest representation follows the
original storage policy.

Restore writes only a new, isolated data root and never overwrites an existing
root or archive. It verifies authentication, path/type bounds, exact manifest
inventory, file hashes, integrity, foreign keys, Artifact bytes and compatible
versions before atomically publishing the root. Partial restores are removed.
Files/directories use 0600/0700. A sibling-parent maintenance lock serializes
publication. The restored root remains fenced until the explicit cutover command;
neither model work nor external effects run before activation.

Activation attests that the source is retired and the operator checked effects
after the backup recovery point against authoritative external records. Never
run two copies of one identity. Preserve provider idempotency receipts and
reconcile `unknown`; a cold snapshot cannot promise exactly-once effects or
recover work accepted only after its recovery point. The existing fixed-source
and frozen-snapshot compatibility checks still apply after restore.

Preserve the frozen execution environment configuration when continuing accepted
Runs. In particular, the default local workspace is derived from `data_root`;
restoring to a different root changes that default and fails compatibility
checks. An explicit `execution.root` can preserve the original environment after
the source is retired. Workspace files and toolchains are outside this archive:
provision them separately. Installing different OCR language data or changing
other frozen capabilities likewise does not upgrade existing Runs in place.

<a id="security-security"></a>

## Security

Keep credentials out of issues, logs, prompts, YAML and artifacts. Configuration
uses SecretStore references. Never upload a real data root or authentication URL
as a bug reproduction.

For a suspected vulnerability, use GitHub private vulnerability reporting when
available, or contact the repository maintainers privately before opening a public
issue. Include the affected revision, a minimal disposable reproduction, impact
and observed behavior. Do not exercise the issue against another person's data.

The [technical baseline](../designs/product-overview.md#contract-0-文档效力与实现规则) defines approval, capability,
credential and replay boundaries. [Operations](#) describes
backup and recovery. Offline test success does not establish live-provider or
Computer compatibility.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/operations.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/operations.md)。
