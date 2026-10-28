# 跨功能运行时契约

[产品架构](product-overview.md) · [需求主索引](../feature-map.md) · [产品测试方案](../test-plans/product.md)

## 当前职责

本页维护跨功能数据约定、事件一致性、状态权威、配置/Secret、启动关闭顺序和模块接口。局部协议分别维护在相应功能的技术方案中，不再另设独立跨功能协议与约束。

## 当前架构与流程

SQLite 是权威状态；Outbox 与 JetStream 传递唤醒。每个 claim/epoch 约束领域写入，启动先检查兼容与恢复边界，关闭先排空所有权再释放依赖。以下编号保留原协议引用编号，不表示实现已覆盖全部设计目标。

## 协议、实现与配置约束

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="contract-12-eventqueue"></a>

### 12. EventQueue

<a id="contract-121-统一事件总线"></a>

#### 12.1 统一事件总线

- 系统使用一个统一 EventQueue，而不是为 Memory 单独建立队列。
- Task、Worker、Memory、Delivery、Computer 生命周期等异步流程共享统一事件协议。
- 本地与云端协议一致，只替换 Broker Adapter。

<a id="contract-122-交付语义"></a>

#### 12.2 交付语义

- 使用 at-least-once 交付。
- 消费者通过 `event_id` 保证幂等。
- 不保证全局顺序，只按聚合键保证必要顺序，以保留多 Task 并行能力。
- 超过重试阈值的事件进入 DLQ，并通知研发人员。
- DLQ 不直接通知用户，也不阻塞其他无关事件。

<a id="contract-123-一期选型"></a>

#### 12.3 一期选型

- Broker：NATS JetStream。
- Go Client：官方 `nats.go`。
- 事件信封：CloudEvents Structured JSON。
- `CloudEvent.subject` 用作聚合键。
- Broker 通过薄适配层接入，未来可映射到 Kafka record key/partition 或 RocketMQ message group。
- EventQueue 是支撑基础设施，不在一期投入过多自研设计。

<a id="contract-124-权威运行状态与执行权"></a>

#### 12.4 权威运行状态与执行权

- Agent、Task、Run、ToolInvocation、CodeExecutionState/CodeCall/ExecutionClaim、Continuation、Approval、Outbox 和 Computer 元数据统一存入独立的运行状态库；其中 CodeExecutionState/CodeCall/ExecutionClaim 是 ToolInvocation 的内部实现表。
- 一期运行状态库为 SQLite `state.db`，未来云端实现替换为 PostgreSQL。
- EventQueue 只负责唤醒和分发，不是权威状态源。
- RunStore 的 `Claim` 返回一个不向 Agent Loop 暴露内部细节的 `RunClaim`。
- Agent Loop 只持有 `RunClaim`；RunStore 在保存 Checkpoint、进入等待状态和提交最终结果时验证执行权。
- RunClaim 内部可使用随机 claim/lease token、过期时间和版本号实现分布式互斥与故障接管，Pod 身份不进入领域模型。
- 外部工具副作用通过持久化 `invocation_id` 和 ToolExecutor 生成的 `idempotency_key` 防止重复执行；Provider `call_id` 只负责模型协议内的调用—结果关联，不能单独承担副作用幂等，也不能仅依赖 RunClaim。

<a id="contract-125-状态与事件一致性"></a>

#### 12.5 状态与事件一致性

- 运行状态数据库是唯一事实源，NATS JetStream 只承担异步通知、唤醒和分发。
- 每次领域状态变更与对应的 `event_outbox` 记录在同一个数据库事务内提交，不在数据库与 Broker 之间使用分布式事务。
- Outbox Publisher 异步发布事件到 JetStream，成功后将 Outbox 记录标记为已发布；发布失败可安全重试。
- JetStream 使用 at-least-once 交付；消费者在提交业务状态与 `event_id` 去重记录后才确认消息。
- 消费者通过数据库唯一约束或等价 Inbox 记录实现幂等，重复事件不得重复推进 Run、重复写 Memory 或重复创建 DeliveryRequest。
- 一期在 SQLite `state.db` 中实现领域表、`event_outbox` 和消费去重记录；未来迁移 PostgreSQL 时保持同一事务语义和接口。
- 面向外部渠道的 Delivery Outbox 与内部 `event_outbox` 职责分离：前者保存待投递消息，后者可靠传播领域事件。

