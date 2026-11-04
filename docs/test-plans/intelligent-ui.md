# 受控富 UI 交付：测试方案

[功能与验收目标](../features/intelligent-ui.md) · [技术方案](../designs/intelligent-ui.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导特定版本已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-021](../features/intelligent-ui.md#f-021) | 普通 Main 回复经 Bridge/Delivery 生成表格、进度与表单；用户消息不变成组件，单元格文字转义。 | `verify_channels.py --rich-replies-only` 不注入投影或历史行 |
| F-021 | 相同表单并发重试与重启只接纳一次，修改已接受回答、跨消息表单与额外字段被拒绝；回答不创建审批。 | 同一系统场景读回真实输入、回执和冻结投影 |
| F-021 | 无 JavaScript 原生提交；Unicode 字节上限、失败发送保留答案、失去回执后恢复已保存状态、其他表单草稿与普通输入框不丢失。 | Chromium `rich-replies` 阶段，同时保留桌面/移动端截图 |
| F-021 | 未知版本/组件、越界表格、重复字段、伪造审批/Artifact/Host 身份均降级；授权、CSRF 与字段集合验证保持原边界。 | UI、HTTP 与真实 SQLite 组件回归 |

## 层级与关键路径

必须通过正常回复生成端到端组件，不能直接往数据库注入 UISpec 冒充生成链。普通文字请求经显式脚本 Provider 的真实工具续轮生成结构化最终回复，再从 Web 交互和 SQLite 读回证明接纳与恢复。脚本 Provider 仅证明软件协议，不证明真实模型会选择合适组件或理解表格内容。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 覆盖用途与限制 |
| --- | --- |
| [tests/system/verify_channels.py](../../tests/system/verify_channels.py) | `--rich-replies-only` 验证生成至提交闭环；`--ui-components-only` 同时覆盖既有渲染降级、普通草稿和本场景；`channels` core gate 包含本场景 |
| [tests/system/channels_browser.mjs](../../tests/system/channels_browser.mjs) | 实际浏览器执行填表、Unicode 越界、失败/不确定提交、同消息另一表单草稿、重载、原生无脚本提交和移动端布局 |
| [internal/delivery/ui/compose_test.go](../../internal/delivery/ui/compose_test.go) | 生成映射、恶意组件、用户文本保持原样、稳定且独立的表单身份、字段顺序及恢复 |
| [internal/interaction/web/ui_forms_test.go](../../internal/interaction/web/ui_forms_test.go) | 现有认证、CSRF、撤销绑定、原生 POST、重复身份和额外动作字段 |
| [internal/agent/sqlite/channels_test.go](../../internal/agent/sqlite/channels_test.go) | 真实接纳记录的来源读取与绑定隔离，既有并发输入和审批去重 |

既有 `native_components` 的直接投影夹具只证明渲染与降级，不作为生成端完成的依据。真实账户、模型组件选择和语义质量仍需独立评测；不以合成数据或脚本 Provider 宣称通过。

相关验证入口：

```sh
go test ./internal/delivery/ui ./internal/interaction/web ./internal/tui
go test -race ./internal/agent/sqlite -run 'TestChannelSourceContent|TestChannelConcurrentCallback'
python3 tests/system/verify_channels.py --binary bin/steward --rich-replies-only --evidence evidence/my-rich-reply/report.json
make test lint
make test-e2e E2E_EVIDENCE=evidence/my-core-run
```

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
