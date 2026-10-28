# 开发、验证与交付：技术方案

[功能说明](../features/engineering.md) · [测试方案](../test-plans/engineering.md) · [产品测试方案](../test-plans/product.md)

## 当前架构

产品逻辑与测试分属明确模块。Makefile 和现有 system runner 提供本地入口，CI 自动执行这些入口；发布流水线独立准备原生包和草稿发布。

```mermaid
flowchart TD
    S["源码与固定依赖"] --> M["Makefile / system runner"]
    M --> P["PR 软件检查"]
    M --> E["扩展回归 / race"]
    S --> R["显式原生打包 / 草稿发布"]
```

### 运行时边界（F-039）

```mermaid
flowchart TD
    UI["Web / TUI / CLI"] --> Q["只读查询：分页、按 ID、内存投影"]
    UI --> A["单一 Actor / Agent Loop"]
    BOOT["后台依赖准备与重试"] --> READY["成功后发布能力与开放 ingress"]
    READY --> A
    A --> T["统一 Tool Executor / 受信 builtin 目录"]
    T --> TASK["Task 生命周期判断"]
    T --> CODE["CodeMode VM / 重启 / 重放协调"]
    CODE --> T
    T --> SQL["SQLite：claim / fencing / 原子提交"]
    A --> SQL
    SQL --> OUT["持久化结果与 Outbox"]
    STAGE["Artifact 流 / 哈希：事务外，文件锁"] --> SQL
    A --> PREVIEW["临时流式预览"]
    A --> TRACE["有界生命周期轨迹 / 写失败诊断"]
```

- `agent.RunFailure` 仅表示终态失败已提交，Web 可继续处理新请求；基础设施错误不能冒充该类型。CLI 可用 `errors.Is` 检查模型 cause。
- `app.WithEvents` 先交给 UI 生命周期，再在可取消的后台连接循环准备 broker/能力/恢复扫描。入口在准备成功前关闭；失败保留脱敏日志并重试。能力准备仅缓存成功，不为重试生成第二条执行管线。
- `task/transitions.go` 计算继续/停止/完成约束，适配器在同一事务读取权威事实、应用 CAS 并创建或 steering Root。`tool/codemode/execution.go` 定义其消费的持久边界并拥有 VM 重建循环；`agent/sqlite` 实现 SQL、claim 和调用接纳。
- `tool/builtins.go` 是 Host 名称、权限、内部可见性和分发家族的唯一目录。外部 provider 不能注册任意 Host 工具。冻结 ToolSpec 与角色裁减规则保持原有字节和语义。
- `agent.OperationError` 保留 operation、分类和可解包 cause；对外 Error 文本不拼接原始错误内容。召回服务不可用与状态库故障分开分类。
- `message_update` 是进程内预览，无持久序号/重放保证；完整 GenerationResult、Tool 结果、checkpoint 和业务事件仍由原有事务保存。生命周期 trace 有单次写入期限，写失败报告 `TRACE_UNAVAILABLE`。这不替代 F-031/F-034/F-035/F-036 的观测产品。
- 内部 Phase/Role/RunStatus 使用命名类型，序列化字符串不变。Provider Bridge 的图片、assistant/tool 消息转换拆成有名函数；不修改 QuickJS prelude/runtime 指纹。

迁移：不增加 schema 版本，不重写历史数据或已冻结请求。Artifact staging 文件名继续沿用生成的平面 key；文件锁需要支持 advisory flock 的本地文件系统，存储根的 OS 账户仍为信任边界。升级须先停止旧版本进程，再启动新版本；旧回收器不识别新 staging 锁，禁止混跑旧、新二进制操作同一 data root。

## 当前流程

用户问题 → 登记需求 → 检查代码与相关约束 → 实现与成比例验证 → 更新功能/设计/测试及状态 → 审查交付。F-038 不授予发布授权。

## 协议、实现与配置约束

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="contract-14-技术选型"></a>

### 14. 技术选型