<a id="contract-16-coding-agent-实现契约"></a>

### 16. Coding Agent 实现契约

<a id="contract-161-通用数据约定"></a>

#### 16.1 通用数据约定

- 所有领域 ID 都是不透明字符串；MVP 默认使用 UUIDv7，模型与 JavaScript 不生成 ID。
- SQLite 时间保存为 UTC 微秒整数，进程间协议使用 RFC 3339 UTC；禁止使用本地时区时间参与排序、租约或幂等判断。
- JSON 字段使用 `snake_case`；进入哈希、幂等或重放校验前必须做 canonical JSON 编码。
- 所有可并发修改的聚合根包含从 `1` 开始的 `version`，更新使用 compare-and-swap；状态机迁移失败不得静默覆盖。
- 删除默认采用保留审计信息的 tombstone/终态，不级联删除 Task、Run、ToolInvocation、Approval、Conversation 或 Artifact 元数据。
- 模型可见的 `ArtifactRef`、`TaskRef` 等引用都是不可解析字符串；只有对应 Store 可以解析。

一期使用三个 SQLite 数据库：

| 数据库 | 权威内容 | 必要设置 |
|---|---|---|
| `state.db` | Task、Run、ToolInvocation、Code Mode、Approval、Artifact 元数据、Automation、Outbox/Inbox | WAL、`foreign_keys=ON`、`busy_timeout`、版本化 migration |
| `conversation.db` | ConversationMessage、消息序号、Compaction 结果 | WAL、单 Conversation 单调序号、幂等消息 ID |
| `memory.db` | Memory 记录、证据、FTS/向量索引 | WAL、FTS5；向量索引通过接口接入 |

数据库文件、Artifact 二进制和日志位于配置的 data root 下；不得依赖进程当前目录。migration 必须前向执行、可在空库重建，并在不兼容版本时 fail-fast，不能带病启动。

`state.db` 的空库初始化可使用从前二十一个历史 migration 生成的 schema，减少重复 ALTER 引起的 schema 解析。快照对应 schema version 21，不是另一种磁盘格式；既有库照常逐版升级，快照之后的新增迁移仍逐版执行。生成文件绑定历史脚本摘要，漂移时 fail-closed；创建、版本更新和后续 migration 在同一个 immediate transaction 内。测试必须验证表/索引/触发器及初始数据与逐版重放一致、失败全回滚，以及提交前后 kill/restart。

<a id="contract-162-最小持久化模型"></a>

#### 16.2 最小持久化模型

以下是 Coding Agent 必须落库的最小逻辑字段。实现可以拆表或增加内部字段，但不得删除语义：

