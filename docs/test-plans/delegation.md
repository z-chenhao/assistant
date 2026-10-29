# Worker 子任务委派：测试方案

[功能与验收目标](../features/delegation.md) · [技术方案](../designs/delegation.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-006](../features/delegation.md#f-006) | 验证递归、并发限额、等待时释放执行槽、取消传播及恢复不重复创建 Child。 | 保护已接通的限定范围；Live/配置另行验证 |

## 层级与关键路径

覆盖递归上限、Worker 权限削减、Parent 等待释放槽位、Child 完成先于 Parent 等待及重启后采用同一结果。不能用测试构造的树替代真实 admission。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_children.py](../../tests/system/verify_children.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_scheduling.py](../../tests/system/verify_scheduling.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

现有入口仍需在后续相关实现变更时按风险运行；需求范围变化时同步扩展场景。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="children-verification-and-operations"></a>

### Verification and operations

```sh
make build
python3 tests/system/verify_children.py --binary bin/collie --evidence evidence/children.json
```

The maintained harness owns Launch (isolated data root and explicit Fake), Doctor
(model add/probe/select), Drive (PTY and production CLI), Evidence (Run tree,
capabilities, model requests, results, approvals, Artifacts, counters and Outbox),
and Cleanup (reap owned processes and delete disposable state). The system runner selects it; repeat count is explicit. It tests Root/Child/Grandchild, explicit Artifact passage,
forged calls, invalid parameters, inaccessible references, Child failure,
recoverable approval, live cancellation and six SIGKILL/restart boundaries.

`children-*.json` records before/after state and binary SHA256. Unit/race tests
separately cover admission rollback, simultaneous duplicate callers, old claims,
late Task epochs, profile changes, ceilings, fixed reductions and per-invocation
approvals. A successful protocol fixture is not model-quality evidence. Full
Golden Cases are not scored by this Child fixture; [Core](product.md) and
[M6](product.md) gates cover additional mechanics with their own proof limits.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
