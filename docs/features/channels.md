# 终端、Web 与飞书

从多个渠道进入同一对话，并将回复发送到冻结目标。

[需求主索引](../feature-map.md) · [技术方案](../designs/channels.md) · [测试方案](../test-plans/channels.md)

## 功能说明

用户能完成模型设置、交代工作、查历史和状态。

用户能登录本地 Web，使用 Conversation、Ongoing work、Settings、附件和审批。

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

**需求**：用户能登录本地 Web，使用 Conversation、Ongoing work、Settings、附件和审批。

**当前实现**：认证路由、服务生命周期、前端嵌入与语义 fallback 已实现。

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

**当前实现**：签名回调、p2p 身份绑定、收发和本地回执已实现；access token 由外部维护。

**边界与剩余工作**：补凭据刷新/失效诊断体验；不确定远端投递目前只能依赖已保存本地回执，可信远端核实能力仍待定义。

**验收目标**：验证真实绑定、token 生命周期、通知偏好、撤销、重启与不确定投递；未知结果不盲目重发。

## 如何使用

标准 `erii` 使用终端；配置 Web token 与运行时后执行 `erii web serve`，使用其打印的本地地址。

飞书需配置应用凭据、回调入口，并通过可信 CLI `erii channel manage` 绑定个人身份；群聊不在当前范围。Settings 可选择未来通知渠道。当前 Web 的刷新方式仍有限。

## 详细操作参考

- [渠道接入详情](#channels-web-feishu-and-safe-delivery)
- [安装与部署详情](installation.md)
- [交付与回执协议](../designs/channels.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="channels-web-feishu-and-safe-delivery"></a>

## Web, Feishu and safe delivery

ERII-28 adds authenticated interaction and rendering to the existing
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
  token_ref: env:ERII_WEB_TOKEN
feishu:
  app_id: your-app-id
  tenant_key: your-tenant-key
  verification_token_ref: env:ERII_FEISHU_VERIFICATION_TOKEN
  encrypt_key_ref: env:ERII_FEISHU_ENCRYPT_KEY
  access_token_ref: env:ERII_FEISHU_ACCESS_TOKEN
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
printf '%s' '{"op":"bind","channel":"feishu","subject":"your-tenant-key:your-app-id:your-open-id","destination":"your-chat-id","enabled":true,"version":0}' | bin/erii -config /absolute/config.yaml channel manage
bin/erii -config /absolute/config.yaml web serve
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
- `/settings`: local/account model metadata, explicit Test/Select and future
  notification preference. Selecting a provider requires explicit consent.
- `/progress`: read-only Task summaries and aggregate Child counts; no raw Tool,
  model, Run ID or Child trace. Pause/continue/cancel stays in conversation.
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
bin/erii delivery list  # retained requests, including unknown/failed
bin/erii delivery work  # one eligible transport/reconciliation scan
bin/erii chat work      # drain Agent work, then delivery
bin/erii tui            # live delivery polling and /history
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

打开 Conversation 或 Ongoing work 即可自动接收更新，不需要额外配置。阅读上方内容时，“New messages”只提示有新内容，点击后再定位到最新消息，草稿和附件仍保留。输入消息和选择文件不会停止后台更新；批准/拒绝具体动作也不会整页跳转。

短暂断网时页面提示重连，恢复后读取当前已提交状态。会话失效时，在提示中的 Sign in 打开另一页完成登录，再回到原页点击 Retry updates；原页保留的草稿和文件无需重填。Settings 只实时更新全局状态和登录过程，不在后台覆盖配置表单或更换活动模型。禁用 JavaScript 时仍提供原有语义 HTML 与表单提交。