| 记录 | 最小字段 |
|---|---|
| `AgentConfig` | `agent_id, active_model_profile_id?, setup_status, config_version, created_at, updated_at` |
| `AuthConnection` | `connection_id, provider, auth_kind, credential_ref, account_label?, workspace_label?, status, expires_at?, last_refreshed_at?, created_at, updated_at, version` |
| `ModelProfile` | `profile_id, name, provider_kind, endpoint?, wire_api, model_id, auth_connection_id?, reasoning_effort?, context_window, max_output_tokens, generation_defaults_json, compatibility_json, capabilities_json, status, verified_at?, created_at, updated_at, version` |
| `ModelSnapshot` | `snapshot_id, profile_id, profile_version, provider_kind, endpoint_hash?, wire_api, model_id, auth_connection_id?, reasoning_effort?, context_window, max_output_tokens, generation_defaults_json, compatibility_json, capabilities_json, created_at` |
| `Task` | `task_id, agent_id, conversation_id, title, anchor_task, anchor_artifact_refs, completion_policy, status, task_epoch, created_at, updated_at, completed_at?, cancelled_at?, version` |
| `TaskInstruction` | `instruction_id, task_id, task_epoch, source_message_id?, instruction, artifact_refs, created_at` |
| `Run` | `run_id, agent_id, task_id?, parent_run_id?, root_run_id?, kind, worker_position?, status, wait_reason?, task_epoch?, depth, capability_snapshot_id, model_snapshot_id, desired_state, checkpoint_ref?, result_ref?, error_code?, started_at?, ended_at?, created_at, updated_at, version` |
| `CapabilitySnapshot` | `snapshot_id, agent_config_version, role, tool_specs_hash, skill_set_hash, data_scope, can_spawn_agents, created_at` |
| `ToolInvocation` | `invocation_id, run_id, parent_invocation_id?, provider_exchange_id?, provider_call_id?, tool_name, arguments_json, request_hash, replay_policy, idempotency_key?, status, wait_reason?, attempt_count, result_ref?, error_json?, result_ready_at?, delivery_seq?, created_at, updated_at, version` |
| `CodeExecutionState` | `execution_id, tool_invocation_id, source_ref, source_hash, input_ref?, input_hash, runtime_version, prelude_version, state, desired_state, wake_seq, processed_wake_seq, execution_epoch, claim_token?, claim_expires_at?, result_ref?, error_json?, created_at, updated_at, version` |
| `CodeCall` | `execution_id, call_key, call_id, tool_name, request_hash, tool_invocation_id, child_run_id?, status, output_ref?, error_json?, attempt_count, created_at, updated_at` |
| `CodeStoreEntry` | `execution_id, key, value_ref, value_hash, updated_at, version` |
| `Approval` | `approval_id, invocation_id, risk_type, scope, frozen_arguments_hash, status, decision_source?, expires_at?, decided_at?, created_at, version` |
| `Artifact` | `artifact_id, owner_agent_id, task_id?, run_id?, media_type, name, byte_size, content_hash, storage_key, visibility, retention_class, created_at, expires_at?, deleted_at?` |
| `ConversationMessage` | `message_id, conversation_id, sequence, role, content_json, source, source_event_id?, created_at` |
| `DeliveryRequest` | `delivery_id, message_id, channel, destination_ref, status, idempotency_key, attempt_count, last_error?, created_at, updated_at` |
| `EventOutbox` | `outbox_id, event_id, subject, type, payload_json, occurred_at, published_at?, attempt_count` |
| `EventInbox` | `consumer, event_id, processed_at` |
| `Automation` | `automation_id, agent_id, instruction, trigger_type, trigger_spec, timezone, enabled, next_fire_at?, created_at, updated_at, version, intent_privacy_epoch` |

枚举值以本文状态机为准：

- `Run.kind = main | worker`；Worker 的 `worker_position = root | child`。
- `Task.status = active | paused | completed | cancelled`。
- `Run.status = queued | running | waiting | completed | failed | cancelled`。
- `ToolInvocation.status = queued | running | waiting | succeeded | failed | cancelled | unknown`。
- `ToolInvocation.replay_policy = safe | query_before_retry | never`。未声明策略的新 Tool 必须按 `never` fail-closed，不能默认安全重放。

最低唯一约束与索引：

