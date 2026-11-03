# MCP 与真实外部能力：技术方案

[功能与需求](../features/integrations.md) · [测试方案](../test-plans/integrations.md) · [主索引](../feature-map.md)

本文的架构与流程描述当前有效实现；明确标记的待实现方案不表示已落地。实现级协议仍服从 [跨功能协议与约束](product-overview.md#contract-0-文档效力与实现规则)。

## 当前架构

MCP 适配器先发现允许工具，生成冻结 ToolSpec；调用时重新核对服务端声明，经受限子进程执行。真实服务扩展仍必须进入同一 Registry/Executor。

```mermaid
flowchart TD
    C["显式 MCP 配置"] --> D["发现与允许列表"]
    D --> R["Tool Registry / 冻结快照"]
    R --> E["ToolExecutor"]
    E --> P["受限 stdio 进程"]
    N["远程认证服务：待实现"] -.-> E
```

## 当前流程与目标差异

配置 → 发现并冻结工具 → Agent 选择工具 → 审批/执行 → 结果规范化。新增远程服务需先设计认证、边界和未知结果处理，再接相同执行路径。

- **F-024**：启动服务器、tools/list、tools/call、schema 指纹与工具允许列表已实现。 当前子进程受限、无继承凭据/网络；这不是通用远程认证 MCP 客户端。
- **F-025**：GitHub 精确 PR 监听提供 SecretStore PAT 认证读取及失效诊断/替换后恢复；Agent 的远程服务工具、OAuth 和写动作生命周期仍未实现。

## 代码与依赖

- [internal/tool/mcp/client.go](../../internal/tool/mcp/client.go)
- [internal/computer/sandbox/sandbox_linux.go](../../internal/computer/sandbox/sandbox_linux.go)
- [internal/app/runtime.go](../../internal/app/runtime.go)

### F-042 · 可靠的环境事件监听

当前消费者是显式配置一个 GitHub.com PR 的本地操作员；入口为 `github` CLI 与生产 `WithEvents` 生命周期。新增 `internal/observer/github` 私有适配器，使用固定 GitHub.com GET 路径，拒绝所有 HTTP redirect，分页由本地确切端点与页码构建，不跟随外部 Link 或响应 URL。凭据由 SecretStore 每轮重新解析；错误只保存安全代码。`/user` 身份用于排除自身评论/评审；PR 响应核对仓库全名、编号及 head，并在首次成功时固定不可变 repository ID。

领域必需机制为可靠持久化、版本去重、短事务、停止/隐私 fencing 与健康；当前政策为一个确切 PR、只读 PAT、定期重扫和有限历史。两者分开，均只在仓库内部使用，不稳定通用 Source 插件协议、外部 schema 或新的 Task 生命周期。监听不会调用 `AcceptInbound/Submit`，以免把第三方文本变成用户话语或越过 F-045 的主动启用边界。

每轮流程：SQLite 注册/读取 watch 并接受版本/隐私 epoch 票据 → 在事务外读取完整的评论、review 和当前 head check-runs → 单短事务重检票据、enabled、隐私 epoch、repository ID → 插入新观察及现有 EventOutbox 唤醒 → 更新 head 与成功时间。完整扫描失败只保存故障状态，不保存部分事件或推进成功边界。同类持续故障只在安全代码变化时写健康唤醒，恢复再写一次；内部唤醒不直接创建工作或用户通知。并发 poll、stop/resume 与 Forget 会使旧票据失效。运行时 shutdown 取消并等待 watcher 退出后关闭数据库。

Migration 033 在 `state.db` 增加 `github_watch/github_observation`；不修改既有 Run、工具或冻结能力快照。watch 身份固定为仓库名/PR，repository ID 进一步防止名称复用。来源 ID、类型、来源时间、规范化字段及完整源正文摘要构成版本 hash，唯一键 `(scope,event_key)` 使重复扫描/重启不产生重复观察。review 的 submitted_at 不是编辑时间，因此完整源正文摘要仍参与私有 event_key，即使修改发生在 16 KiB 展示边界之外也产生新版本；原始长正文和该摘要字段不另行持久化或展示。历史观察保持不可变；Forget 后只查询当前 epoch，显式 resume 不会将旧版本重新发布。所有查询只读，不触发扫描或修改健康。

选择全量有界重扫而非时间高水位：评审/CI 端点的修改和乱序不能由单个时间游标完整表示，重扫能补回 GitHub 仍保留的版本；代价是 API 读取量。每页 4 MiB、每类 20 页、总事件正文 8 MiB，超限整体失败，不伪装完整接收。正文截断保留来源链接，接收不宣称 GitHub 的历史审计或外部 exactly-once。旧 head 的 CI、删除和中间变更不受保证；不扩展 webhook 服务器、OAuth、通用工作流或自动 Task 接续。

### 明确 Task 关联与 Main 接续

本机所有者可 CAS 绑定确切 watch scope、已有 Task 和当前 Task epoch；关联冻结 privacy epoch、授权 generation 及当前观察序号作为接收起点。`github_task_binding` 是此单一 GitHub 适配器的私有授权/消费位置，不是通用 Source/工作流协议。只处理绑定之后的新观察，不以话题相似、自由文本 TaskRef 或 PR 内容自行关联。

`main_inbox` 的既有 event_id 强引用 task_update，因此不将外部观察伪装成 Worker 结果。私有 `github_task_followup` 记录观察→现有 Main 的不可变因果，复用 createMain、Actor、ToolInvocation、EventOutbox 和恢复扫描；每次只接纳一个来源版本，观察与因果唯一键防止重启/乱序重复。消息使用已有 RuntimeContext 的 JSON 框、Origin=runtime，明确是第三方证据而非新用户命令/审批；不写 main_input 或用户 Conversation。

交给 Main 的原目标与来源正文各最多 4 KiB，截断明确标记为 preview；完整 Task/观察继续保存在原存储，不改写版本身份。信息不足时沿已有授权读取原 Task 或询问所有者，不凭缺失细节继续。这样单个长评审不能制造无法压缩的受保护上下文。

只有因果 Main 受此附加裁决：同一 Task 至多一个新 append，来源/hash 已接纳的重复仍按 Task 原规则复用回执；新 Task、另一 Task、Automation/Memory 控制及直接副作用被拒绝。原 Task Worker 继续原工具、审批和 unknown 裁决。成功 append 在同一事务记录 continued_epoch 并推进 binding 期望代次，使合法接续不会撤销自己，其他动作不能借此重新授权。

执行 runtime 准备必须在发现 Tool/依赖、公开可执行 Actor 或运行任何恢复前，以可信 owner 配置调用 ConfigureGitHub，包括配置为空；Store 当前 exact scope 私有且默认拒绝。OpenControl 的 status/evidence/model 等只读打开不调用持久配置协调，不改变既有 binding/generation；显式 owner bind 在 CAS/Task/watch 校验通过后与其他 scope 撤销同事务提交，read-only 操作不替代执行启动。单事务禁用其他 scope 的绑定、增加 generation 并撤销旧 Main；配置重加不恢复绑定，须显式 bind。恢复查询只接纳当前确切 scope；调度选择、可执行等待、claim 与 append 都核对当前 scope，旧进程跳过其他 scope 的 Main，普通 Worker 仍可前进。停止中的原 join 边界不变。既有 claim/模型接受/Host 工具写入继续用 DB generation、Task epoch/status 与隐私 fence 拒绝迟到结果，不增加新的 claim 授权层。只有具有该私有因果行的 Main 增加这些条件，普通 Main/Worker 仍采用原权限与运行语义。unbind、watch stop、Task pause/cancel/manual complete、配置移除/切换、明确 CREDENTIAL_REJECTED 或 REPOSITORY_IDENTITY_CHANGED 原子撤销旧因果 Main；私有终止原因仍由同一个 stop recovery 排空，Run 终态不可逆。Forget 继续使用现有屏障。新用户输入将外部 Main 标为 superseded，并用既有后继 Main 接纳输入，避免真实用户消息因旧观察授权失效而遗失。

内部 Task explicit_end_epoch 区分人工 complete 和 once Root 自然完成，未增加 Task 状态或修改模型工具 schema。自然完成的原绑定可继续；人工结束的当前代次不能自动恢复，只有显式用户 append 后的新代次才可重新绑定。新迁移保留原 Run/模型/能力快照与历史回执，不回填自动关联；此前没有任何 GitHub Task 绑定。

轮询健康与已接纳因果的授权分开：仅 healthy 且无 failure_code 的 watch 可新 promotion；已 assigned 的原始观察不因 UNAVAILABLE、INVALID_RESPONSE、CREDENTIAL_MISSING、ACCESS_DENIED_OR_RATE_LIMITED、SOURCE_UNAVAILABLE 或 SOURCE_MOVED 丢失。CREDENTIAL_MISSING 合并了 SecretStore 暂不可读等原因；403/429 同码也不能断言明确撤权；404/3xx 只证明当前来源不可读/移动，不跟随 redirect、不改变原 exact scope。选择保留先前完整接纳的证据，避免网络抖动永久吞掉已推进唯一键/序号的观察；明确凭据拒绝和仓库身份变化采取保守 hard revoke，健康恢复也不自动重新绑定。该策略不保证当前来源仍可用，也不授予新的外部动作权限。

真实消费者只有可信 App 启动与此 GitHub 适配器，当前 scope、绑定 generation、健康/授权区别和不可变因果为领域必需；通用 Source、热配置平台、通用 claim 框架与新的错误分类没有证据，不暴露或稳定化。配置撤销持久保留代次，轮询故障保留已接纳工作，两者复用现有短事务与 fence，是比新增第二调度/授权管线更窄的可逆设计。

F-020 继续负责原消息/话题路由；GitHub 因果 Main 通过已有 github_task_followup.task_id 复用 taskReplyOrigin，与原 Task Worker 一起回原绑定/消息。显式通知偏好、撤权与未知投递裁决不变；来源接收和 Task 控制不决定新通知目的地。剩余范围为其他真实服务、外部关联类型、源平台历史保留及真实模型范围判断质量。

## 验证与交付

验收场景、已有入口和剩余覆盖统一在 [测试方案](../test-plans/integrations.md) 维护。实际执行结果以对应版本的 PR 或 CI 证据为准；架构说明不证明回归或 Live 验证通过。

## 协议、实现与配置约束

本功能协议在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="capabilities-mcp-skills-and-controlled-coding"></a>

## MCP, Skills and controlled coding

External MCP and curated Skill content enter the existing registry and ToolExecutor.
File/command tools and empty Skill discovery are built in by default (see [execution](coding.md)). Main and
Root receive the same frozen work surface; Root still lacks Task control and
user delivery. Neither role receives Skill management. No second discovery API,
execution loop, approval pipeline or model credential channel is introduced.

Use `config.example.yaml` for configuration. Paths are operator configuration;
frozen ToolSpecs represent their scopes with hashes. A `resource_hash` binds
each new ToolSpec to its configured scope. Descriptions and hashes are part of
frozen `tools_json`; Skill resources also determine `skill_set_hash`.
Descriptions reach the Pi Bridge. Full Skill bodies enter context only on view.

<a id="capabilities-mcp-transport"></a>

### MCP transport

Configure an absolute executable, argument vector, dedicated provider directory,
server name and tool allowlist. No shell, inherited credentials, proxy settings,
user HOME or network access is available to the server. Its executable, system
runtime files and configured provider directory are the only file-read scopes;
only that directory is writable. Put interpreted server code/dependencies there
if using a system interpreter. The provider directory must not overlap domain
data, Skills, the coding checkout or another provider directory. Scope checks
resolve aliases through existing ancestors even before new data directories
exist. The command must be self-contained or use
the exposed system runtime; arbitrary host package paths are unavailable.

The adapter implements MCP `2025-03-26` newline JSON-RPC stdio initialization,
initialized notification, bounded paginated `tools/list`, and `tools/call` text
results. Each operation owns a short-lived initialized session; every call
rediscovers its advertised tool and rejects schema/description drift before
dispatch. Unknown allowlist entries, duplicate/invalid names, oversized frames,
unsupported schemas, server requests, malformed replies and unsupported content
fail closed. Notifications are bounded; they never update a running catalog.
See the [MCP transport specification](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports)
and [tools specification](https://modelcontextprotocol.io/specification/2025-03-26/server/tools).

The operator must classify each exposed tool as `read_only` or `external_effect`.
Provider annotations cannot grant permissions. Reads use `safe`; effects use
`high` risk and `never` replay because standard MCP has no authoritative receipt
query/idempotency contract. Every effect requires its own frozen approval.
Transport loss or a malformed result after dispatch is `unknown`; no retry or
claimed success follows. A call result must be an object with a non-null content
array, text entries with explicit string text, and a boolean `isError` when
present. Empty content arrays and explicit empty text are valid.
Raw provider errors are discarded. Cancellation terminates and collects the
session's process group. The executor supplies timeout, audit, conflict, quota,
result ordering and Artifact spillover semantics. A server-wide conflict key
serializes calls to the same configured provider across Runs.

HTTP/SSE, authenticated external MCP connections and production website access
are not implemented. The local stdio adapter exercises real structured protocol
calls against deterministic external-system Fakes without opening host network
or credential access. Future Guest-connected adapters must retain the same
registry, frozen policy and invocation lifecycle.
