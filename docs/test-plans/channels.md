# 终端、Web 与飞书：测试方案

[功能与验收目标](../features/channels.md) · [技术方案](../designs/channels.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-017](../features/channels.md#f-017) | 验证真实终端首条输入、后台工作时追加、退出和重启。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-018](../features/channels.md#f-018) | 真实浏览器验证设置、发消息、附件、审批和已有对话展示。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-019](../features/channels.md#f-019) | 长任务与输入草稿并行时，回复/进度更新可见，焦点、草稿及附件不被覆盖。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |
| [F-020](../features/channels.md#f-020) | 验证真实绑定、token 生命周期、通知偏好、撤销、重启与不确定投递；未知结果不盲目重发。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |

## 层级与关键路径

实际浏览器/PTY/签名回调分别驱动；输入草稿、附件与焦点必须在后台更新时保留。飞书失效 token、撤销绑定和发送结果未知单独验证；本地回执不冒充远端核实。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_channels.py](../../tests/system/verify_channels.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_tui.py](../../tests/system/verify_tui.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_delivery.py](../../tests/system/verify_delivery.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

- **F-019**：通过具名投影与真实浏览器覆盖；不以软件回归证明真实模型质量或外部账户可用性。
- **F-020 剩余覆盖**：补凭据刷新/失效诊断体验；不确定远端投递目前只能依赖已保存本地回执，可信远端核实能力仍待定义。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="channels-verification-and-evidence"></a>

### Verification and evidence

```sh
make test
make test-race
make lint
make test-full
python3 tests/system/verify_channels.py --binary bin/collie --evidence evidence/channels.json
python3 tests/system/verify_channels.py --binary bin/collie --evidence evidence/channel-leases.json --delivery-lease-only
python3 tests/system/verify_channels.py --binary bin/collie --evidence evidence/components.json --ui-components-only
```

`verify_channels.py` owns Launch (private root, local platform Fake and real Web
process), Doctor (auth and real browser setup), Drive (HTTP/UI/CLI boundaries),
Evidence (SQLite/Conversation/API/platform ledgers and browser screenshots), and
Cleanup (joined children, stopped Fake server and removed disposable state).
CI selection follows the current engineering design; repetitions require an explicit runner option.
The pinned Browser dependency is the repository's existing `playwright-core`.
Screenshots and DOM evidence are under `evidence/channels-ui/`.

The harness covers concurrent callback/source retries, identity/auth/recipient
rejection, input commit and delivery SIGKILL windows, approval Steering plus
restart and callback races, inbound/outbound attachment bytes, unknown-send
non-retry, and GC-08/09/12/14 channel/state fragments. It also covers
slow sends/uploads, cumulative multipart duration beyond the default lease,
competing live/recovery consumers, lost ownership cancellation, late settlement
and renewed ingress after restart. The existing platform Fake owns deterministic
response delays and an independent send-attempt ledger. Unit/contract tests cover
migration rollback, CAS/revocation, route/projection freezing, encryption,
credential redirect refusal, session expiry, XSS and unknown/invalid-component
fallback. The channel harness also drives Web account login through the real Manager/Bridge
against the existing local OAuth Fake, actual Probe/Select, hidden-model rejection,
restart and logout, then scans persisted files for credential values. Existing
suites retain replay, cancellation, account lifecycle and
Artifact access coverage.

Scripted-policy transport fragments are not full product scores. Complete 20-case
semantic grading and live Feishu/ChatGPT authorization remain release evidence;
this slice does not claim M7 or the complete first release is finished. No live
external account or write is required by CI.

Feishu transport shapes follow the official [message API](https://open.feishu.cn/document/server-docs/im-v1/message/create),
[resource API](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1),
and [callback encryption guide](https://open.feishu.cn/document/server-docs/event-subscription-guide/event-subscription-configure-/encrypt-key-encryption-configuration-case).

<a id="delivery-inspectable-verification"></a>

### Inspectable verification

The system runner selects `tests/system/verify_delivery.py` for delivery regression. It reuses
`MainHarness` and `Terminal` for Launch, Doctor, Drive, Evidence and Cleanup.
Each run captures 20 actual SIGKILL/restart cases: Main and Root final replies at
Conversation append, delivery commit, checkpoint, send and acknowledgement
boundaries. Root acceptance is delivered before faulting its final Main follow-up.
Database snapshots prove one semantic message/receipt, correct destination,
retired outbox, reused generations, and read-only duplicate scans. PTY transcripts
prove final replies and frozen-hash approvals reach the real terminal surface.

The explicit `-test-delivery` Fake requires `-test-bridge` and supports
`retry-once`, `unavailable`, `ambiguous`, and `unknown`. It wraps the same durable
terminal acceptance boundary. Recovery evidence covers bounded retries, query-only
unknown, accepted-send reconciliation, and duplicate approval callbacks without
unnecessary Main inference. Go tests additionally cover concurrent consumers,
claim fencing, identity drift, atomic event failures, legal/illegal transitions,
and populated v11 migration. These are M3 conformance fragments, not full product
Golden Case scores or a claim that M5 Durable Replay is complete.

<a id="web-components-build-and-verify"></a>

### Build and verify

```sh
npm ci --prefix web --ignore-scripts
npm run build --prefix web
make build
npm run check --prefix web
npm run lint --prefix web
python3 tests/system/verify_channels.py --binary bin/collie --evidence evidence/components.json --ui-components-only
```

Commit component source, lockfile and generated assets together. `make test`
rejects stale embedded assets; CI installs the same pinned dependencies. Go-only
builds consume those checked-in assets and need no frontend server or network.
The maintained channel browser action proves actual React composition, native
table cells and fallback, frozen approval fields, artwork loading, responsive
layout, CSP-compatible execution and local-only frontend requests. Its complete
action also drives model setup, file upload and actual message submission.
It deliberately holds the local JavaScript bundle while entering a draft and
selecting an attachment through the usable server form, then releases it and
checks text, file bytes, focus, selection and source identity before submitting.
The composer retains the native form and input nodes and only applies shared
control styles. The existing image scenario also holds the bundle while opening
a native picker, completes selection after enhancement, then verifies scoped
pixels at the actual Pi HTTP peer; text-only and recovery checks still apply.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。

## 工程基础重构回归

F-018/F-039：`foundation_test.go` 实际 HTTP listener 在 Main provider failure 后仍服务并执行下一请求；`web/page_test.go` 证明 Settings/Tasks 不依赖历史查询；`conversation/sqlite/store_test.go` 验证 205 条记录无重叠/遗漏分页和作用域查询。`tests/system/verify_channels.py` 通过实际 Web 证明缺失依赖不隐藏控制页、修复后恢复 ingress、GET 不修改已稳定业务状态。

## F-019 实时更新验收

- `internal/interaction/web/updates_test.go`：条件 ETag、消息/审批/状态变化、历史游标、HTML 转义、无输入/内部字段、失败无部分响应；条件请求也必须重新认证和检查绑定；Settings/Tasks 不查询 Conversation。
- `tests/system/verify_channels.py` 调用维护中的 `channels_browser.mjs` live 场景：在真实 Actor/SQLite/HTTP/浏览器链路中，输入草稿和选择文件后收到新回复，保留原输入节点、request source ID、光标/选区、焦点和阅读锚点；断网后重连；Task/Worker 产生冻结审批与 busy 状态，拒绝动作不重载、不执行副作用；另一页显示任务取消；会话轮换后保留草稿并显式恢复。
- 原有 setup/components/approval/移动端/无 JavaScript 场景继续验证渐进增强、受控投影和原表单兼容；整个渠道回归保留签名/身份/去重、未知副作用和重启场景。

不增加 golden case、模型效果分数或独立 CI 编排。完整检查使用既有 `make test lint` 与 channels/core runner；执行结果保留在既有 evidence 报告中。

## Web 信息架构与流程回归（F-018/F-019）

沿用 `verify_channels.py` 和真实浏览器 action，不新增脚本或 CI gate：

- 原生设置：模型来源 → 保存 → Verify → 明确授权 Select → Conversation；候选 probe 与活动状态分开；ChatGPT 登录、可见目录、重启和退出仍经真实 Manager/Bridge（外部 OAuth 为受控 peer）。通知保存后停留原页，重载仍选实际目的地。
- 原生输入：延迟脚本时编辑、选文件与打开 picker，增强后保持同一节点/文件；拒绝或丢失确认不自动重发并保留草稿和附件；任务上下文可移除而不丢输入。
- Tasks：列表筛选、分页、详情目标/阶段、对应的冻结审批、已由 Main 交付的结果，以及带任务上下文回到 Main；不能用 Worker 结果替代 Main 交付。详情和 Conversation 保留同一审批 hash。
- Developer：列表打开独立 Trace 详情，树与时间线对齐；节点类型决定可用 Tab；完整 Context/JSON/来源/长正文和 Diff 深链接可达，原生链接、移动端与无 JavaScript 路径继续可用。
- 读取边界：真实 SQLite 验证跨 20 条限制、同时间稳定游标、scope 拒绝、归属校验与只读池；Web 审批过期投影不写库/事件，原运行时恢复仍落库。Trace 详情不能依赖全局列表成功。
- 模块测试：notification 持久化选择、候选 proof version、登录本地返回、Host 审批单卡片/用户文本不改写、任务消息通过既有 ingress；`make test lint` 检查所有包及嵌入资源一致。

系统断言采用受控模型/HTTP 响应，证明产品链路与回归，不证明真实模型质量或真实账户可用性。执行结果与截图留在本次 evidence，不把测试计划写成执行报告。
