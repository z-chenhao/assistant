# Skills 与经验沉淀：测试方案

[功能与验收目标](../features/skills-experience.md) · [技术方案](../designs/skills-experience.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-022](../features/skills-experience.md#f-022) | 验证资源范围、版本漂移拒绝、二进制资源引用与上下文按需加载。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-023](../features/skills-experience.md#f-023) | 从真实纠正或执行结果自动形成候选、按证据验证并可再次调用；普通 Worker 仍不能随意改 Skill。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |

## 层级与关键路径

读取覆盖目录版本与资源漂移；经验闭环从真实纠正/执行结果开始，不直接调用 Reviewer 伪造自动沉淀。晋升要绑定同方法的验证证据，并检查普通 Worker 无写权限。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_capabilities.py](../../tests/system/verify_capabilities.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_memory_maintenance.py](../../tests/system/verify_memory_maintenance.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

- **F-023 剩余覆盖**：接入纠正/结果事件与可信 Reviewer；当前验证证据限于 workspace test，需明确其他方法的证据标准。

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
