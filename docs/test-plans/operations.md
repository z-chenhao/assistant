# 诊断、备份恢复与费用：测试方案

[功能与验收目标](../features/operations.md) · [技术方案](../designs/operations.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-028](../features/operations.md#f-028) | 验证备份认证、坏包拒绝、隔离恢复、兼容升级及激活 fencing。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-029](../features/operations.md#f-029) | 验证价格冻结、并发预留、usage 缺失和授权等待/拒绝的执行边界。 | 保护已接通的限定范围；Live/配置另行验证 |

## 层级与关键路径

覆盖备份损坏、错误密钥、路径越界、恢复隔离与显式激活；费用覆盖价格冻结、整数边界、并发预留及 usage 缺失。仅使用 disposable root，不读取个人数据。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_operations.py](../../tests/system/verify_operations.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_spend.py](../../tests/system/verify_spend.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

现有入口仍需在后续相关实现变更时按风险运行；需求范围变化时同步扩展场景。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="operations-forward-upgrade-rehearsal-and-evidence"></a>

### Forward upgrade rehearsal and evidence

Restore a backup into an isolated root first. All three schema versions are
checked before any forward migration; a future/incompatible database rejects
startup without partially upgrading another database. State migration 026 adds
only Outbox quarantine fields/index. Empty and every supported prior schema
follow the same forward migration path. Failed DDL rolls back version and schema
together. Never lower `user_version` or downgrade in place. Retain the old binary
and original cold backup until the rehearsal completes.

```sh
make test
make test-race
make lint
make test-full
python3 tests/system/verify_operations.py --binary bin/collie --evidence evidence/operations.json
python3 tests/system/verify_capacity.py --binary bin/collie --evidence evidence/capacity.json
```

Real interactive acceptance shares `tests/system/event_broker.py`: an owned, pinned
JetStream test peer, built from the Go integration tests, with a versioned JSON
handshake and EOF shutdown. No broker server enters the product binary. TUI and
Web OAuth fixtures exercise production broker startup and immediate first input.

The system runner owns operations and capacity checks, including default-limit Child batches in a reused state root. Its existing event gate adds a real CLI/NATS/Sidecar lifecycle,
authentication, poison isolation/restart, stream upgrade, broker outage and
shutdown evidence. Existing suites retain process/Sidecar kills, late/duplicate/
out-of-order events, approval/side-effect crash windows, cancel fences and
query-only unknown handling. No separate execution or replay pipeline is added.
`TestOperationsPriorSchemaRollbackAndActiveWorkUpgrade` reconstructs real v25,
retains accepted work/snapshots, rejects partial migration, upgrades and drains
the work exactly once. Operations harness cleanup joins processes and removes
its isolated roots, ciphertext archive and restored state.

The capacity sampler runs the unchanged 100-item batch/fairness/cancel/restart
harness three times by default. Compare the same machine, fixture, binary
boundary and harness; retain individual samples, medians, ranges and binary
hashes. The 1000-item test is a separate capacity observation, not a semantic
Golden Case score. Model/Tool calls keep their existing per-call watchdogs (45 s
for a model step; frozen Tool deadlines). The provider breaker counts consecutive
infrastructure errors, cools down and resets on success; business item failures
do not trip it. Existing account monetary admission remains independent of Tool
approval and does not become a Token budget. Default scheduler and safety limits
remain unchanged until measurements justify a change. Release-time live accounts,
macOS compatibility and semantic Golden Case scoring remain separately reported.

Recorded local samples are in [operations-capacity.json](product.md).
On one Apple M4 Max (16 logical CPUs, 64 GiB), the unchanged 100-item harness had
these observations; all six samples passed with the original limits.

| Observation | Before | After |
| --- | --- | --- |
| Three samples (seconds) | 9.327, 8.726, 8.893 | 9.041, 8.971, 9.012 |
| Median (seconds) | 8.893 | 9.012 |
| Max–min (seconds) | 0.601 | 0.070 |

The median changed by about +1.3%, within the observed before range; three
samples do not establish a general performance difference. Defaults stay at
16 Children/tree, 32 global Workers and the original safety ceilings. A separate
three-cycle test reused one state root for 3000 Children: 2994 completed, six
injected business failures were aggregated, and every cycle released all Worker
leases. These are bounded deterministic capacity observations, not a production
duration or model-quality certification.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
