# JavaScript Code Mode 与持久恢复：测试方案

[功能与验收目标](../features/codemode.md) · [技术方案](../designs/codemode.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-008](../features/codemode.md#f-008) | 验证真实 QuickJS 执行、等待/重启、Store 观察复用、参数漂移拒绝及已完成调用不重做。 | 保护已接通的限定范围；Live/配置另行验证 |

## 层级与关键路径

使用真实 Node/QuickJS，而非脚本结果 Fake；覆盖 Promise 并发、Store 读写、资源等待释放 VM、固定源漂移及重放命中。记录未 await 的已接受调用，不声称失败脚本会回滚效果。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_codemode.py](../../tests/system/verify_codemode.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_code_recovery.py](../../tests/system/verify_code_recovery.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

现有入口仍需在后续相关实现变更时按风险运行；需求范围变化时同步扩展场景。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="codemode-launch-and-verify"></a>

### Launch and verify

Install Node **24.21.0**, then install the exact WASM dependency:

```sh
npm ci --prefix runtime/javascript --ignore-scripts
make build
python3 tests/system/verify_codemode.py --binary bin/erii --evidence evidence/codemode.json
python3 tests/system/verify_code_recovery.py --binary bin/erii --evidence evidence/code-recovery.json
```

The reusable harness owns Launch, Doctor, Drive, Evidence, and Cleanup. It
creates temporary configuration and databases, activates the explicit
`codemode` model fixture, then drives `chat send`/`chat work`. The model fixture
selects requests; the real Node/Pi/QuickJS VM executes the JavaScript. The
Computer Fake is injected only with this test fixture. Production has no Fake
Computer fallback. The configured production [Computer](../designs/computer-use.md) and
[Computer Use](../features/computer-use.md) adapters have separate harnesses and provisioning
requirements.

The report includes exact explicit outputs, nested invocation identities,
source/Store/event readback, a real Child Worker result with inherited frozen
model identity, approval rejection without Provider dispatch, schema version,
before/after-effect SIGKILL recovery with one durable identity and one external
receipt, and cleanup. The system runner selects this harness; repeat count is explicit. `make test` includes
real Sidecar transport, Promise, resource, cancellation, and security fixtures.

To enable Code Mode in an ordinary installation:

```yaml
version: 1
javascript:
  command: /absolute/path/to/node
  script: /absolute/path/to/erii/runtime/javascript/src/sidecar.mjs
```

Both paths must be trusted and outside an editable coding workspace. There is
no implicit executable discovery or default model. Capability snapshots freeze
the versioned ToolSpec for each new Run. Existing snapshots retain their
original Tool surface. A Sidecar protocol failure produces a sanitized Tool
error, never a raw exception or inherited credential.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