1. 同一 Agent 只能有一个非终态 Main Run。
2. 同一 Task 只能有一个非终态 Root Worker Run。
3. `AgentConfig.active_model_profile_id` 必须引用 `ready` Profile；ModelSnapshot 创建后不可更新。`ModelProfile(provider_kind, model_id, endpoint)` 与 `AuthConnection(provider, status)` 必须可索引查询。
4. `Run(parent_run_id, status)`、`Run(task_id, status)` 和 `Task(agent_id, status, updated_at)` 必须可索引查询。
5. 模型直接调用对 `(run_id, provider_exchange_id, provider_call_id)` 唯一；`provider_call_id` 只在一次 Provider exchange 内有效，不能单独承担 Run 级幂等。Code Mode 调用对 `(execution_id, call_key)` 唯一。
6. `ConversationMessage(conversation_id, sequence)`、`DeliveryRequest(idempotency_key)`、`EventOutbox(event_id)` 与 `EventInbox(consumer, event_id)` 唯一。
7. Outbox 必须有 `published_at IS NULL` 索引；Claim、waiting、到期 Approval、到期 Automation 必须可按下一次唤醒时间扫描。
8. Artifact 以 `content_hash` 校验内容完整性，但不能仅以内容哈希代替权限和所有权检查。

<a id="contract-163-原子事务跨库提交与不变量"></a>

#### 16.3 原子事务、跨库提交与不变量

以下事务边界不可拆开：

- 首次激活模型：已验证 ModelProfile + AuthConnection 引用 + AgentConfig `active_model_profile_id/setup_status/config_version`。SecretStore 写入先成功，再提交引用；数据库提交失败时清理未被引用的新 Credential。
- 创建 Run：读取 Active ModelProfile，生成不可变 ModelSnapshot 与 Capability Snapshot，并与 Run/必要 Outbox 原子提交；Child 使用 Root/Parent 已有 ModelSnapshot，不重新读取 Active Profile。
- `task_manage(create)`：Task + Task Anchor + Root Run + Capability Snapshot 引用 + `run_ready` Outbox。
- `task_manage(append)`：TaskInstruction + Task 状态/epoch 更新 + 新 Root Run（若需要）+ Steering/`run_ready` Outbox。
- `agent.run()` admission：CodeCall（若来自 Code Mode）+ Agent ToolInvocation + Child Run + Root 树计数 + `run_ready` Outbox。
- 任意状态迁移：领域记录 + 对应 EventOutbox；Broker 发布不参与数据库事务。
- Tool 成功：规范化结果/Artifact 元数据 + ToolInvocation 终态；提交成功后才能发送 `tool_execution_end`。Artifact 二进制在事务外写临时对象并校验哈希，文件锁保护活跃 staging；短事务重检配额/授权、原子发布并提交元数据；恢复器跳过活跃锁，清理无引用对象。
- `conversation.db` 与 `state.db` 不做分布式事务。Main 最终回复使用确定性的 `message_id/delivery_id`、幂等 Append 和可恢复提交顺序：先写 ConversationMessage，再写 DeliveryRequest，二者都成功后才推进 Main Checkpoint；任一步崩溃都由恢复器补齐，渠道发送成功只更新 DeliveryRequest，不重写语义消息。

必须始终成立：

1. EventQueue 消息可以重复或丢失后重发，但不能重复创建 Child Run、重复推进状态或重复投递用户消息。
2. 已返回“Host 已接受”的调用一定能从数据库恢复；没有持久化成功的调用视为未接受。
3. Run 与 CodeExecution 的终态不可逆；ToolInvocation 除 `unknown` 外的终态不可逆，`unknown` 只能由 Reconciler 基于外部权威证据收敛为 `succeeded/failed`。
4. 取消、暂停、恢复与接管都通过 epoch/fencing token 拒绝迟到写入。
5. 同一 `call_key` 的 `request_hash` 变化必须返回 `NON_DETERMINISTIC_REPLAY`，不能静默创建第二个逻辑调用。
6. 外部副作用不宣称 exactly-once；系统提供的是“持久化意图、幂等键、执行结果查询与不确定状态显式化”。

<a id="contract-164-统一-host-tool-error"></a>

#### 16.4 统一 Host Tool Error

Direct Tool、`tools.*`、`cua.*` 与 `agent.run()` 共用同一错误外形：

```typescript
interface HostToolErrorShape {
  code: string;
  message: string;
  retryable: boolean;
  details?: Record<string, unknown>;
}
```

JavaScript 收到带安全字段的 `Error`，其中可读取 `code/retryable/details`；不得包含密钥、内部路径、Prompt、SQL、Run ID 或原始 Provider 响应。MVP 至少支持：

