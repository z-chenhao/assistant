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
- **F-002 原生覆盖**：Linux 使用独立会话 D-Bus、临时带密码 GNOME 钥匙串和真实 SecretStore；供应商仍是受控 OAuth/Responses peer。macOS 使用下文可选的所有者 Keychain 往返检查。真实 OpenAI 账号授权、桌面解锁交互及各发行版集成需单独 Live 验证。
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
Security/cancellation contracts use the current Go/Node and system regression entry points.

For optional owned native Keychain read/update/delete verification on macOS:

```sh
STEWARD_TEST_KEYCHAIN=1 go test ./internal/llm/secret -run TestKeychainOwnedRoundTrip -v -timeout 30s
```

The ordinary account gate uses its owned in-memory SecretStore. Ubuntu checks
also runs `verify_account.py --native-secret-service` with an isolated native
Linux Secret Service; neither path requires a real model account. Real
Qwen/vLLM/SGLang/LM Studio compatibility remains
release-time Live Shadow evidence. Deterministic software conformance runs in the [system suite](product.md);
provider contracts alone do not establish semantic product scores.

### Linux 原生账户回归

安装测试依赖 `libsecret-1-0 libsecret-tools gnome-keyring dbus libglib2.0-bin` 后运行：

```sh
python3 tests/system/verify_account.py --binary bin/steward --native-secret-service --evidence evidence/native-account.json
```

该入口自行创建私有 D-Bus 和临时 XDG 数据/运行目录，启动带合成密码的前台 GNOME Keyring，移除桌面显示环境；不连接、锁定或删除用户当前会话的钥匙串。`secret-tool` 只用于独立检查和测试过期时间控制，不是生产 SecretStore 依赖。临时密码和 OAuth 合成凭据只经 stdin/内存传输，结束后回收进程和私有文件。CI 通过既有可取消的 entrypoint/process_step 执行并保留同一证据格式。

- 原生组件：二进制值跨进程读回、替换、删除及重复删除；无效输入、取消和不可用会话失败关闭；暂停私有钥匙串进程使请求在途等待，确认读操作按期限取消并可恢复；锁定后读、写、删除均有界失败且不暴露诊断；重启钥匙串并用原密码解锁后原值仍在。
- 账户入口：真实 CLI → Go Manager → isolated Go Bridge → 受控 OAuth/Responses peer，经生产原生 SecretStore 登录、验证/选择、Main 对话和重启；并发刷新只旋转一次，401 失效、跨进程退出、SIGKILL 后清理和原账户重新认证保持既有语义。
- 隔离/泄漏：不同账户分别保存；access/refresh/ID token 不进入 SQLite/WAL、日志、普通 CLI 输出或 trace。Web 异步存储失败给出中英文修复提示，重试不保留上次无关错误，也不展示原生诊断。

此证据证明软件与原生存储链路；不证明真实 OpenAI 账户授权、供应商可用性或 macOS 当前机器的 Keychain 访问。Linux 禁用 cgo、不支持平台、缺库和无会话的部署不提供原生凭据写入；环境 API Key 不算 OAuth 写入回退。

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
