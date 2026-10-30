# 终端、Web 与飞书

从多个渠道进入同一对话，并将回复发送到冻结目标。

[需求主索引](../feature-map.md) · [技术方案](../designs/channels.md) · [测试方案](../test-plans/channels.md)

## 功能说明

用户能完成模型设置、交代工作、查历史和状态。

用户能登录本地 Web，使用 Conversation、Tasks、Settings、附件和审批。

用户阅读或编辑草稿时仍能接收状态和回复更新，且草稿不会丢失。

已绑定个人身份能接收消息、附件和审批，并维持长期可用。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-017"></a>

### F-017 · 终端交互

**需求**：用户能完成模型设置、交代工作、查历史和状态。

**当前实现**：标准启动/TUI/CLI、输入与已交付回复投影已接通。

**边界与剩余工作**：终端输出没有远端确认协议，不宣称屏幕显示 exactly-once。

**验收目标**：验证真实终端首条输入、后台工作时追加、退出和重启。

<a id="f-018"></a>

### F-018 · 基础 Web 工作台

**需求**：用户能登录本地 Web，使用 Conversation、Tasks、Settings、附件和审批。

**当前实现**：独立 Conversation、Tasks 列表/详情、分层 Settings 和 Developer Trace 列表/详情。模型配置按来源 → 保存 → 验证 → 显式选择推进；通知设置独立。认证、附件、冻结审批、原生表单和语义 fallback 保留。

**边界与剩余工作**：实时区域刷新见 F-019；开发区由 F-031 跟踪。

**验收目标**：真实浏览器验证设置、发消息、附件、审批和已有对话展示。

<a id="f-019"></a>

### F-019 · Web 实时状态与回复更新

**需求**：用户阅读或编辑草稿时仍能接收状态和回复更新，且草稿不会丢失。

**当前实现**：每秒条件读取已提交的会话、审批、工作状态与任务摘要，只更新对应区域。输入节点、未发送草稿、附件、焦点和选区保留；阅读旧内容时保持位置并提示新消息。审批使用原冻结哈希异步提交，确认后读取当前状态。

