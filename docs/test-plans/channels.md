# 终端、Web 与飞书：测试方案

[功能与验收目标](../features/channels.md) · [技术方案](../designs/channels.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导特定版本已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-017](../features/channels.md#f-017) | 验证真实终端首条输入、后台工作时追加、退出和重启。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-018](../features/channels.md#f-018) | 真实浏览器验证设置、发消息、附件、审批和已有对话展示。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-018](../features/channels.md#f-018) | 真实浏览器组合待回答/审批/审阅/故障与普通工作，验证顺序、计数到明细、权限、重复/已解决/过期事项；后台更新、断线与旧页面下保护草稿/附件/焦点/位置。沿用现有渠道浏览器入口；F-004 未完成前仅能验证已有事项，不能据渲染器宣称闭环完成。 | 未实现；目标入口回归及相应 Live/效果评测待补 |
| [F-019](../features/channels.md#f-019) | 长任务与输入草稿并行时，回复/进度更新可见，焦点、草稿及附件不被覆盖。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |
| [F-020](../features/channels.md#f-020) | 验证真实绑定、token 生命周期、通知偏好、撤销、重启与不确定投递；未知结果不盲目重发。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |
| [F-020](../features/channels.md#f-020) | 签名回调→Actor→原消息同步/Task 异步交付→受控飞书 HTTP peer；验证精确原来源、单 Main 分组、可信全文/文件链接、通知偏好和完整语义，覆盖重复/重启/撤权/发送未知。 | 个人 p2p 入口已有回归；显式 GitHub PR 接续由 F-042/F-004 驱动，并经真实 Feishu HTTP peer 验证原话题，真实飞书展示、链接可达性与语义质量待 Live/效果验收 |

## 层级与关键路径

实际浏览器/PTY/签名回调分别驱动；输入草稿、附件与焦点必须在后台更新时保留。飞书失效 token、撤销绑定和发送结果未知单独验证；本地回执不冒充远端核实。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 覆盖用途与限制 |
| --- | --- |
| [tests/system/verify_channels.py](../../tests/system/verify_channels.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 |
| [tests/system/verify_tui.py](../../tests/system/verify_tui.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 |
| [tests/system/verify_delivery.py](../../tests/system/verify_delivery.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 |

- **F-019**：通过具名投影与真实浏览器覆盖；不以软件回归证明真实模型质量或外部账户可用性。
- **F-020 剩余覆盖**：显式 GitHub PR 以外的跨平台事件接续与成果组织；真实账户的原消息回复/卡片展示、工作台链接可达性和客户端登录。loopback 只能证明同机访问。不确定投递继续只查询本地回执，可信远端核实能力仍待定义。

原消息与成果投影回归入口：

- [reply_origin_test.go](../../internal/agent/sqlite/reply_origin_test.go)：真实 SQLite 与 Actor 验证 create 来源、同步/Task 结果、显式偏好、旧路由不回填及失败迁移回滚；既有 Channel 回归继续覆盖 CAS、撤权、route/projection 冻结和回调去重。
- [projection_test.go](../../internal/interaction/feishu/projection_test.go)：真实 HTTP reply wire、原来源转义、无自选接收者、目标失效无 fallback、可信 origin 校验、摘要边界和审批全文。
- `verify_channels.py` 的 `feishu_topic_delivery`：签名 callback→Main→Task→冻结 Delivery→HTTP peer，检查原消息回传、Conversation 分页锚点、完整 Artifact、单一简短通知、重启/重复回调、显式 TUI 偏好与 unknown 不重发。`feishu_auth_lifecycle` 和现有 multipart/租约场景同时覆盖 source reply；无 workspace 配置保留原全文/附件。

聚焦验证可复用已有函数：`PYTHONPATH=tests/system python3 -c 'from pathlib import Path; from verify_channels import feishu_topic_delivery; feishu_topic_delivery(Path("bin/steward").resolve(), {"checks": [], "scenarios": {}})'`。跨模块变更仍运行 `make test lint` 和相关 channels 系统门禁。合成模型证明机械链路，不证明摘要的真实模型质量或远端平台可达性。

## 详细测试入口与证明边界

本功能协议在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="channels-verification-and-evidence"></a>

### Verification and evidence

```sh
make test
make test-race
make lint
make test-full
python3 tests/system/verify_channels.py --binary bin/steward --evidence evidence/channels.json
python3 tests/system/verify_channels.py --binary bin/steward --evidence evidence/channel-leases.json --delivery-lease-only
python3 tests/system/verify_channels.py --binary bin/steward --evidence evidence/components.json --ui-components-only
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
python3 tests/system/verify_channels.py --binary bin/steward --evidence evidence/components.json --ui-components-only
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

系统断言采用受控模型/HTTP 响应，证明产品链路与回归，不证明真实模型质量或真实账户可用性。执行结果与截图留在对应运行的 evidence，不把测试计划写成执行报告。


## F-020 自建应用凭据与失效诊断

- `internal/interaction/feishu/auth_test.go`：真实loopback HTTP验证并发单飞、提前到期刷新、旧响应不能失效新token、取消等待者、临时刷新失败与恢复、过期token不回退、关闭中迟到结果、异常TTL/缺失字段、密钥不回显、重定向不泄密、仅只读探测可重查。
- `internal/config/channels_test.go` 与既有CLI语法回归：凭据模式二选一、SecretStore引用、受限端点与 `channel status/check`。
- 既有 `verify_channels.py` 的 `feishu_auth_lifecycle`：无需浏览器，以真实CLI/Actor/Delivery和受控飞书HTTP peer验证配置检查、token拒绝后的持久安全重试、过期主动刷新、无效凭据的安全诊断与持久失败码、撤销绑定、凭据不落盘，以及已发生副作用但回执丢失后重启仍不重发。该场景接入现有channels门禁，不新增脚本或CI阶段。
- 真实飞书自建应用、实际账户权限和公网回调配置需独立Live验证；软件peer不证明真实平台授权、模型效果或永久 exactly-once。

Steward 重构检查补充：Chat 的 Computer/Worker 只读详情与真实执行状态一致；明暗切换、头像暂停、移动抽屉与键盘焦点恢复；发送、附件、审批冻结哈希、旧消息定位和 live 更新沿用本页原有场景。

界面语言的默认、持久化、认证/CSRF、实时片段及冻结审批内容回归由 `internal/interaction/web/locale_test.go` 覆盖；中英文与移动浏览器检查见[品牌测试方案](branding.md)。


工作区精修：updates_test 验证待审批计数包含内嵌卡片且不重复、解决后清空；computer_test 验证 status/frame 的认证、绑定撤销、身份裁剪与 no-store。渠道浏览器继续覆盖真实草稿、附件、审批、背景更新和最新消息入口；Task 读取维持独立于 Conversation，Worker 采用有界 Inspector 结果。执行报告保存在 evidence，不据软件 peer 宣称真实 Guest 画面兼容性。

Chat / Worker：`workers_test.go` 验证认证与绑定撤销、只读字段和面板仅属于 Chat；`observability/sqlite/inspection_test.go` 验证真实只读数据库中的运行时间排序、50 条上限、运行中查询和 Agent 隔离。`channels_browser.mjs` 的 live 场景通过真实 Web 验证 Computer/Worker 互斥、焦点返回和草稿/附件节点保留。

本机免 token：`server_test.go` 通过真实 loopback HTTP 验证无 SecretStore 时直接进入页面及只读 API、同源设置表单、绑定撤销；拒绝远端 peer、跨站／同站不同 origin、伪造 Host 与转发身份。显式 token 模式的原有认证、CSRF、session 过期回归继续执行；配置检查覆盖可省略 token 与非法引用。

待确认入口沿用 live 浏览器场景：真实审批到来时，入口按 Computer → Worker → 待确认排列，点击聚焦原审批；冻结 hash、未发草稿和附件仍保留，解决后入口消失。顶栏预留三行，计数变化不改变对话偏移。

布局与焦点回归沿用同一个 `channels_browser.mjs`：inspection 在 851／900px 断言详情完整堆叠，Enter 选择 Generation 后焦点仍位于对应树行；live 在 820／900px 验证任务正文与进度值没有压缩为零宽；共用移动检查通过键盘打开／关闭导航、320px 当前设置标签露出及 1001px Computer modal。原有草稿、附件、审批、滚动、会话恢复与无 JavaScript 检查继续执行。人工补充检查明暗主题、模型／通知／六类能力详情、长内容和宽屏恢复；模拟视口不等同于物理触摸或屏幕阅读器验证。
