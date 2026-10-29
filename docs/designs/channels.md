# 终端、Web 与飞书：技术方案

[功能与需求](../features/channels.md) · [测试方案](../test-plans/channels.md) · [主索引](../feature-map.md)

最近核对：2026-10-29。本文的架构与流程描述当前有效实现；明确标记的待实现方案不表示已落地。实现级协议仍服从 [跨功能协议与约束](product-overview.md#contract-0-文档效力与实现规则)。

## 当前架构

渠道将输入归一化为 InboundEvent，经身份、来源去重进入同一 Actor。Main 的语义消息与冻结路由产生 DeliveryRequest，各适配器记录投递回执。

```mermaid
flowchart TD
    T["TUI"] --> I["InboundEvent / 身份绑定"]
    W["Web"] --> I
    F["Feishu"] --> I
    I --> A["共享 Actor / Main"]
    A --> D["Conversation + 冻结 DeliveryRequest"]
    D --> O["渠道适配器 / 回执"]
```

### 控制面与查询边界（F-039）

Web 在依赖准备期间开放，ingress 保持关闭；缺失能力修复后后台重试准备，成功后恢复接纳。TUI 在依赖恢复后刷新就绪提示并重新调度已接收工作；恢复提示与已有历史展示可独立到达。已持久化终态的 Main 模型失败由 Run 记录承载，不关闭 HTTP listener；未能确立权威状态的基础设施错误仍使执行循环明确失败。

页面查询按用途分开：Models 读取模型与必要账户信息，Notifications 只读渠道设置，Tasks 通过独立只读池读取分页摘要/详情，Conversation 读取最近 100 条和审批。`before` 是排他的 sequence 游标；返回升序消息，页面提供 Earlier messages，API 通过 `X-Collie-Before` 返回下一页 cursor。GET 不 Flush intent、不写 projection；交付循环独立 Flush 并冻结投影。已有投影只读复用，无投影时内存合成。交付直接按 conversation/message ID 取一条，Main 初始化按冻结 ID 集合分批读取，不扫描不相关记录，也不改变已有上下文边界。

### Go 实现职责（F-040）

Web POST 分发只选择有名动作处理函数；消息/附件、模型、账户、通知各自维护请求步骤，认证、CSRF 和 Server 生命周期仍为统一入口。

### Web 信息架构与呈现职责（F-018）

侧栏只有 Conversation、Tasks、Settings 和次级 Developer 入口；没有重复头像入口、静态绿色状态灯或未实现功能的空 Tab。共同的导航、面包屑、状态、按钮、面板和反馈使用一套中性样式。视觉主操作分别是发送、处理当前任务、配置下一步和选择观测节点。

`routes.go` 维护认证后的路由分发，`navigation.go` 维护规范路径、旧地址跳转和本地返回地址校验；`page.go` 只组装共享框架与分派加载。`settings.go`、`tasks.go`、`conversation.go` 与 `approval_view.go` 拥有各自的查询/呈现。前端样式按 shell、workspace、conversation、observability 分开，共享设计变量；没有引入新框架或第二套状态管理。

Models 概览 → 来源选择 → 新模型表单 → 模型详情验证/选择；Notifications 单独子页。候选模型的 probe 成功不等于 profile 已激活：呈现检查已存 Probe 的 profile/auth version、BridgeIdentity 和能力，真正选择仍由原 Control.Select 校验权限、实时凭据和版本。表单失败留在对应流程，凭据和底层错误不反射到页面。选中模型只影响新 Run，已有冻结执行树保持原配置。通知选项绑定实际持久化值，不可用的旧绑定明确提示。

Tasks 的分页、详情、历史与 Main 交付由 `task/sqlite.Reader` 提供；共享同一进度定义的 TUI/旧 API 保持原摘要契约，见 [任务设计](tasks.md)。详情中的继续沟通先校验所有权，再作为带可见任务说明的普通用户消息进入现有 Main ingress；不增加 Task 控制 API。

Web 审批查询使用只读 `Runs.ApprovalView`，已过期授权按当前时间投影状态，不执行恢复、落库或唤醒；CLI/运行时仍调用原 `Runs.Approvals` 恢复。卡片只识别 security 来源且身份匹配的 Host 通知，不按正则删改用户文本，也不改写持久化 Conversation/投递投影；精确参数、完整 Raw action 和冻结哈希保留。

旧 `/settings` → Models、`/progress` → Tasks；旧 Trace query/冻结上下文链接保留全部选择参数并跳转到规范详情路径。已知子页未登录返回 401 登录表单，成功后仅返回校验过的本地页面；API 认证响应不变。共享认证、CSRF、binding、CSP 和 no-store 继续先于业务处理，无 schema 迁移。

### Web 实时投影与界面生命周期（F-019）

`GET /api/updates?view=/|/tasks|/tasks/{taskRef}|/settings/...&before=...` 返回当前视图的具名 HTML 区域与 `latest` sequence；`before` 仅用于 Conversation 的排他历史游标。状态与内容由现有 Core 查询取得，复用 SSR 的 UISpec 验证、转义、语义 fallback 和审批投影。完整页面与后台读取共用 `loadConversation` 和具名模板；没有新 Agent Loop、模型请求、领域写入口、事件表或迁移。

每次请求先执行已有认证与渠道绑定检查，再判断 `If-None-Match`；未变化返回 304。响应不含 composer、source_id、原始 Run/Tool trace 或模型凭据。历史页保留原游标，同时读取最新头部 sequence 提示新消息，不悄悄跳回最新窗口。不同区域读取的是各自已提交投影，不宣称跨数据库的原子快照；下次读取收敛，审批提交始终重新校验冻结哈希与权限。

浏览器每秒读取一次，单一调度链与请求取消防止过时响应覆盖新视图；单次 10 秒 deadline，网络失败最多退避到 30 秒。隐藏/离开页面时取消读取，返回时立即刷新。401/403 停止轮询并保留当前页面，另页登录和显式重试不会重建输入节点。设置表单不参与后台替换，避免覆盖用户填写或悄悄更换模型。

前端职责拆分为：`main.tsx` 仅启动；`composer.ts` 增强原生表单、附件状态和单次发送，不替换 DOM；`projections.tsx` 负责受控 UISpec 和 React root 生命周期；`live-updates.ts` 负责读取、区域协调、阅读位置与连接状态。按稳定消息 sequence/TaskRef/审批 ID 复用不变的组件节点，只释放被移除的 React root。文件选择器不序列化到网络或本地存储，也不随刷新重挂载。读者锚点移出最新 100 条窗口时保留当前内容，用户点击新消息提示后才切换。

审批仍使用 `POST /approval`；渐进增强请求通过 `Accept: application/json` 获取接受回执，原 HTML 表单继续使用 303。前端只提交用户所点表单的 ID、原冻结哈希和决定，每次交互发送一次。确认丢失或旧审批失效时读取当前投影，不自动重发决定、不把旧同意转给新审批，也不把回执当成外部动作完成。

选择条件读取而非将 NATS/Run trace 接入浏览器：复用现有权威查询和认证，避免为单用户工作台建立另一条订阅、事件重放或执行管线。代价是可见页面存在周期性查询；条件响应、隐藏暂停和退避控制其成本。已接受 Run、Capability/Model Snapshot、来源隐私和持久化格式保持原样。

## 当前流程与目标差异

验证身份 → 规范输入与附件 → 接受并去重 → Actor 执行 → 冻结回复目标 → 适配呈现并发送。Web 通过带 ETag 的条件读取更新独立页面区域，输入状态保留在原节点。

- **F-017**：标准启动/TUI/CLI、输入与已交付回复投影已接通。 终端输出没有远端确认协议，不宣称屏幕显示 exactly-once。
- **F-018**：分层 Models/Notifications、Tasks 列表/详情和独立 Trace 详情已接通，保留认证路由、前端嵌入与语义 fallback。 实时更新与开发区分别由 F-019、F-031 跟踪。
- **F-019**：已提交会话、审批、全局状态、任务摘要和登录过程通过条件读取实时刷新；草稿、文件、焦点、选区和阅读位置独立保留。传输与模型 token streaming 分开；阶段摘要生产仍由 F-005 负责。
- **F-020**：签名回调、p2p 身份绑定、收发和本地回执已实现；access token 由外部维护。 补凭据刷新/失效诊断体验；不确定远端投递目前只能依赖已保存本地回执，可信远端核实能力仍待定义。

## 代码与依赖

- [internal/tui/session.go](../../internal/tui/session.go)
- [internal/interaction/web/server.go](../../internal/interaction/web/server.go)
- [internal/interaction/web/lifecycle.go](../../internal/interaction/web/lifecycle.go)
- [web/src/main.tsx](../../web/src/main.tsx)
- [internal/interaction/feishu/handler.go](../../internal/interaction/feishu/handler.go)
- [internal/interaction/feishu/delivery.go](../../internal/interaction/feishu/delivery.go)
- [internal/app/channels.go](../../internal/app/channels.go)

## 验证与交付

验收场景、已有入口和剩余覆盖统一在 [测试方案](../test-plans/channels.md) 维护。本次建档未执行本功能的回归或 Live 验证，不以架构图替代运行证据。

## 协议、实现与配置约束

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="channels-deduplication-restart-and-uncertainty"></a>

### Deduplication, restart and uncertainty

Schema 025 adds bindings, event/source aliases, frozen message routes, preferences,
UI projections, imported-resource mappings and per-part channel receipts.
Migration is forward-only, transactional, preserves existing TUI receipts and
runs from empty or every prior supported schema.

The input transaction checks the enabled binding, canonical semantic hash and
both callback and source identities, then commits the existing `main_input`,
Conversation intent, route and Outbox event together. A new callback ID carrying
an already accepted source ID reuses its receipt. Identity or parameter drift
fails closed. Web/TUI/Feishu inputs preserve the authoritative Conversation order.

The Composer persists UISpec v1 in Delivery, never semantic Conversation or model
context. Fixed templates support escaped semantic Markdown text, tables, Artifact
cards, progress, frozen confirmation cards and field summaries. They do not execute
HTML/JS/React. Unknown versions, unknown components and invalid/render-failed
components degrade to semantic text. TUI remains semantic text/media fallback;
Feishu uses a controlled plain-text card and uploaded file attachments. Each
outbound file retains its name and exact bytes. Missing/oversized resources fail
closed rather than exposing a host path or silently sending the wrong attachment.

The Web shell uses the Collie personal-agent layout, locally bundled
ad-agent Button/Card components and shared native-control styles, OpenIntelligentUI's native DataTable,
and the owner-selected Collie border collie logo/default agent avatar. The React layer only
enhances fixed UISpec v1 components; it does not add a second
Agent or model connection. Server rendering remains the semantic fallback.
The composer keeps its server-rendered form and input nodes throughout enhancement.
Shared control styles apply without replacing text, focus, source identity, selected
files or an open native picker. Delayed-bundle browser regressions cover both a
selected attachment and a picker whose selection finishes after the bundle loads.
No frontend asset requires an external request. Source revisions, local changes,
MIT/Apache-2.0 notices, build and verification commands are recorded in
`web/README.md`; bundled notices are at `/licenses.txt`.
OpenIntelligentUI's arbitrary generated HTML/JS path is outside this integration
and requires an explicit architecture/security decision under the baseline.

Local Web receipt acceptance is durable and idempotent. Feishu UUID deduplication
has a bounded provider window, so the adapter does **not** promise indefinite
idempotence. Successful parts retain authoritative response IDs before Outbox
acknowledgement. Restart reuses those receipts. A disconnected send without an
authoritative receipt becomes `unknown` and is query-only; it is never blindly
resent. Interrupted uploads can leave an unreferenced provider file, but cannot
justify repeating a user notification. The user can see the semantic message in
Web history while a remote transport remains unconfirmed.

The shared Delivery service renews its fenced claim through composition, receipt
queries, every upload/message part and settlement. A slow successful request or
cumulative multipart operation may exceed the configured lease without becoming
a second attempt. Renewal checks the current token, version and live expiry;
an expired or replaced owner cannot renew. Lost ownership cancels that transport
and joins its heartbeat without stopping Web ingress. The current owner recovers
from authoritative receipts; an uncertain Feishu send stays query-only.
The service revalidates the live fence before starting transport, so a scheduling
pause that consumes the original lease cannot authorize even a fast send.

<a id="delivery-recoverable-semantic-delivery-collie-15"></a>

## Recoverable semantic delivery (retired task #15)

Main replies reach the local terminal through a durable Delivery Outbox. The
Conversation stores semantic content; `state.db` stores delivery intent,
transport state, and the terminal's immutable accepted-message projection.
No online accounts, broker, or model inference are needed to drain accepted
deliveries. [Web/Feishu adapters and UISpec composition](../features/channels.md) extend
this delivery path and have separate transport/UI verification.

```mermaid
sequenceDiagram
    participant Main
    participant Conversation as conversation.db
    participant State as state.db
    participant Terminal as terminal projection / TUI
    Main->>State: Frozen reply intent (reply:exchange ID)
    Main->>Conversation: Idempotent Append
    Main->>State: DeliveryRequest + delivery_outbox + domain event
    Main->>State: Advance reply checkpoint
    State->>Terminal: Compose and send with frozen delivery ID/key
    Terminal-->>State: Accepted receipt (or query after interruption)
    State->>State: sent + remove delivery_outbox + domain event
    Terminal->>Terminal: Render retained semantic history
```

There is no distributed transaction. A restart reuses the accepted generation,
reply intent and IDs, appends missing semantic messages, enqueues missing delivery
requests, then advances the checkpoint. Sending never rewrites Conversation.
Duplicate ingress callbacks keep the original Main receipt. Repeated Reporter
events keep the original Main follow-up. Repeated enqueue checks all frozen
identity fields and fails closed on drift.

The producer owns the private enqueue boundary (`delivery/sqlite.Enqueue` inside
its transaction); `delivery.Service` owns transport and reconciliation; adapters
own presentation and delivery receipts. Tools and Workers do not select a
channel or send completion messages. The composer accepts assistant semantic
parts, including opaque Artifact references, and rejects Tool traces. No arbitrary
HTML or executable UI enters the protocol.

| State | Meaning | Next state |
| --- | --- | --- |
| pending | Durable intent, no dispatch claim | sending |
| sending | Leased attempt; a crashed sender may have sent | sent, retry, failed, unknown; expired lease may be reclaimed |
| retry | Known transient failure with no effect; persisted wake time | sending |
| sent | Authoritative accepted receipt | Terminal |
| failed | Definitive rejection or exhausted known-safe retries | Terminal |
| unknown | Outcome cannot be established | Query only; sent/failed on authoritative evidence |

Each claim has a fresh Host token and expiry, plus a version checked by CAS.
A late acknowledgement cannot
change a newer owner's row or a terminal receipt. Delivery state and its internal
CloudEvent commit together. `delivery_outbox` owns transport work and is distinct
from `event_outbox`, which propagates internal domain events.

An adapter declares whether the provider guarantees idempotency for the frozen
action/key. Recovery queries a prior sending attempt first. An authoritative
absence permits another send only through an idempotent adapter: the expired
sender may still be alive. A provider without idempotency or a reliable query
remains unknown. An unknown row is never automatically resent, even when a later
query reports absence. Raw provider errors are replaced with bounded codes.

The local terminal adapter implements acceptance as an immutable unique-key
insert into `terminal_message` in `state.db`, with an internal acceptance event.
Acceptance and acknowledgement use separate transactions. The TUI reads that
retained projection rather than displaying pending assistant intents. Physical stdout has no acknowledgement
protocol: reconnect and `/history` can show accepted history again. This is one
stored semantic message, not generic exactly-once rendering or external delivery.

Only completion, failure, needs-input and approval intents can enqueue. Progress
updates remain read-only activity and create no notification or Main inference.
Root completion/failure goes through Reporter and Main. Security originates
approval notices in the invocation transaction, then the normal Conversation-first
recovery path delivers them. `/approve ID HASH` and `/deny ID HASH` submit the
displayed frozen hash to Security and wake the original Run. Duplicate matching
decisions remain idempotent. Approval expiry/invalidation still fails closed.

<a id="web-components-collie-web-components"></a>

## Collie Web components

This is a locally bundled presentation layer for the existing Go Web server.
The conversation layout uses a warm neutral canvas, soft selected navigation,
a tennis-green submit action, centered thread and rounded composer. React enhances the escaped server HTML;
the existing forms and semantic fallback remain usable without JavaScript.

<a id="web-components-pinned-source-and-changes"></a>

### Pinned source and changes

| Source | Revision | Reused code or asset | Local changes |
| --- | --- | --- | --- |
| [ad-agent](https://github.com/z-chenhao/ad-agent) | `e43216ffa7bd686b496a15f1d8a99ac180857caa` | `web/src/components/ui/{button,card}.tsx`, `web/src/components/ui/styles.ts`, `web/src/lib/utils.ts`, neutral style tokens | Retain used card components, button variants/sizes and style tokens; Collie supplies layout, routes and content |
| [OpenIntelligentUI](https://github.com/CopilotKit/OpenIntelligentUI) | `8391a94c1c3939c679e40ddd7bfc08a9ef690e70` | `apps/app/src/components/generative-ui/table-catalog.tsx` native `DataTable` and Zod props | Remove the CopilotKit catalog registration/import; retain validation and accessible table; map the existing UISpec table through a private adapter |
| Collie brand | Owner-selected 2026-10-29 generation | `assets/brand/collie.png` | Unmodified original image, displayed at different sizes by CSS |
| [Google Fonts](https://github.com/google/fonts) | Included OFL notices | Lora and DM Sans variable fonts | Unmodified, self-hosted font files; no CDN |

Upstream licenses and artwork provenance are in `licenses/`. The binary embeds
the required MIT/Apache-2.0 license texts and serves them at `/licenses.txt`.
There are no paid frontend services. Pinned runtime dependencies are React,
React DOM, Zod and the ad-agent component helpers. Tailwind and esbuild compile
checked-in local assets; they do not run as product services.

OpenIntelligentUI's native component is integrated here. Its Python agent,
CopilotKit/AG-UI transport, model routing, browser-key flow and generated
HTML/JavaScript sandbox are not integrated. Collie's frozen ModelSnapshot,
SecretStore, Actor, approval and controlled UISpec v1 contracts remain the owners
of those responsibilities. Enabling arbitrary generated code requires a separate
architecture/security decision.

The client accepts only supported UISpec v1 components. Invalid/unknown versions,
components and unsupported table dimensions retain the server-rendered result.
Empty or failed renders use authoritative semantic text from the Host, never a
replacement fallback supplied inside a projection. There is no HTML evaluator,
dynamic component import, remote asset or browser credential input.

## Changelog

2026-10-29：F-018/F-019 按用户任务重构整个 Web 信息架构。比较继续在 Settings/Trace 单页堆叠控件与按流程拆页，选择稳定子路由与共享 SSR 呈现；完整 Context 并入 Generation，保留深链接。独立 task 只读查询、纯 Web 审批读取和原生表单生命周期减少界面与执行的耦合；不新增控制管线、Task 状态、依赖、脚本、CI gate 或数据迁移。旧 HTML 地址仅跳转，原 API 与冻结执行语义不变。


2026-10-29：输入区保留原生表单与文件选择器 DOM，只复用共享控件样式；修复脚本加载期间打开文件选择器后附件丢失的问题。真实浏览器延迟 bundle 后跨加载完成图片选择并核对实际 Pi HTTP pixels。

2026-10-28：测试入口随当前 Makefile/runner 对齐。旧说明中的 make verify、Delivery/security 分片与自动五轮是历史调度；当前默认单次，重复次数通过 runner --repeat 显式指定。保留机制验收要求。

2026-10-28：按统一文档结构迁入本功能的协议与历史决策，更新引用并移除旧文件；此为文档归属调整，不改变运行时协议。

本节追加记录每次架构/流程取舍。旧图不保留在正文；历史正式决定保留在本节。首次记录为现状建档，不推断历史决策日期。

| 日期 | 类型 | 决策与理由 | 取舍及影响 | 关联 |
| --- | --- | --- | --- | --- |
| 2026-10-28 | F-040：按职责拆分 Go 实现。 | 保持原协议、路由、schema 与原子事务边界；详见工程方案。 |
| 2026-10-28 | F-039：Web 故障隔离与分页纯查询；交付按 ID 定位。 | 控制面可用于修复运行依赖，避免读请求产生运行副作用或随历史无限增长；API 数组形状保留，超过 100 条需跟随游标。 |
| 2026-10-28 | 现状建档 | 渠道共享对话、审批与执行，不引入渠道专属 Agent Loop。Web 实时化需保留提交后的权威结果；飞书 unknown 不通过重复发送消除。 | 明确当前边界与待补齐链路；本次仅建立文档，未修改运行时。 | [跨功能契约](product-overview.md#contract-0-文档效力与实现规则) |

后续记录需包含实际比较过的选项、采用理由、兼容/迁移影响和相关需求或 PR；未决建议不能写成已实施决策。

2026-10-28：F-019 将五秒条件整页 reload 替换为独立区域条件读取。比较整页轮询、事件推送与已提交投影读取；选择复用现有查询和 SSR，保持输入/渲染/更新生命周期各自内聚。接口新增只读 updates 与可选 JSON 审批回执，原接口默认行为和领域协议不变；无需数据迁移。真实浏览器验证草稿/附件/焦点/选区/阅读位置、状态与审批、断网和会话恢复。