| 层次 | 已确认选型 |
|---|---|
| 核心后端 | Go |
| 部署形态 | 一期模块化单体，未来可拆服务 |
| 模型 Provider | 独立 Node.js Model Provider Bridge：复用 Pi AI 行为；支持本地 OpenAI-compatible 与 OpenAI Sign in with ChatGPT |
| Code Mode | 独立 Node.js JavaScript 沙箱 |
| Agent Tool | `builtin__agent__run`、Code Mode `agent.run()` 与 `tools.*`、`cua.*` 共用 ToolInvocation 管线；复用 QuickJS 和父 ToolInvocation 的内部恢复协议 |
| 本地隔离环境 | Lume macOS VM + Guest 内 Cua Driver |
| Memory/Conversation 权威存储 | 分离的 SQLite 数据库 |
| 事件队列 | NATS JetStream |
| 事件协议 | CloudEvents Structured JSON |
| 本地/云端执行抽象 | ComputerProvider |
| Artifact 传递 | `artifact_ref` |

<a id="contract-141-go-工程组织"></a>

#### 14.1 Go 工程组织

- 一期只使用 `cmd/ + internal/`，暂不创建 `pkg/`；只有未来确实需要对外 SDK 时才增加公共包。
- `cmd/erii` 是一期唯一的 Go 主进程入口。
- `internal/app` 是不包含业务逻辑的 Composition Root，负责依赖组装、启动和关闭。
- 接口由使用方所在模块定义；SQLite、NATS、飞书、macOS VM 等实现放在对应模块的具名子包中。
- 不建立通用的 `ports/`、`adapters/`、`controller/`、`service/`、`repository/` 大目录。

```text
cmd/erii/
web/                     # TypeScript 展示层；构建产物嵌入 Go Web Adapter
provider-bridge/          # Pi AI Provider/Auth Bridge；无 Tool/Computer 权限
internal/
  app/
  config/
  interaction/
  agent/
  task/
  conversation/
  memory/
  context/
  llm/
  secret/
  skill/
  tool/
  security/
  computer/
  artifact/
  delivery/
  event/
  observability/
runtime/
  javascript/            # QuickJS Code Mode Sidecar；无模型凭据
```

具名实现子包示例：

```text
memory/sqlite
conversation/sqlite
llm/providerbridge
secret/keyring
event/nats
computer/lume
computer/cua
interaction/feishu
tool/mcp
```

<a id="contract-142-模块职责边界"></a>

#### 14.2 模块职责边界

- `interaction`：渠道接入、身份绑定，并将输入归一化为 `InboundEvent`。
- `agent`：Agent Actor、Main/Worker Agent Loop、Task 创建决策、Run 树生命周期、创建权限、深度与 Worker 调度；Code Mode 通过 AgentService 只创建 Child Run，不建立 Workflow 模块。
- `task`：持久化工作对象、状态及其关联 Run，不承载执行循环。
- `conversation`：保存权威消息历史。
- `memory`：长期记忆、召回与 Memory Curator。
- `context`：为每次 Turn 组装、裁剪、冻结和压缩上下文，不拥有来源数据。
- `llm`：管理 ModelProfile/ModelSnapshot、Provider Bridge、模型目录、能力 Probe 与固定主模型调用；一期不做动态模型路由。
- `secret`：封装 OS Credential Store 与测试用内存实现；只返回受控 Credential，不把 Secret 暴露给领域存储或模型。
- `skill`：Skill 的发现、读取、候选、审核和生命周期。
- `tool`：工具注册、发现、ToolInvocation 与调用；包括 AgentToolExecutor、Computer Tool Executor、Code Mode 的内部 CodeExecutionState/CodeCall Ledger 和 Sidecar 桥接。
- `security`：在产生副作用前执行风险识别、授权与确认裁决。
- `computer`：管理逻辑 Computer、运行 Instance、Checkpoint、恢复和并发仲裁。
- `artifact`：管理产物生命周期及 `artifact_ref`。
- `delivery`：管理 `DeliveryRequest`、Outbox 和渠道发送。
- `event`：定义 CloudEvents 信封、发布订阅接口及 NATS JetStream 实现。
- `observability`：承接日志、指标、Trace、RunTrace 和 DLQ 告警，不参与业务决策。
- `config`：加载和校验进程配置。
- `app`：组装模块并管理进程生命周期，不承载业务逻辑。

<a id="ci-continuous-integration-and-delivery"></a>

## Continuous integration and delivery

CI is the automation that runs software checks. Unit, integration and end-to-end
are test levels; using CI does not turn a unit test into an end-to-end test.

<a id="ci-pull-requests-and-main"></a>

### Pull requests and main

[CI](../../.github/workflows/ci.yml) runs one `Ubuntu checks` job. The existing
required status name is preserved. One checkout and dependency setup serve:

