# Developer Observability 执行检查器

在 Steward Web 检查完整执行关系、逐轮模型上下文和工具轨迹。

[需求主索引](../feature-map.md) · [技术方案](../designs/observability.md) · [测试方案](../test-plans/observability.md)

## 功能说明

开发者从现有 Web 的 Developer → Observability 进入执行列表与因果关系时间线。

展开任意模型轮次查看 system/user/assistant/tool、工具定义、响应与上下文来源，并比较相邻轮次。

开发者能区分 Host 冻结请求与 Bridge/SDK 转换后实际发送的请求。

开发者能看出何时执行、等待审批或 Child、失败、重新尝试、重放或复用结果，并查看已有用量。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-031"></a>

### F-031 · Developer 导航与执行关系检查

**需求**：开发者从现有 Web 的 Developer → Observability 进入执行列表与因果关系时间线。

**当前实现**：Web Developer → Observability 提供 Trace 列表，按 Main / Root、状态和 UTC 时间筛选；Conversation 作为 Session 关联 Main 与后台工作。选中 Trace 后进入独立详情，在同一导航区查看对齐的 observation 树/时间线，右侧检查节点详情，支持键盘/原生链接、浏览器前后退与刷新。Main 的 Task 创建/管理回执作为跨 trace 因果链接；Trace 详情的 Task execution 选择器列出各 epoch 的 Root，Child 在实际 Parent Worker 的 trace 内展示。

**边界**：一个 Main Run 或一个 Task epoch 的 Root 执行树是一条 trace；Root 没有 Main parent。每页最多 100 个 observation，分页缺席的 parent 明确标记并可打开；不同分页看到当前提交状态。只读且沿用现有所有者授权。输入/输出与 Task 标题在列表中使用有界预览，节点详情和冻结上下文有完整读取入口；旧元数据 API 仍保持原含义。

**验收目标**：真实工作能从入口定位到 Main、Task、Root 和 Child；Root 不被误标为 Main 的 Child。

<a id="f-034"></a>

### F-034 · 逐轮上下文、来源与 Diff

**需求**：展开任意模型轮次查看 system/user/assistant/tool、工具定义、响应与上下文来源，并比较相邻轮次。

**当前实现**：在 Generation 的 Context Tab 内按 step 顺序打开已接纳的模型轮次，查看有序 system/user/assistant/tool 消息、工具 Schema、模型响应、压缩来源/省略记录与冻结召回。消息有可读正文和完整存储 JSON 两种视图；JSON 保留工具调用 ID、参数、结果、图片引用及原始来源。相邻轮次可查看新增、移除、变化与移动项，并打开前后完整内容。

新策略在同一逻辑输入内仅首次产生 Retrieve context；后续 Generation 的召回区通过 preparation 中的确切 `recall_step` 读取同一冻结记录。Prepare context 仍逐次记录预算检查和实际请求冻结，旧记录没有复用引用时保留原来的逐 step 查询，不推测缺失召回。

**边界**：读取 Host 冻结记录，不重新组装 context，不冒充 Bridge/SDK 最终请求。来源 kind/ref、召回 hit、summary_request/working_set、省略引用和 history_artifact 按已存记录呈现；旧记录没有细粒度来源时明确未知，不解析文本猜测来源或自动开放私有压缩归档。Diff 按保存的 source ref / tool call ID 对齐，旧消息按位置、工具按名称、重复键按出现顺序；比较存储项，不做语义相似度匹配。每页最多 40 条，正文每段最多 8192 UTF-8 字节，链接可读完全部正文与 JSON；不静默截断。待完成或失败 generation 不伪造响应，已保存的 clipped 标记在响应 JSON 中保留。

**验收目标**：UI 与被冻结的 Host 请求逐项一致，工具调用/结果对应正确，来源和省略可追溯，长文本可完整查看。

<a id="f-035"></a>

### F-035 · 供应商最终请求观测

**需求**：开发者能区分 Host 冻结请求与 Bridge/SDK 转换后实际发送的请求。

**当前实现**：新版本生产 Bridge 在 SDK 完成 system 合并、工具转换及图片投影后，在实际 HTTP fetch 边界记录独立 attempt。选中 Generation 或 Provider HTTP 节点，打开 **Requests** 查看脱敏最终请求、格式版本、UTC 时间与脱敏正文 hash。HTTP 重试各有独立 attempt，不把 generation 恢复接纳计数当网络次数。

**边界**：不保存请求头、connection、API key 或 access token；Node 与 Host 都移除已知凭据。图片像素替换为 Artifact 引用（已知时）、预览 JPEG hash、原图/预览尺寸，不保存 Base64。脱敏正文最多 256 KiB，超限保留 hash 与明确省略原因；详情按 8192 字节续读。capture 为 best-effort，旧 Bridge/历史记录、队列溢出、数据库故障或强制退出可留空；缺失不能证明没有发送。开始事件表示 fetch 尝试，响应头事件不代表模型成功或服务端业务接受；原始 SSE 响应正文不另存。

**验收目标**：HTTP peer 接收的请求与观测记录对应，差异明确标记为脱敏或媒体引用；请求头凭据不可见。

<a id="f-036"></a>

### F-036 · 调用、等待、恢复与用量时间线

**需求**：开发者能看出何时执行、等待审批或 Child、失败、重新尝试、重放或复用结果，并查看已有用量。