| 错误码 | 语义 | 模型/脚本处理 |
|---|---|---|
| `INVALID_ARGUMENT` | Schema 或引用无效 | 修正请求，不自动重试 |
| `NOT_FOUND` | Tool/对象/Artifact 不存在或不可见 | 检索或询问，不自动重试 |
| `PERMISSION_DENIED` | Capability/Data Scope 不允许 | 不得绕过 |
| `APPROVAL_DENIED` | 用户或策略拒绝副作用 | 基于已有结果收尾 |
| `TIMEOUT` | 单次模型或工具超时 | 返回调用方决定是否重试 |
| `CANCELLED` | Parent/用户取消 | 停止派生新工作 |
| `PROVIDER_UNAVAILABLE` | 瞬态重试耗尽 | `retryable=true`，由调用方决定下一步 |
| `RESOURCE_LIMIT` | 触达高安全硬上限 | `retryable=false`，使用已有结果收尾 |
| `UNKNOWN_SIDE_EFFECT` | 外部效果无法确认 | 禁止自动重试，交 Main/用户处置 |
| `NON_DETERMINISTIC_REPLAY` | 固定调用键对应请求改变 | fail-closed，记录故障 |

`STALE_EPOCH`、`CLAIM_LOST`、`DUPLICATE_EVENT` 等只用于宿主协调与日志，正常情况下不暴露给模型；Executor 应接管或重放。

<a id="contract-165-配置与-secret"></a>

#### 16.5 配置与 Secret

配置使用一个版本化 YAML 文件，环境变量只做部署覆盖；Secret 使用环境变量引用或 OS Secret Store，不把明文写入 YAML、SQLite、Artifact、RunTrace 或日志。最小配置组：

```text
app.data_root / app.log_level
llm.provider_bridge_command / llm.provider_bridge_version
llm.active_profile_id（由首次设置生成，不要求用户手写）
llm.request_timeout / llm.catalog_cache_ttl / llm.probe_timeout
secret.backend（macOS 默认 keychain；测试为 memory）
embedding.provider / embedding.model / embedding.api_key_ref
nats.url / nats.stream_prefix
codemode.sidecar_command / codemode.runtime_version / codemode.resource_limits
computer.provider / computer.instance_profile
limits.child_concurrency / limits.max_depth / limits.max_descendants
limits.max_turns / limits.max_root_tool_calls / limits.max_code_host_calls
security.policy_file / artifact.retention / delivery.default_channel
```

- 进程配置启动时一次性校验；用户级 ModelProfile/AuthConnection 在 Setup Wizard 或设置中校验。没有 Active Profile 时进入 `setup_required`，而不是进程 crash。
- Active Profile 存在但 Credential 缺失、Provider Bridge 不兼容或 Profile 校验失败时，Agent Ingress 保持关闭并进入可修复的设置状态；数据库、迁移和诊断仍可使用。
- Agent Config 变更生成新版本和新 Capability Snapshot；进行中的 Run 不热更新权限。
- 开发测试必须提供 Fake LLM、Fake Tool、Fake Computer、内存/临时目录 ArtifactStore，不要求真实账号或 macOS VM 才能运行核心测试。

<a id="contract-166-进程启动关闭与恢复顺序"></a>

#### 16.6 进程启动、关闭与恢复顺序

