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

- **F-002 原生覆盖**：Linux 使用独立会话 D-Bus、临时带密码 GNOME 钥匙串和真实 SecretStore；供应商仍是受控 OAuth/Responses peer。macOS 使用下文可选的所有者 Keychain 往返检查。真实 OpenAI 账号授权、桌面解锁交互及各发行版集成需单独 Live 验证。

## 详细测试入口与证明边界

本功能协议在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

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
- 账户入口：真实 CLI → Go Manager → Node/Pi → 受控 OAuth/Responses peer，经生产原生 SecretStore 登录、验证/选择、Main 对话和重启；并发刷新只旋转一次，401 失效、跨进程退出、SIGKILL 后清理和原账户重新认证保持既有语义。
- 隔离/泄漏：不同账户分别保存；access/refresh/ID token 不进入 SQLite/WAL、日志、普通 CLI 输出或 trace。Web 异步存储失败给出中英文修复提示，重试不保留上次无关错误，也不展示原生诊断。

此证据证明软件与原生存储链路；不证明真实 OpenAI 账户授权、供应商可用性或 macOS 当前机器的 Keychain 访问。Linux 禁用 cgo、不支持平台、缺库和无会话的部署不提供原生凭据写入；环境 API Key 不算 OAuth 写入回退。

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