**当前实现**：统一 Agent、Generation、Tool、Retriever、上下文准备、审批/费用授权等待、工具执行片段、HTTP 尝试和恢复事件。节点详情展示保存的输入/输出、模型、时间信息与可用来源；generation 提供可读角色消息、工具调用卡片、完整上下文、Diff 与最终请求入口。汇总唯一已接纳 generation 的 token/cache、已有 provider operation 耗时与按币种区分的已记账费用。

**边界**：生命周期耗时含等待，不标成模型推理时间；HTTP 条仅测 fetch 到响应头，provider operation 含准备/流式读取。没有起止记录的召回/准备事件是时间点，未知耗时/usage/价格保持未知，部分 usage 标明已知记录数。恢复使用已接纳结果产生 reuse 事件，不新增 generation、usage 或 HTTP 尝试；工具 unknown 不由浏览触发重试。新工具事件携带原始 invocation ID，旧事件无该字段时不以重复的 provider call ID 猜测对应关系。

**验收目标**：受控中断恢复后，UI 不把复用结果显示成新外部调用；未知结果、缺失 usage 和未 dispatch 等待如实展示。

Developer 的 [Memory 页面](memory.md)展示真实记忆正文与来源；记忆来源中的实际 memory_manage 调用可跳转到 Trace，两者沿用同一工作台权限。

## 如何使用

登录现有 Web，点击侧栏 **Developer · Traces**。页面与 API 沿用工作区 token/session 和 Web binding 授权；不需要另行配置开发者服务或选定模型。

1. 列表页顶部提供名称、Trace ID 或模型的字面子串搜索（ASCII 大小写不敏感），全局顶栏为独立时间范围与 Refresh。角色/状态按分组单选呈现，筛选侧栏可收起（手机默认收起），按角色、状态和 UTC 创建时间组合查询；起点包含、终点不包含，支持最近 24 小时／7 天／30 天及自定义边界。条件与分页写入 URL，可逐项清除；修改条件与刷新从第一页读取。点击 Session 查看同一 Conversation 的相关 Trace。
2. 图表显示完整筛选范围的真实 Trace 时间计数；表格按创建时间、类型、名称、Input、Output、wall time、模型、Token 与已结算费用紧凑分列，ID 独立且默认隐藏，列显隐在当前浏览器保留。缺失指标保持未知，已知记录不足时标注 Partial；页脚报告完整筛选计数及当前页条数。
3. 点击名称在默认半屏的右侧非模态面板展开详情，保留底下列表的筛选与滚动位置；关闭及浏览器后退恢复列表，支持前后 Trace、全屏展开，Open full page 打开独立 `/developer/observability/{run}`。直接书签和无 JavaScript 时仍使用完整详情页。左侧 **Trace tree** 以类型颜色、真实层级连接线、分支折叠和实际耗时呈现，搜索保留匹配祖先；面板外边缘与树/正文分栏支持鼠标和键盘调宽，偏好只在当前浏览器保留，右侧为 **Observation details**；返回 Traces 保留列表条件。选择节点时保留树/时间线滚动位置；浏览器前后退恢复 URL 中的选择。Refresh 读取新提交状态，不自动轮询。
4. 节点 Preview 将类型、UTC 时间、耗时、状态及 Session/Run 放在顶部，正文依次为 Input、Output；支持 Formatted/JSON 切换、结构化 JSON 折叠与复制。部分片段或无法精确表示的数字保留原始 JSON 阅读。Generation 的 Preview 展示有界消息与接受响应，模型请求时间线折叠在下方。时间线显示响应头、首段响应数据、首段可见文本或首个工具调用、响应流终止事件，并区分等待响应头超时、响应流空闲和整轮超时。**Context** 是完整 messages / tools / response / provenance / recall 的阅读器，支持 JSON、来源与长正文连续读取。**Requests** 查看最终 HTTP 请求。存在前一轮时显示 **Compare**。HTTP 节点提供 Overview / Requests。
5. 选择 Tool、Retriever、等待或 HTTP 节点，核对各自保存的数据和时间语义。API/Page 不提供重试、重放、审批或取消动作；无 JavaScript 时原生导航、筛选和分页仍然可用。
6. 沿 Task 因果链接或 Parent 导航核对角色关系；Conversation 和 Tasks 返回日常工作入口。长时间 Task 的继续会创建新的 epoch/trace。

旧 `/developer/context` 与带 `?run=` 的书签自动转向规范 Trace 详情，保留 step、section、item、format 与正文偏移；原 API 不变。

这是安装所有者授权下的敏感诊断。原有冻结 context 与新增脱敏最终请求均保留在本地 state.db，沿用其保留及加密备份边界；页面/API no-store，不新增公开访问、导出或第三方发送。观测故障不决定执行或重放。旧元数据 API、CLI `steward ops trace RUN_ID` 保留。

当前对齐 Langfuse 的 Session/Trace/Observation 数据与浏览体验；不依赖 Langfuse 服务，未接入 OTel 导出、Prompt Management、Dataset/Evals、聚合 Agent Graph 或全文搜索。Prompt 继续由项目文件/Git 管理，模型效果评测继续离线维护。

## 详细操作参考

- [运维操作详情](operations.md)

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/observability.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/observability.md)。
