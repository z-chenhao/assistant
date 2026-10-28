# 统一工具执行与审批：测试方案

[功能与验收目标](../features/tools-approvals.md) · [技术方案](../designs/tools-approvals.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-007](../features/tools-approvals.md#f-007) | 验证拒绝/过期/失效审批不 dispatch、取消 fencing、崩溃窗口及 query-only 恢复。 | 保护已接通的限定范围；Live/配置另行验证 |

## 层级与关键路径

覆盖批准前/后及 dispatch 前/后中断、Steering 失效审批、可信 query、过期/冲突决定和 unknown 保留。断言外部效果次数与内部回执身份。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_tools.py](../../tests/system/verify_tools.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_delivery.py](../../tests/system/verify_delivery.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

现有入口仍需在后续相关实现变更时按风险运行；需求范围变化时同步扩展场景。

## 详细测试入口与证明边界

工具命名回归：`TestNamespacePreservesSingleUnderscoresAndSourceBoundaries` 验证 builtin 的工具名前缀和 MCP server 的精确匹配；`TestMCPWireNameKeepsServerAndToolUnderscores` 通过真实 MCP stdio 发现 `mcp__browser_use__click_button`，并拒绝含结构分界符的 server/tool 名。`TestLegacyBuiltinSnapshotIsRejectedWithoutAliasesOrMutation` 验证旧名不成为别名、冻结快照不被改写且失败为 `MODEL_INCOMPATIBLE`。Child 与 Code Mode 的 SQLite/CLI 回归继续验证新名下的权限隔离、终态和恢复。

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="tools-deterministic-evidence"></a>

### Deterministic evidence

The system runner selects `tests/system/verify_tools.py` for Tool regression. Each run launches isolated
real CLI processes with explicit `-test-bridge approval`/`approval-never` fixtures,
proves setup readiness, drives decisions and steering, kills Go at four execution
boundaries for both unsafe strategies, injects an oversized external receipt after
a Worker effect commit/SIGKILL and later reconciles the original call, reads back
invocation/approval/Outbox and a separate Fake external ledger, then joins children
and removes temporary data.
Reports are `evidence/tools-{1..5}.json`. The fixture tool is unavailable without
explicit test injection. The Fake ledger is not a domain database and has no
access to Run/Task state.

Focused Go tests cover expiry/denial, wrong principal/hash, duplicate decisions,
argument drift guards, unsupported schema rejection, resource limits/conflicts,
independent out-of-order completion, unknown reconciliation and migration
rollback. `go test -race ./...` covers their concurrent boundaries. These are
SC-06/07 and GC-11/12/13 approval/recovery **fragments**; full real purchase,
calendar and booking quality requires separate Live evidence. [Artifact handling](../features/artifacts-media.md)
and [channel delivery](../features/channels.md) have their own maintained harnesses; excess
inline output still fails `RESOURCE_LIMIT`. These Tool fragments alone do not
establish those capabilities or full Golden Case scores.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
