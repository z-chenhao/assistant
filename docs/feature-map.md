# Erii Feature Map

本页是产品需求的主索引：记录每一个可迭代需求的简述和完成状态，链接到功能说明。功能说明分别关联技术方案和测试方案。测试命令与 CI 编排继续由 [Testing](test-plans/product.md) 管理。

最近盘点：2026-10-28，依据当前工作区实际代码（包含已有未提交变更）与现有回归入口进行静态核对；本次没有重新执行所有功能测试，不表示真实模型、线上账户或平台兼容性验收通过。

[产品架构总览](designs/product-overview.md) · [文档维护规则](#文档维护规则) · [功能模板](features/_template.md) · [技术方案模板](designs/_template.md) · [测试方案模板](test-plans/_template.md)

## 状态定义

| 状态 | 是否完成 | 含义 |
| --- | --- | --- |
| 已完成 | 是 | 本行限定范围已有非测试入口和实际调用链，并有相关回归入口；条件和证明范围见功能说明及测试方案 |
| 部分完成 | 否 | 已有局部实现，但目标用户流程仍有明确缺口 |
| 未实现 | 否 | 需求方向已确认，目标入口或闭环尚未接通；拥有底层数据不算功能完成 |
| 待定义 | 否 | 未来需求已登记，使用范围和验收条件尚需明确 |

完成状态只属于本行需求，不自动扩展为整个模块、全部平台或所有业务场景完成。配置依赖、刻意限制、待实现内容和未验证项应分别说明。

当前共 40 项需求：21 项已完成、10 项部分完成、7 项未实现、2 项待定义。此汇总随下表一起维护。

## 需求索引

| ID / 详情 | 需求 | 简要描述 | 是否完成 / 状态 |
| --- | --- | --- | --- |
| [F-001](features/model-connections.md#f-001) | 本地模型连接与选择 | 用户能连接 OpenAI-compatible 端点，测试后显式选择活动模型。 | 是 · 已完成 |
| [F-002](features/model-connections.md#f-002) | ChatGPT 登录与跨平台凭据 | 用户能登录、重新认证和退出 ChatGPT 账户，凭据可安全持久化。 | 否 · 部分完成 |
| [F-003](features/conversation.md#f-003) | 可恢复的 Main 对话 | 用户能连续对话、追问和纠正要求，短时工作在 Main 中完成。 | 是 · 已完成 |
| [F-004](features/tasks.md#f-004) | Task 生命周期与继续工作 | 用户能把耗时工作交给后台，之后通过对话追加、暂停、继续、取消或结束。 | 是 · 已完成 |
| [F-005](features/tasks.md#f-005) | 执行中的有意义进度 | 用户主动查看时能了解当前阶段、摘要与子任务完成情况。 | 否 · 部分完成 |
| [F-006](features/delegation.md#f-006) | 可递归的 Child Worker | 复杂工作可由 Worker 拆成独立子任务，并由父级汇总。 | 是 · 已完成 |
| [F-007](features/tools-approvals.md#f-007) | 工具、审批与未知副作用处理 | 用户能确认具体外部动作；重复请求或重启不能盲目重复副作用。 | 是 · 已完成 |
| [F-008](features/codemode.md#f-008) | 可恢复的 JavaScript 编排 | Agent 能用标准 JavaScript 串联工具、处理并发并显式输出结果。 | 是 · 已完成 |
| [F-009](features/memory-context.md#f-009) | 自动记忆形成与语义归并 | 后台从用户事实与反馈形成记忆，能处理纠正、重复和过时信息。 | 否 · 部分完成 |
| [F-010](features/memory-context.md#f-010) | 按作用域召回记忆与对话 | 每轮或按需检索可访问的 Memory 和原始 Conversation。 | 是 · 已完成 |
| [F-011](features/memory-context.md#f-011) | 长期上下文压缩 | 长对话保留目标、决策、约束和待办，省略内容可追溯。 | 否 · 部分完成 |
| [F-012](features/artifacts-media.md#f-012) | 可重新打开的文件产物 | 用户能上传、保存、搜索、下载交付物，引用具有访问边界。 | 是 · 已完成 |
| [F-013](features/artifacts-media.md#f-013) | 用户图片到模型视觉输入 | 用户上传图片后，选定视觉模型能实际收到授权图片内容。 | 否 · 部分完成 |
| [F-014](features/artifacts-media.md#f-014) | 音视频与文档内容处理 | 用户交付音视频或文档后，Agent 能获取适合任务的内容和来源。 | 否 · 部分完成 |
| [F-015](features/automation.md#f-015) | 一次性与周期自动化 | 用户可设定明确时区的 at/cron 计划，并修改或取消。 | 是 · 已完成 |
| [F-016](features/automation.md#f-016) | 阈值状态与安静通知 | 周期观察在首次进入阈值区间时通知，其余成功检查保持安静。 | 是 · 已完成 |
| [F-017](features/channels.md#f-017) | 终端交互 | 用户能完成模型设置、交代工作、查历史和状态。 | 是 · 已完成 |
| [F-018](features/channels.md#f-018) | 基础 Web 工作台 | 用户能登录本地 Web，使用 Conversation、Ongoing work、Settings、附件和审批。 | 是 · 已完成 |
| [F-019](features/channels.md#f-019) | Web 实时状态与回复更新 | 用户阅读或编辑草稿时仍能接收状态和回复更新，且草稿不会丢失。 | 否 · 未实现 |
| [F-020](features/channels.md#f-020) | 可维护的飞书交互与通知 | 已绑定个人身份能接收消息、附件和审批，并维持长期可用。 | 否 · 部分完成 |
| [F-021](features/intelligent-ui.md#f-021) | 富组件从生成到交互的闭环 | 正常回复能产生适当的受控组件，失败可退回语义文本。 | 否 · 部分完成 |
| [F-022](features/skills-experience.md#f-022) | 受版本控制的 Skill 读取 | Agent 可发现可用方法，再按需读取正文和资源。 | 是 · 已完成 |
| [F-023](features/skills-experience.md#f-023) | 执行后的自动经验闭环 | 用户纠正、恢复失败或重复成功能进入候选、验证和晋升流程。 | 否 · 部分完成 |
| [F-024](features/integrations.md#f-024) | 受限本地 MCP 工具接入 | 操作员可把允许的本地 stdio 工具加入 Agent 工作面。 | 是 · 已完成 |
| [F-025](features/integrations.md#f-025) | 可认证的真实服务接入 | 用户能连接实际数据源与服务，支撑研究、日程、监控等具体工作。 | 否 · 未实现 |
| [F-026](features/coding.md#f-026) | 代码理解、修改与交付 | 用户能委派仓库理解、修改、验证，并获得可审查交付物。 | 否 · 部分完成 |
| [F-027](features/computer-use.md#f-027) | 配置型 macOS Computer Use | Agent 能在明确配置的隔离 Guest 中观察和操作应用与浏览器，并恢复逻辑 Computer。 | 是 · 已完成 |
| [F-028](features/operations.md#f-028) | 本地诊断与加密恢复 | 操作员能检查健康、导出受控诊断、备份并隔离恢复数据。 | 是 · 已完成 |
| [F-029](features/operations.md#f-029) | 用量记账与配置型费用授权 | 操作员能查看用量，并按明确价格元数据控制账户费用。 | 是 · 已完成 |
| [F-030](features/capability-setup.md#f-030) | 统一能力设置与诊断体验 | 用户能在设置中看见记忆、embedding、MCP、Skills、代码工作区及 Computer 是否已连接，并完成配置验证。 | 否 · 未实现 |
| [F-031](features/observability.md#f-031) | Developer 导航与执行关系检查 | 开发者从现有 Web 的 Developer → Observability 进入执行列表与因果关系时间线。 | 否 · 未实现 |
| [F-034](features/observability.md#f-034) | 逐轮上下文、来源与 Diff | 展开任意模型轮次查看 system/user/assistant/tool、工具定义、响应与上下文来源，并比较相邻轮次。 | 否 · 未实现 |
| [F-035](features/observability.md#f-035) | 供应商最终请求观测 | 开发者能区分 Host 冻结请求与 Bridge/SDK 转换后实际发送的请求。 | 否 · 未实现 |
| [F-036](features/observability.md#f-036) | 调用、等待、恢复与用量时间线 | 开发者能看出何时执行、等待审批或 Child、失败、重新尝试、重放或复用结果，并查看已有用量。 | 否 · 未实现 |
| [F-032](features/evals.md#f-032) | Web Evals 入口 | 后续可从 Developer 查看评测结果，并关联对应执行。 | 否 · 待定义 |
| [F-033](features/database.md#f-033) | Web Database 检查入口 | 后续可检查执行、对话、记忆和文件元数据，理解数据关系。 | 否 · 待定义 |
| [F-037](features/installation.md#f-037) | 安装、首次启动与显式部署 | 安装、检查、前台启动及自定义部署，并保持持久数据。 | 是 · 已完成 |
| [F-038](features/engineering.md#f-038) | 标准开发与显式发布流程 | 固定依赖、构建、风险分层验证及可审查交付。 | 是 · 已完成 |
| [F-039](features/engineering.md#f-039) | 运行时工程基础重构 | 故障隔离、短事务、只读分页、加密召回复用、明确模块与工具分发边界。 | 是 · 已完成 |
| [F-040](features/engineering.md#f-040) | Go 模块边界与可读性重构 | 领域持有适配器和接口类型，入口分阶段，长事务函数分职责，保持现有协议。 | 是 · 已完成 |

## 文档组织

```text
docs/feature-map.md                 需求索引、稳定 ID、是否完成
docs/features/<feature>.md          解释、需求、边界、如何使用、验收目标
docs/designs/<feature>.md           最新架构、最新流程、实施缺口、决策 changelog
docs/test-plans/<feature>.md        验收场景、测试层级、已有入口、缺失覆盖
```

一个功能可以包含多项有独立完成状态的需求，用稳定 ID 和锚点定位。每项新增需求必须落在明确功能文档中；不要把独立待办藏在 changelog 或泛化“模块完成”中。

## 文档维护规则

1. **登记需求**：先在本页添加稳定 ID、简述、状态和功能链接。新需求分配新 ID；已有 ID 不复用，不因改名而更换。
2. **解释需求**：功能文档说明用户问题、行为、使用条件、已有能力、剩余缺口和验收标准。尚无入口时明确“未实现”，不写成可执行教程。
3. **维护技术方案**：架构和流程始终更新为当前有效版本。区分当前运行实现、已确认待落地方案和待讨论建议；正文不堆叠旧版本。
4. **记录取舍**：每次改变架构或流程，在技术方案 changelog 追加日期、决策、比较过的选项、理由、影响及关联需求/PR/既有决策。历史不覆写；后续用新记录标记替代。现状建档日期不冒充历史实现日期。
5. **维护测试方案**：按需求 ID 对齐验收场景、层级、已有测试、未覆盖项及必要 Live 检查。计划与执行报告分开；测试结果保存到既有 evidence/报告流程，不能把“有测试代码”写成“测试已通过”。
6. **结束迭代**：实现、功能用法、最新技术方案、测试计划和本页状态一起更新。只有目标链路完成并有对应验证证据才能把新迭代标为已完成；范围缩小时明确记录，不能删掉缺口后宣称完成。
7. **保持边界**：跨功能契约维护在 designs 内，历史决定归入对应技术方案的 Changelog，安装/操作/排障归入 features，统一测试与 CI 边界分别归入 test-plans/product 和 designs/engineering；不再保留独立 guides、ADR 或技术基线体系。真实模型效果评测仍遵循 [测试与评测边界](test-plans/product.md#testing-agent-evaluation)。

复杂功能写独立场景矩阵；简单功能的测试方案可简短引用已有回归入口，无需为文档层级另建测试框架。
