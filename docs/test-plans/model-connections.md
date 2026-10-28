# 模型连接与账户：测试方案

[功能与验收目标](../features/model-connections.md) · [技术方案](../designs/model-connections.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-001](../features/model-connections.md#f-001) | 连接、测试、选择、工具续轮和重启均沿真实 Provider Bridge 验证。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-002](../features/model-connections.md#f-002) | 在每个声明支持的平台完成登录、刷新、退出、进程中断恢复和凭据隔离验证。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |

## 层级与关键路径

覆盖显式选择前拒绝输入、模型能力声明漂移、账户刷新/退出竞态与平台后端缺失。Live 账号验证另行记录，不用测试 socket 证明原生凭据可用。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_local_provider.py](../../tests/system/verify_local_provider.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_account.py](../../tests/system/verify_account.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

- **F-002 剩余覆盖**：补齐支持平台的凭据后端及相应设置反馈；不能把环境变量 API Key 视为 OAuth 凭据写入实现。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="provider-bridge-verification-and-operations"></a>

### Verification and operations

`make test` runs Go tests and Node contracts against an owned HTTP endpoint through
actual subprocesses. the system runner drives the CLI, reads SQLite independently,
and writes `evidence/local-provider.json` and `evidence/pi-bridge.tap`. The harness
owns launch (ephemeral HTTP port and data root), doctor (binary, Node, installed Pi),
drive (discover/add/test/select/status, keyed/manual/failure variants), evidence
(usage, failures, snapshot IDs), and cleanup (reap children, close endpoint, delete
state). Go's actual Pi crash test kills the exact Node child after a text delta,
rejects partial output, restarts the same Bridge, and reads the catalog back.
Security/cancellation contracts use the current Go/Node and system regression entry points.

For optional owned native Keychain read/update/delete verification on macOS:

```sh
ERII_TEST_KEYCHAIN=1 go test ./internal/llm/secret -run TestKeychainOwnedRoundTrip -v -timeout 30s
```

CI uses deterministic Fakes/environment references and requires no OS credential
store or real model account. Real Qwen/vLLM/SGLang/LM Studio compatibility remains
release-time Live Shadow evidence. Deterministic software conformance runs in the [system suite](product.md);
provider contracts alone do not establish semantic product scores.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
