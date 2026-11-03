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

**需求**：用户能登录本地 Web，使用 Conversation、Tasks、Settings、附件和审批，优先定位待回答、批准、审阅或修复的事项，并找到相应任务与下一步。

**当前实现**：独立 Conversation、Tasks 列表/详情、分层 Settings 和 Developer Trace 列表/详情已接通，Tasks 提供 Needs me 筛选，并在列表首部优先展示具名问题、确切成果审阅、冻结审批与当前可处理故障。模型配置按来源 → 保存 → 验证 → 显式选择推进；通知设置独立，认证、附件、冻结审批、原生表单和语义 fallback 保留。

**边界**：待办来自 F-004 的当前记录、冻结审批和 Task 执行事实；已完成 Task 的当前成果审阅也会显示。复用现有导航，依据权威状态展示等待谁、需要什么，不从模型进度文本推测；普通健康状态保持安静，计数与明细一致。处理动作经现有 Main、审批或设置入口，不绕过任务控制。实时区域刷新见 F-019，开发区由 F-031 跟踪。

**验收目标**：真实浏览器验证设置、发消息、附件、审批和对话展示；各类事项可正确定位并处理，刷新后计数与明细收敛且无重复项，已处理或过期事项不继续请求行动；后台更新保持草稿、附件、焦点与阅读位置，断线与陈旧页面不误报完成。

<a id="f-019"></a>

### F-019 · Web 实时状态与回复更新

**需求**：用户阅读或编辑草稿时仍能接收状态和回复更新，且草稿不会丢失。

**当前实现**：每秒条件读取已提交的会话、审批、工作状态与任务摘要，只更新对应区域。输入节点、未发送草稿、附件、焦点和选区保留；阅读旧内容时保持位置并提示新消息。审批使用原冻结哈希异步提交，确认后读取当前状态。

