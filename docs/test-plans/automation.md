# 自动化与安静监控：测试方案

[功能与验收目标](../features/automation.md) · [技术方案](../designs/automation.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-015](../features/automation.md#f-015) | 验证时区、重复触发、修改/取消竞态、停机补偿及重启不重复工作。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-016](../features/automation.md#f-016) | 验证跨入、持续在内、跨出再跨入、旧 fire 及缺失观察值的行为。 | 保护已接通的限定范围；Live/配置另行验证 |

## 层级与关键路径

覆盖时区、DST、到期补偿、并发调度、计划 CAS、旧 fire、取消与 Forget。阈值用受控观察序列验证安静/通知切换；真实数据源连接在 F-025 单独验证。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_automation.py](../../tests/system/verify_automation.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

现有入口仍需在后续相关实现变更时按风险运行；需求范围变化时同步扩展场景。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="automation-verification"></a>

### Verification

```sh
make build
python3 tests/system/verify_automation.py --binary bin/erii --evidence evidence/automation.json
go test -race ./internal/automation ./internal/agent/sqlite -run 'TestAutomation|TestCalendar|TestRejectMalformed' -count=5
```

The reusable harness owns isolated Fake profile setup, CLI/model Tool ingress,
fixed clocks, state/Tool/snapshot/delivery readback, actual SIGKILL at four
fire/Outbox/admission boundaries, restart and stable duplicate drain. It joins
children and deletes disposable state. The system runner selects it; repeat count is explicit. `tests/fixtures/automation-v1.json` pins GC-02/14 scheduling fragments;
GC-08 covers reminders and delayed cancellation. These are deterministic L0/L1
protocol checks. They do not award full Golden Case semantic/model scores or
establish channel or full M7 hardening acceptance. See [channels](../features/channels.md)
and [operations](../features/operations.md) for those implementations and their own evidence.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