1. `make test lint`: Go unit/component checks, Node runtime contracts, frontend
   freshness/types, the real Chromium contract and retained infrastructure tests.
2. `make test-e2e`: nine deterministic product flows through actual CLI/UI,
   stores, brokers and runtimes. See the [coverage map](../test-plans/product.md#testing-system-coverage).

Documentation-only PRs run the local link/index check without installing Go,
Node or Chromium. The `Ubuntu checks` job still executes and reports a status;
there is no workflow-level path filter that leaves branch protection pending.
Unknown paths, code changes, empty diffs and main pushes use the full PR path.

Failures stop the job; failed or cancelled work does not count as a pass. The
process owner forwards cancellation, allows fixture cleanup and kills remaining
owned process groups. Results and failed logs are uploaded with source/run
identity. Tests never need an online model account.

The job has a 20-minute stop boundary. It is not a latency claim. This replaces
38 job instances and their setup/build/upload/join overhead; GitHub-hosted queue
time and wall-time improvements must be measured on the new workflow. Local
results do not establish remote CI performance. There is no custom changed-file
selector, secondary required gate, benchmark comparator or cross-job build bundle.

<a id="ci-extended-regression"></a>

### Extended regression

[Extended regression](../../.github/workflows/regression.yml) runs weekly and via
manual dispatch. It executes all retained deterministic system gates, then the
complete Go race inventory. It uploads failures as well as passing results and
has a 60-minute outer limit. A failure remains a defect to investigate; it cannot
be waived into a release pass. Run it before releases and broad protocol/state
changes, or use the corresponding focused gates during development.

The tests remain deterministic software regressions. No Golden benchmark,
semantic Judge, paid model call or clause-audit inventory runs in either workflow.

<a id="ci-delivery"></a>

### Delivery

[Native packages](../../.github/workflows/release.yml) remains separate. Manual
runs or explicit version tags build native archives and checksums on supported
Linux/macOS architectures. Version tags create an unpublished draft release for
human inspection. An ordinary PR neither deploys nor publishes a release.

Real-account login, external-site compatibility, macOS Computer and real-model
quality require their appropriate explicit live validation. A green PR does not
claim those checks ran. Tests, model evaluation and release authorization have
separate outcomes.

<a id="contributing-repository-ownership"></a>

### Repository ownership

| Location | Purpose |
| --- | --- |
| `cmd/erii`, `internal/` | Application and Go tests beside their implementation |
| `provider-bridge`, `runtime/javascript`, `guest-browser`, `web` | Product runtimes, frontend and their local tests |
| `tests/system/` | Deterministic CLI/UI/process tests and shared fixture support |
| `tests/harness/` | Unit/component regressions for retained test and setup utilities |
| `tests/fixtures/` | Small synthetic protocol, transport and coding samples |
| `scripts/` | Installation, dependency setup, packaging and local maintenance |
| `docs/` | Current architecture, operations and development guidance |

The full Golden Case benchmark, real-model evaluation runners, scores and
historical clause-audit ledgers are maintained outside the product checkout.
They are not PR prerequisites. See [evaluation boundaries](../test-plans/product.md#testing-agent-evaluation).

## Changelog

| 日期 | 决策 | 理由与影响 |
| --- | --- | --- |
| 2026-10-28 | F-039：终态 Run 失败与基础设施错误分开；UI 先开放，依赖准备只缓存成功。 | 保留控制面以修复故障；不使用跳过失效能力的降级注册，避免静默更改冻结能力。 |
| 2026-10-28 | F-039：Task 规则、CodeMode 协调从 SQLite 适配器移回所属模块；统一 Host 目录与显式 Artifact 分发。 | 保持一个事务权威和一条工具管线，避免以通用 service/ports 层替代现有领域。 |
| 2026-10-28 | F-039：流式预览不逐片落库，生命周期 trace 保留有界写入与诊断。 | 避免每个 token 竞争权威 writer；完整请求/结果仍持久，丢失的临时预览不能作为执行失败或重试依据。 |
| 2026-10-28 | F-039：命名状态类型、Bridge 转换函数和分类错误。 | 保持线协议、快照与数据库兼容；只解包内部 cause，避免日志/界面泄露私人内容。 |
| 2026-10-28 | 把贡献流程、工程组织与 CI 说明归入同一功能及方案。 | 不维护额外 CONTRIBUTING/CI 正文；保留软件测试与独立模型评测边界。 |