桌面安装通过 `install.sh` 安装 `erii`；裸命令进入终端设置/对话。
无显式配置或 `ERII_NATS_URL` 时，启动器在独立 data-root 锁内托管一个
loopback 随机端口、文件持久化的 JetStream，随后仍按下述顺序连接 Broker
并开放 Ingress。显式配置保留外部 Broker 路径。没有 Active Profile 时仍
保持 `setup_required`，不自动选择模型。用户数据与版本化安装目录分离；
App 关闭完成后再停止本地 Broker。入口由原 `cmd/personal-agent` 改名为
唯一的 `cmd/erii`；数据库、ModelSnapshot 与协议无需迁移。
安装和组织决定见 [ADR 004](installation.md#decision-004-004--erii-installation-and-repository-layout)。

启动顺序固定为：

1. 加载并校验配置，初始化日志、Trace 与进程身份。
2. 打开 SQLite，执行 migration，校验数据库和 ArtifactStore 可写性。
3. 初始化 SecretStore，启动并校验 Model Provider Bridge；加载 Active ModelProfile/AuthConnection。
4. 若没有可用 Active Profile，开放仅含 Setup/Diagnostics 的 TUI/Web，完成首次设置后继续；不得开放 Agent Ingress。
5. 连接 NATS JetStream 并校验/创建版本化 Stream 与 Consumer。
6. 初始化 Tool Registry、Security、ComputerProvider；启动并握手 Code Mode Sidecar。
7. 启动 Outbox Publisher 与 Inbox Consumer。
8. 扫描过期 Run/Execution Claim、未发布 Outbox、未投递结果、到期 Approval/Automation 与 `wake_seq` 差异，幂等发布恢复事件。
9. 启动 RunExecutor/Delivery/Memory Curator，最后开放 Conversation/Task 渠道 Ingress。

正常关闭顺序相反：先停止接收新 InboundEvent 和新 Tool admission，再把活动执行推进到最近的可持久化边界、停止续租并保留恢复意图；随后 Drain Consumer/Outbox、请求 Sidecar 正常关闭、关闭 NATS 和数据库。正常进程退出不是用户 cancel，不得把 Task、Run 或 Child 标记为 cancelled。

<a id="contract-167-模块接口的最小边界"></a>

#### 16.7 模块接口的最小边界

首个实现至少定义并用 Fake 实现验证以下使用方接口：

- `ModelProfileService.Add/Test/Select/List`：用户控制面；只允许 `ready` Profile 成为 Active。
- `ProviderAuth.Start/Continue/Refresh/Logout/Status`：浏览器、Device Code 与 token refresh 统一为 UI-neutral 事件。
- `ModelCatalog.List/Refresh` 与 `ModelProbe.Run`：目录发现和能力验证分离，目录声明不能代替 Probe。
- `LLMProvider.Generate/Continue/Cancel`：必须接收不可变 ModelSnapshot，流式事件规范化为 Agent Loop 事件；Provider 专属消息不进入领域层。
- `SecretStore.Get/Put/Delete`：OS Credential Store 实现与测试 Fake；领域表只保存 `credential_ref`。
- `RunStore.Claim/Checkpoint/Wait/Complete/Fail`：所有写入验证 claim token 与 epoch。
- `TaskService.Search/Manage`：唯一的 Task 控制面。
- `ToolRegistry.Describe/Resolve` 与 `ToolExecutor.Execute`：所有工具调用的统一入口。
- `AgentService.StartChildRun`：只创建 Child Run，不创建 Task。
- `JavaScriptRuntime.Start/Cancel/Dispose`：遵循 6.2.4 的版本化 RPC。
- `ArtifactStore.Put/Open/Describe/Delete`：返回不透明 ArtifactRef，读取强制校验作用域。
- `ComputerProvider.Ensure/Checkpoint/Restore/Destroy` 与 `ComputerUseRuntime`：Provider 和使用协议分离。
- `EventBus.Publish/Subscribe`：实现只传 CloudEvents；领域服务不依赖 NATS 类型。
- `ConversationStore.Append/List`、`MemoryStore.Search/Apply`、`DeliveryService.Enqueue`。

接口由使用方包定义；不要先建立通用 Repository/Service 基类，也不要为了未来云端拆分引入远程 RPC。

## Changelog

| 日期 | 决策 | 理由与影响 |
| --- | --- | --- |
| 2026-10-28 | 将跨模块契约与各功能局部协议分开归属在 designs 内。 | 保留数据、权限与恢复语义及原编号，未变更模型可见协议或数据库。 |
