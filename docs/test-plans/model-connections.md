# 模型连接与账户：测试方案

[功能与验收目标](../features/model-connections.md) · [技术方案](../designs/model-connections.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导特定版本已通过或真实模型效果已达标。

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

| 入口 | 覆盖用途与限制 |
| --- | --- |
| [tests/system/verify_local_provider.py](../../tests/system/verify_local_provider.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 |
| [tests/system/verify_account.py](../../tests/system/verify_account.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 |

- **F-002 文件覆盖**：测试验证目录和文件权限、重新打开、轮换、删除和拒绝符号链接。
- **Web 登录未回调**：`verify_channels.py` 的 `account_presentation` 用浏览器验证单击直达受控授权页。自动打开后只显示一行等待状态和取消操作，弹窗被拦截时显示手动入口。未发送回调时验证页面可取消、没有写入账号、工作区仍需设置，随后重新发起并以受控 OAuth peer 完成授权与模型验证和选择。现有 `channels` CI 门禁还验证 Resources 默认收起模型，可展开连接标识，测试和切换完成后原位替换为对应状态。重新授权后测试可恢复既有当前模型。受控 SecretStore 验证重启、页面加载和轮询期间无模型凭据读取，显式加载模型目录时才读取。无有效 Probe 时先验证，已有 Probe 时可直接切换，账户模型按账户分组，重连与取消回到 Resources，登录状态只显示在发起页面。
- **Google 登录 Live 验收**：用获准的测试账号从 Web 的 Connect a model 发起 ChatGPT 登录，选择 Google，验证授权页正常完成、返回 Steward 后可选择模型。若授权页报错且重试无效，记录阶段、时间、公开错误文案以及本地状态，并确认可回到 Steward 取消和重新发起。证据只保留脱敏内容。

## 详细测试入口与证明边界

本功能协议在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="provider-bridge-verification-and-operations"></a>

### Verification and operations

`make test` runs Go tests and Node contracts against an owned HTTP endpoint through
actual subprocesses. the system runner drives the CLI, reads SQLite independently,
and writes `evidence/local-provider.json` and `evidence/pi-bridge.tap`. The harness
owns launch (ephemeral HTTP port and data root), doctor (binary, native model bridge, installed QuickJS),
drive (discover/add/test/select/status, keyed/manual/failure variants), evidence
(usage, failures, snapshot IDs), and cleanup (reap children, close endpoint, delete
state). Go's native bridge crash test kills the exact bridge child after a text delta,
rejects partial output, restarts the same Bridge, and reads the catalog back.
The Go component checks run the production RPC server in their own test-binary
subprocess, so a clean checkout does not need a prebuilt `bin/steward` or Node
to run these checks. The Node compatibility launcher remains covered by its
package tests, whose pretest step builds the application binary.
Security/cancellation contracts use the current Go/Node and system regression entry points.

### 私有文件账户回归

```sh
python3 tests/system/verify_account.py --binary bin/steward --evidence evidence/file-account.json
```

该入口使用隔离数据根、真实 CLI、文件 SecretStore 和受控 OAuth/Responses peer，覆盖跨进程轮换、并发刷新、401 失效、退出、SIGKILL 后清理与原账户重新连接；凭据不进入 SQLite/WAL、日志、普通输出或 Trace。文件组件另验权限、重新打开、轮换、删除、路径与链接拒绝。无需 D-Bus、钥匙串或凭据服务进程。真实供应商授权仍需独立 Live 验证。

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。

Native Go Bridge 的 `independent generation deadlines abort the right phase` 回归并行覆盖 30 秒无响应头、HTTP 200 或 HTTP 400 后 90 秒无响应数据、持续流动到 180 秒总上限，以及收到 HTTP 200 后静默 32 秒仍能完成。检查对应的诊断阶段、错误或完成结果。Go Loop 回归验证实际派发的 context 为 Bridge 总预算和清理留足时间。

### Internal Go protocol reference coverage

`TestModelDependencyBoundaries` 保护 Go 模型实现的依赖方向，随 `make test` 在 PR 中运行；具体规则见[工程回归](engineering.md#工程基础重构回归)。

`go test ./internal/llm/provider` covers all ten chat wire families with an owned HTTP
peer, streaming tool arguments, usage and terminal handling; four classifier
protocols (including llama.cpp logit readout escalation), OpenRouter images,
Responses fetch/cancel, AWS EventStream CRC checks and provider credential names.
WebSocket checks cover a real HTTP Upgrade, masked client frames, cached input
continuation, cancellation and SSE fallback before output. Catalog tests check
42 providers separately from protocol execution. None of these fixtures proves
live service availability, cloud credential discovery or unimplemented provider
login flows. Product tests continue to cover the two current connection paths.

`provider-bridge/test/native-account.test.mjs` drives the real Go subprocess against
the owned OAuth peer, including PKCE, JWT rejection, refresh rotation, revocation
and identity binding. The Go Host final-acceptance tests use that native model
bridge protocol; account configuration fixtures explicitly select a loopback
transport and do not change production endpoints.

## 原生协议回放边界

`TestMistralNativeFieldsAndToolReplay` 使用受控 HTTP peer 检查 Mistral 原生字段、九位工具调用身份与结果关联、thinking 回放及输入不可变；`TestResponsesTokenLimitKeepsPartialOutput` 验证底层协议保留 Responses 的 max_output_tokens 截断文本和用量；Host 适配器因无法表达截断状态而拒绝其作为完成结果。`TestWebsocketDoesNotResumeIncompleteResponse` 防止截断响应复用不完整的缓存基线。真实供应商兼容性另行验证。

`TestExplicitCloudEndpointsArePreserved` 验证 Bedrock、Vertex 和 Azure 的显式 HTTPS/HTTP 端点不会被区域或环境变量替换；默认端点仍按区域或 Azure 环境配置解析。

账户目录选用回归：`TestAccountCatalogSelectionNeedsNoProbeAndFencesChanges` 验证目录选用不生成 Probe、保留 128,000 输出与图片能力，以及目录缺失、认证版本变化、配置竞争、事件写入失败均不部分激活；退出后不可用。`TestAccountCatalogUsesModelLimitsAndImageCapabilities` 验证缺失元数据补充自 Pi 聊天目录、明确服务端字段优先、隐藏模型不可选。自定义端点原有 Probe／取消／冻结测试继续运行。Schema 44 只更新激活触发器，已有历史迁移与新库门禁继续覆盖。