**边界**：这是已提交结果的实时刷新，不展示未经提交的模型 token。网络错误保留当前内容并退避重连；页面隐藏时暂停，重新可见后立即读取。会话/绑定失效保留草稿，提供另页登录与显式重试。任务阶段摘要由 [F-005](tasks.md#f-005) 的 Root 进度链路生产。

**验收目标**：长任务与输入草稿并行时，回复/进度更新可见，焦点、草稿及附件不被覆盖。

<a id="f-020"></a>

### F-020 · 可维护的飞书交互与通知

**需求**：已绑定个人身份能接收消息、附件和审批，并维持长期可用。

**当前实现**：签名回调、p2p 身份绑定、收发和本地回执已实现。自建应用可通过 SecretStore 中的 app secret 自动换取 tenant access token，按有效期提前刷新，并发请求共用刷新结果；CLI 可检查连接与凭据失效状态。也可显式选择外部维护 access token。

**边界**：完成范围为配置型自建单租户应用与已绑定个人 p2p 渠道；不包含应用商店多租户授权。没有可信远端回执的投递保留 `unknown`、只查询本地已存回执，不通过重发消除未知。真实飞书账户的权限、可达性和平台行为需单独 Live 验证。

**验收目标**：验证真实绑定、token 生命周期、通知偏好、撤销、重启与不确定投递；未知结果不盲目重发。

## 如何使用

标准 `collie` 使用终端；配置 Web token 与运行时后执行 `collie web serve`，使用其打印的本地地址。

飞书需配置应用凭据、回调入口，并通过可信 CLI `collie channel manage` 绑定个人身份；群聊不在当前范围。Settings → Notifications 可选择未来通知渠道，保存后显示实际选择；原目的地不可用时显式提示，不回退成 Automatic。已提交的消息、审批、任务阶段与状态按区域更新。

### 飞书凭据与连接检查（F-020）

自建应用可选择自动维护 tenant access token：

```yaml
feishu:
  app_id: your-app-id
  tenant_key: your-tenant-key
  verification_token_ref: env:COLLIE_FEISHU_VERIFICATION_TOKEN
  encrypt_key_ref: env:COLLIE_FEISHU_ENCRYPT_KEY
  app_secret_ref: env:COLLIE_FEISHU_APP_SECRET
```

这里只填写 SecretStore 引用，实际密钥不进入 YAML。`app_secret_ref` 与 `access_token_ref` 必须且只能配置一个。原有外部 token 方式仍可用；新配置在重启后生效，不改变已绑定的身份或通知目标。缓存只存在于进程内，不向领域数据库保存 access token。

```sh
collie -config config.yaml channel status
collie -config config.yaml channel check
```

`status` 只显示配置模式，不发网络请求；新 CLI 进程显示 `unchecked` 是正常情况。`check` 按需取得 token 并读取机器人的自身信息，不发送消息、不修改绑定或处理未知投递。无需先设置活动模型。结果以 `state` 为准：`ready`、`credentials_unavailable`、`credentials_rejected`、`unavailable` 或 `not_configured`；失败时附安全的 `action`，不回显密钥或供应商原始响应。`ready` 证明检查当时机器人信息接口鉴权成功，不保证全部业务 API 权限。

自动模式在到期前刷新；显式 token 拒绝会使对应缓存失效，下一次由原 Delivery 队列调度的安全尝试使用新 token。临时取得凭据失败允许既有有限退避；应用凭据被明确拒绝会终止该次投递，并保存 `credentials_rejected`。已经发出的消息若超时、断连或缺少有效回执，仍是 `unknown`，重启和凭据刷新都不会授权重发。修正凭据后先 `channel check`，再通过正常对话继续工作；不把旧未知消息当作未发送。

### Web 操作路线

| 分区 | 入口与主要操作 | 下一步 |
| --- | --- | --- |
| Conversation | `/` 输入消息、添加/移除附件、确认具体审批；历史使用 Earlier messages | 后台工作在 Tasks 跟踪；缺少可用模型时进入设置 |
| Tasks | `/tasks` 按 Open / Needs me / Finished / All tasks 筛选、搜索与翻页 | 点击任务查看目标、当前阶段、审批、Main 交付、附件与历史；返回原始请求或带任务上下文继续对话 |
| Settings → Models | `/settings/models` 显示当前模型和其他已保存模型 | Connect model → 选择来源 → 保存 → 模型详情 Test connection → 勾选授权 → Select model → 开始对话 |
| Settings → Notifications | `/settings/notifications` 显示已保存的通知目的地 | 显式保存未来通知偏好；不迁移已冻结交付 |
| Developer → Traces | `/developer/observability` 筛选执行列表 | 打开独立 Trace 详情，选择树/时间线节点，检查 Context、Requests 或相邻轮次 Compare |

Task 详情只展示该任务已由 Main 交付的内容，不将 Worker 结果冒充最终回复。Continue in conversation 会带上可见且可移除的任务上下文，再交给 Main 理解要求；页面不直接调用 Task 控制。审批只在每个页面显示一次主要操作，完整参数和原冻结哈希保留；不要求 Web 用户复制 CLI 命令。

登录后回到原本的本地子页面。设置失败在对应页显示安全错误并保留可恢复字段；增强后的发送失败保留草稿和附件，不自动重发。没有 JavaScript 仍能通过原生链接和表单完成主要路线。`/settings`、`/progress` 和旧开发者深链接跳转到新页面。

## 详细操作参考

- [渠道接入详情](#channels-web-feishu-and-safe-delivery)
- [安装与部署详情](installation.md)
- [交付与回执协议](../designs/channels.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="channels-web-feishu-and-safe-delivery"></a>

## Web, Feishu and safe delivery

retired task #28 adds authenticated interaction and rendering to the existing
`InboundEvent -> Actor -> Main/Worker -> ToolInvocation -> Conversation ->
DeliveryRequest/Outbox -> Adapter` path. There is one Conversation per Agent.
No channel creates its own reasoning loop, Task state machine or approval store.

<a id="channels-launch-and-identity-enrollment"></a>

### Launch and identity enrollment

Configure the real Provider Bridge as in `config.example.yaml`. Without an
active, tested profile the Web setup page remains available and ingress stays
`setup_required`. Neither startup nor a missing credential selects a Fake.

```yaml
web:
  listen: 127.0.0.1:8787
  token_ref: env:COLLIE_WEB_TOKEN
feishu:
  app_id: your-app-id
  tenant_key: your-tenant-key
  verification_token_ref: env:COLLIE_FEISHU_VERIFICATION_TOKEN
  encrypt_key_ref: env:COLLIE_FEISHU_ENCRYPT_KEY
  access_token_ref: env:COLLIE_FEISHU_ACCESS_TOKEN
```

These are SecretStore references, never literal tokens. Web access tokens need
at least 24 bytes. Set credentials through the installation's credential store
or its explicit environment-reference backend. Feishu's configured access token
must remain valid; rotation/refresh belongs to the credential owner. A missing
or expired credential never falls back to another account or recipient.

The installation owner enrolls the exact Feishu tenant/app/open ID and p2p chat
through the OS-authenticated CLI. A callback cannot enroll itself. Group chats
and bot senders are rejected to avoid broadening access to the personal history.

```sh
printf '%s' '{"op":"bind","channel":"feishu","subject":"your-tenant-key:your-app-id:your-open-id","destination":"your-chat-id","enabled":true,"version":0}' | bin/collie -config /absolute/config.yaml channel manage
bin/collie -config /absolute/config.yaml web serve
```

`web serve` prints its loopback URL, enrolls the local browser identity once,
and runs in the foreground until SIGINT/SIGTERM. It owns HTTP ingress, scans the
same Actor/Automation claims and independently drains the same Delivery Outbox,
so one busy Worker cannot delay another Run’s approval notice. It joins its
work on shutdown. An explicitly revoked Web binding stays revoked on restart.
`channel manage` also accepts `list`, `preference`, `prefer` and versioned `bind`
updates. Set `enabled:false` with the current binding version to revoke access.
There is no destination migration: enroll a new destination and select it for
future work. Frozen deliveries keep their original destination. A changed Feishu app/tenant
cannot acquire an old binding’s pending delivery.

Web uses a process-local, one-hour HttpOnly/SameSite=Strict session after token
login, or a Bearer token for API access. It rejects DNS rebinding, cross-site
POSTs and invalid identities, provides no CORS access, and serves a restrictive
CSP. Auth URLs and device-code events stay ephemeral and are visible only in the
authenticated settings surface. Account login, reconnect and logout use the existing Manager/Bridge
lifecycle. The settings picker retains connected account metadata across restart.
Account model additions use its authenticated visible-model catalog. Credentials never enter Conversation, projections or evidence.

<a id="channels-user-surfaces-and-callback-contract"></a>

### User surfaces and callback contract

- `/`: shared semantic conversation, attachments and pending frozen approvals.
- `/settings/models`, `/settings/models/new`, `/settings/models/{profile}`: current
  model, source choice and focused configuration/verification/explicit activation.
  `/settings/notifications` owns future notification preference.
- `/tasks`, `/tasks/{taskRef}`: bounded, searchable Task workspace and read-only
  details including Main delivery and approvals. Pause/continue/cancel stays in
  conversation; no raw Tool, model, Run ID or Child trace in everyday pages.
- `GET /api/conversation`, `/api/progress`, `/api/status`, `/api/auth`: authenticated
  projections. `POST /api/message` accepts only `source_id` and semantic `content`.
  `POST /upload` stores a local Artifact; `/artifact?ref=...` resolves only through
  the authenticated owner scope and downloads as an attachment, never active HTML.
- `POST /feishu/callback`: Feishu v2 `im.message.receive_v1`, signed exact bytes,
  five-minute timestamp freshness, configured token/app/tenant and bound owner/chat.
  AES-256-CBC encrypted envelopes are supported. URL verification may omit signature
  headers and must authenticate with the configured verification token. It creates
  no input, Run or binding. Publish only this callback through a TLS reverse proxy;
  keep the Web control surface on loopback.

Feishu text and file/image messages normalize to the same semantic ContentParts
as Web/TUI. Downloaded resources are tied to the verified source message. Feishu
approval responses use `/approve APPROVAL_ID FROZEN_HASH` or `/deny ...`; Web uses
controlled confirmation forms. Both use the same frozen-hash Security transition.
Duplicate decisions are idempotent; conflicting, invalidated or expired approvals
fail closed. Steering invalidates old approval arguments before dispatch.

Normal synchronous replies return to the latest input's bound channel. TUI input
remains TUI even if earlier Steering came from Feishu. A Main reply without user
input, Worker approval, completion, failure or needs-input notice uses the future
notification preference. `auto` prefers the first enabled Feishu binding, then
falls back to TUI. Multiple enrolled destinations can be selected explicitly in
settings. Ordinary progress never creates a notification. All routes freeze with
the semantic intent, before the first DeliveryRequest; preference changes cannot
redirect accepted messages. Revocation rejects pending deliveries to that binding.

<a id="delivery-commands-and-operations"></a>

### Commands and operations

```sh
bin/collie delivery list  # retained requests, including unknown/failed
bin/collie delivery work  # one eligible transport/reconciliation scan
bin/collie chat work      # drain Agent work, then delivery
bin/collie tui            # live delivery polling and /history
```

Delivery work runs even with `setup_required`; it does not activate an implicit
provider. TUI polls persisted transport work every 250 ms, including after its
Agent drain becomes idle. Semantic `chat history` remains independent of send
status. It can fill Conversation intents, but does not claim channel acceptance.

`config.example.yaml` documents `delivery.lease_seconds` (1–300),
`retry_milliseconds` (1–60000), and `max_attempts` (1–20). Defaults are 5 seconds,
250 ms, and five attempts. Known transient failures use bounded exponential
backoff. Failed requests remain inspectable; there is no automatic resend or
delete command. Unknown rows retain their uncertainty and reconcile by query.
Future remote adapters must supply their own truthful idempotency/query contract.

State migration v12 preserves v11 requests, keys, status, attempts and history;
it renames the old local channel `local_cli` to `tui` and creates outbox rows only
for pending requests. Prior sent messages remain visible as retained history,
without being resent. Empty/prior schemas and failed-DDL rollback are tested.
The terminal projection is part of state schema v12. Keep `state.db`,
`conversation.db`, and their SQLite WAL files together during operational backup.
New database files use mode 0600 in the private data root.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/channels.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/channels.md)。

## Web 实时更新（F-019）

打开 Conversation 或 Tasks 即可自动接收更新，不需要额外配置。阅读上方内容时，“New messages”只提示有新内容，点击后再定位到最新消息，草稿和附件仍保留。输入消息和选择文件不会停止后台更新；批准/拒绝具体动作也不会整页跳转。

短暂断网时页面提示重连，恢复后读取当前已提交状态。会话失效时，在提示中的 Sign in 打开另一页完成登录，再回到原页点击 Retry updates；原页保留的草稿和文件无需重填。Settings 只实时更新全局状态和登录过程，不在后台覆盖配置表单或更换活动模型。禁用 JavaScript 时仍提供原有语义 HTML 与表单提交。