**边界**：这是已提交结果的实时刷新，不展示未经提交的模型 token。网络错误保留当前内容并退避重连；页面隐藏时暂停，重新可见后立即读取。会话/绑定失效保留草稿，提供另页登录与显式重试。任务阶段摘要由 [F-005](tasks.md#f-005) 的 Root 进度链路生产。

**验收目标**：长任务与输入草稿并行时，回复/进度更新可见，焦点、草稿及附件不被覆盖。

<a id="f-020"></a>

### F-020 · 可维护的飞书交互与通知

**需求**：已绑定个人身份能长期接收消息、附件和审批；用户在原工作话题收到适合该渠道的结果，详细成果与简短通知相互关联，减少重复打扰。

**当前实现**：签名回调、p2p 身份绑定、收发和本地回执已实现。自建应用可通过 SecretStore 中的 app secret 自动换取 tenant access token，按有效期提前刷新，并发请求共用刷新结果；CLI 可检查连接与凭据失效状态，也可显式选择外部维护 access token。新接收的原消息来源随路由冻结，同步回复和自动偏好下的后台 Task 结果引用该原消息；不同来源的 Task 结果经 Main 分组交付。配置可信工作台地址后，IM 提供简短预览与完整回复/文件入口，完整语义和产物保存在原工作台，不额外上传重复详情。

**边界与剩余工作**：当前支持配置型自建单租户应用与已绑定个人 p2p 渠道，不包含群聊或应用商店多租户授权。原消息关联限定于新接收且已绑定的个人消息；旧接受路由不回填，未找到明确来源的任务保留既有通知偏好，不猜测关联。GitHub/PR 事件接续与跨平台成果组织仍依赖 F-042/F-004，尚无新的多渠道事件归并或远端权威回执查询。无可信身份/目的地关联不跨渠道发送，目的地失效不得擅自扩大受众；沿用显式通知偏好、冻结路由与审批。未知投递仅查询已存本地回执，不通过重发消除未知。真实飞书展示、应用权限、工作台可达性与模型语义简洁度仍需 Live/质量验证。

**验收目标**：真实绑定、token 生命周期、通知偏好、撤销、重启与不确定投递正确；结果发送到原话题并可追溯对应成果，多渠道重试/重启不重复产生通知意图，目标平台已可见的详情不机械重复到 IM；目的地失效不扩大受众，未知发送结果不盲目重发。

## 如何使用

标准 `steward` 使用终端；配置 Web 本地监听与运行时后执行 `steward web serve`，使用其打印的本地地址。默认本机直接进入；`token_ref` 是可选登录保护。

飞书需配置应用凭据、回调入口，并通过可信 CLI `steward channel manage` 绑定个人身份；群聊不在当前范围。Settings → Notifications 可选择未来通知渠道，保存后显示实际选择；原目的地不可用时显式提示，不回退成 Automatic。已提交的消息、审批、任务阶段与状态按区域更新。

### Web 待办与回答（F-018/F-004）

Tasks 顶部的 Needs your attention 优先呈现已保存的问题、成果审阅、具体动作审批和需要讨论下一步的失败/未知结果。每条问题或成果可进入对应 Task；成果链接指向确切文件，关联消息可回看原对话。任务状态不决定审阅是否完成：已 completed 的 Task 仍可保留当前审阅。

Task 详情中的 Answer question / Review result 表单将回答作为已认证 Web 用户输入交给 Main。回答不直接重启 Task，成果审阅不批准外部动作；冻结审批仍使用原参数和哈希。相同回答跨 Web 页面或 CLI 重试返回原回执，首次接纳的渠道和回复目标不因重试而改变。暂停、取消、Forget、新修订或过期后的旧回答被明确拒绝。

Chat 右上角提供统一待办入口；问题、审阅或故障在标注的新标签页中打开，原对话的草稿和附件保留。只有动作审批时继续保留 Chat 内的原审批定位。当前页数量由同一呈现投影计算，存在后续页面时用 `+` 和更多入口说明范围，不把第一页说成全部待办。

表单不随普通状态更新重建。另一端回答或任务失效使当前表单消失时，非空本地输入保留为明确未提交、只读且可复制的草稿；它不计入可操作事项。已知过长/无效输入提示先修改，断线或确认丢失保留草稿并读取当前状态，不自动重发。原生表单保留无 JavaScript 的提交与错误恢复路线。

### 飞书凭据与连接检查（F-020）

自建应用可选择自动维护 tenant access token：

```yaml
feishu:
  app_id: your-app-id
  tenant_key: your-tenant-key
  verification_token_ref: env:STEWARD_FEISHU_VERIFICATION_TOKEN
  encrypt_key_ref: env:STEWARD_FEISHU_ENCRYPT_KEY
  app_secret_ref: env:STEWARD_FEISHU_APP_SECRET
```

这里只填写 SecretStore 引用，实际密钥不进入 YAML。`app_secret_ref` 与 `access_token_ref` 必须且只能配置一个。原有外部 token 方式仍可用；新配置在重启后生效，不改变已绑定的身份或通知目标。缓存只存在于进程内，不向领域数据库保存 access token。

```sh
steward -config config.yaml channel status
steward -config config.yaml channel check
```

`status` 只显示配置模式，不发网络请求；新 CLI 进程显示 `unchecked` 是正常情况。`check` 按需取得 token 并读取机器人的自身信息，不发送消息、不修改绑定或处理未知投递。无需先设置活动模型。结果以 `state` 为准：`ready`、`credentials_unavailable`、`credentials_rejected`、`unavailable` 或 `not_configured`；失败时附安全的 `action`，不回显密钥或供应商原始响应。`ready` 证明检查当时机器人信息接口鉴权成功，不保证全部业务 API 权限。

自动模式在到期前刷新；显式 token 拒绝会使对应缓存失效，下一次由原 Delivery 队列调度的安全尝试使用新 token。临时取得凭据失败允许既有有限退避；应用凭据被明确拒绝会终止该次投递，并保存 `credentials_rejected`。已经发出的消息若超时、断连或缺少有效回执，仍是 `unknown`，重启和凭据刷新都不会授权重发。修正凭据后先 `channel check`，再通过正常对话继续工作；不把旧未知消息当作未发送。

### 飞书原消息回复与成果链接（F-020）

新飞书输入的同步答复引用已验证的原消息；后台 Task 完成后仍先经 Main 形成语义交付。在通知偏好为 Automatic 时，Task 结果回到发起该任务的原绑定和消息；显式选中的其他渠道优先，不同时再向 IM 复制一份。审批通知仍保留完整冻结参数，不能用摘要替代授权。

可选配置：

```yaml
feishu:
  # 与上述必需应用/SecretStore 配置合并。
  workspace_url: http://127.0.0.1:8787
```

`workspace_url` 是操作员确认的工作台 origin，只接受 HTTPS 或 literal loopback HTTP，不含账号、密码、路径、查询参数或 fragment。它不部署工作台、不改变监听地址或权限，不携带登录 token。loopback 链接只适合同一电脑，飞书手机或其他设备不能据此访问；远程地址需操作员已有的 HTTPS 部署与访问控制。点击后仍经过现有 Web 身份检查。

配置后普通结果显示最多 1200 个字符的确定性预览和 **Open full response and files**，链接直接定位保存的完整 Conversation 消息与附件。它不是另一次模型摘要，不删除完整语义。已有可访问文件保留在工作台，不机械上传到 IM；未配置地址时保持完整卡片和原附件投递。链接、卡片和原消息目标在第一次传输准备时持久冻结，改配置或重启不会重定向同一交付。原消息不可回复或发送结果未知时，不降级到别的收件人或顶层新消息。

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

本功能协议在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="channels-web-feishu-and-safe-delivery"></a>

## Web, Feishu and safe delivery

Authenticated interaction and rendering use the existing
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
  # Optional: token_ref: env:STEWARD_WEB_TOKEN
feishu:
  app_id: your-app-id
  tenant_key: your-tenant-key
  verification_token_ref: env:STEWARD_FEISHU_VERIFICATION_TOKEN
  encrypt_key_ref: env:STEWARD_FEISHU_ENCRYPT_KEY
  access_token_ref: env:STEWARD_FEISHU_ACCESS_TOKEN
```

Web opens directly for a loopback client when `token_ref` is omitted; no workspace token or session is required. An explicit `token_ref` opts into the existing token/session login, with tokens of at least 24 bytes. Credential values are SecretStore references, never literal tokens. Set credentials through the installation's credential store
or its explicit environment-reference backend. Feishu's configured access token
must remain valid; rotation/refresh belongs to the credential owner. A missing
or expired credential never falls back to another account or recipient.

The installation owner enrolls the exact Feishu tenant/app/open ID and p2p chat
through the OS-authenticated CLI. A callback cannot enroll itself. Group chats
and bot senders are rejected to avoid broadening access to the personal history.

```sh
printf '%s' '{"op":"bind","channel":"feishu","subject":"your-tenant-key:your-app-id:your-open-id","destination":"your-chat-id","enabled":true,"version":0}' | bin/steward -config /absolute/config.yaml channel manage
bin/steward -config /absolute/config.yaml web serve
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

When `token_ref` is configured, Web uses a process-local, one-hour HttpOnly/SameSite=Strict session after token
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
bin/steward delivery list  # retained requests, including unknown/failed
bin/steward delivery work  # one eligible transport/reconciliation scan
bin/steward chat work      # drain Agent work, then delivery
bin/steward tui            # live delivery polling and /history
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

### Steward Web 视觉重构

正式 Web 采用确认的 Steward 单侧栏黑白 Demo：对话、任务、审批、成果、模型/账户、通知、能力与 Developer 继续使用既有实现；Chat 右上角按需读取真实 Worker 执行及 Computer 只读画面。没有引入多会话或独立文件库。品牌与兼容范围见[品牌功能](branding.md)。

Web 默认英文；Settings → Preferences → Interface language 可以保存简体中文。刷新、跨页和实时界面片段使用已选语言，原始对话和交付内容不变。见[品牌与界面语言](branding.md)。


### Steward 工作区精修

导航仅保留 Chat、Tasks、Settings、Developer；任务筛选留在 Tasks 内。Chat 顶部居中两行显示 Agent 名称 Ghost 与动态头像，下方持续对话；右上角显示 Computer 和当前 Worker，点击分别打开互斥详情面板。Worker 包括正在执行和已经结束的记录，按实际开始时间（未开始则取创建时间）倒排，状态保持后端原值，不把 Worker 结果标为用户交付。Task 恢复独立列表与详情，不额外查询 Conversation。成果从任务详情及交付消息进入。

首页待审批提示与实际审批卡片使用同一份权威读取，覆盖已内嵌消息及独立待审批卡片，解决后同步移除。打开具体审批保留草稿；向上阅读可主动回到最新消息。当前提示仅覆盖已有审批，不宣称 F-004 的待回答、审阅闭环已实现。

Chat 右上角 Computer 可展开只读桌面画面与已提交的操作进度；打开且页面可见时约每两秒读取一次完成的画面，关闭/后台停止。查看不会启动或恢复 VM，不提供鼠标键盘接管；离线、暂停、画面不可用及中断都有明确状态。机制与兼容性边界见[Computer 设计](../designs/computer-use.md)。

Chat 的 Computer、Worker 与待确认入口依次在右上角上下排列；窄屏使用具名图标按钮，仍采用互斥详情面板。

任务与执行详情根据导航之后的实际内容宽度切换双列／上下布局。窗口小于 1280px 时，Computer／Worker 使用浮层，避免挤压对话与输入框。设置标签条自动露出当前栏目；能力配置示例放在编辑框之前，空白未配置时展开。键盘打开／关闭导航和切换执行树节点保留正确焦点。
