# Intelligent UI：技术方案

[功能与需求](../features/intelligent-ui.md) · [测试方案](../test-plans/intelligent-ui.md) · [主索引](../feature-map.md)

## 当前架构

具体需求是保持 OpenIntelligentUI 原生组件与设计系统作为基础，按同一生成式契约扩展 HTML/SVG、计算器、模拟与交互控件。当前消费者为 Main 的展示工具、Web 会话、非 Web 文字交付和重启重放。审批、冻结快照与消息来源去重继续使用既有领域机制。

上游固定为 `CopilotKit/OpenIntelligentUI@8391a94c1c3939c679e40ddd7bfc08a9ef690e70`。复用 DataTable、BarChart、PieChart、PlanCard、MeetingTimePicker、ExportOverlay 与设计系统源码；只适配本地导入、资源边界、减少动态效果偏好、导出脚本字符串转义，真实的选择接纳/恢复，以及 Steward 的展示样式。时间选择成功文案为 Time Selected，不把一次选择误称为外部日历预约。

没有引入上游 Python Agent、CopilotKit 服务端或第二套任务状态机。Steward 的持久执行与交付是机制；具体组件选择、紧凑布局和按需加载是当前策略。Web 适配器为私有实现；本次不稳定化通用跨产品 UI 框架。

## 工具与组件契约

模型输入为 `title`、完整 `fallback` 与 `components:[{name,props}]`。上游原生名称与参数如下：

| name | props |
| --- | --- |
| Table | title、columns、rows、source |
| barChart / pieChart | title、description、data:[{label,value}] |
| plan_visualization | approach、technology、key_elements |
| scheduleTime | reasonForScheduling、meetingDuration、timeSlots、可选 title |
| generateSandboxedUi | initialHeight → placeholderMessages → css → html → jsFunctions → jsExpressions |

Table 保留上游 12 列、200 行和来源要求；图表限制 200 点、有限数值，饼图拒绝负值与非空零总和。候选时间必须显式提供且不得重复。Host 另外限制字符串、工具输入大小、组件总数和可回答组件数量。这些是资源与交付所需约束，不是另一套展示语言。

高频工具声明仅描述名称与 props 入口，完整的逐组件 Schema 在宿主边界校验；Skill 与模板提供精确参数，避免把整个目录反复塞进有限上下文。旧 markdown/table/progress/form 与顶层 html/css/javascript 输入明确拒绝。普通对话仍按 Markdown 展示，这不构成 UI 工具输入。

## 持久化与交付

`ui_widget` 保持按工具 invocation 唯一保存不可变输入。成功产物在 Main 回复准备事务中绑定确切消息，Delivery 冻结投影，GET 不创建产物。Web 仅在冻结内容与绑定工具产物精确匹配时，按产物顺序分泡，再显示单独最终文字；原快照不被改写。

scheduleTime 的候选选择通过既有 `/ui/form` 接纳边界转成普通用户意图。身份由确切消息、组件位置与冻结参数生成，客户端只能选择候选下标或拒绝；字段、越界选择、身份漂移及已接受答案的变更均拒绝。并发重试与丢失回执复用同一来源，刷新/重启从已接纳内容恢复，不制造审批或外部预约。无脚本页面提供同一候选选择的固定宿主表单。

Web 更新保留同一投影的 React 根和子组件，变化的响应元数据原位重渲染；原生选择完成不销毁邻近沙箱、重置其局部控件或覆盖宿主草稿。

## 扩展协议与沙箱

generateSandboxedUi 保留上游六字段及 classic-script 函数/表达式语义。宿主产生完成态 `type:open-generative-ui` activity，content 使用 initialHeight、generating、css/cssComplete、html/htmlComplete、jsFunctions/jsFunctionsComplete、jsExpressions/jsExpressionsComplete；HTML 在 activity 中为片段数组。正式 activity 仍在工具与回复提交后交付；生成过程另有可撤销的受控预览，见下节。不承诺跨调用 DOM 增量修改。

沙箱注入上游 THEME_CSS、SVG_CLASSES_CSS 与 FORM_STYLES_CSS，并复用宿主的字体与语义主题令牌。原生组件采用紧凑标题、中性表面与暖色图表，移除演示用装饰和重复外层卡片；共享控件皮肤限定在沙箱根类，不另建组件协议。Inter 字体从已有本地资源内嵌为 data 字体，CSP 仅额外允许 font-src data:，不开放字体网络访问。主题显式跟随宿主，生成 CSS 只补具体布局。ResizeObserver 报告实际 body 高度，宿主检查有限数值并限制在 50–4000px，避免过去 1000px 截断引起的嵌套滚动。跟随最新消息时，高度变化继续保持底部；正在操作组件时不抢滚动。展开仍属于宿主，可用 Escape 退出。正常渲染不显示 Text version 入口；失败时显示一次完整文字替代，无脚本渠道保留既有替代。宿主提问操作行使用可换行的 flex 布局与明确 gap，沙箱模板按共享 actions 组排列按钮。

宿主在生成 CSS/HTML 之前安装 `Websandbox.connection.remote.sendPrompt({text})` 与 `openLink({url})` 适配器。只接受确切 iframe Window、opaque origin、固定协议和有界参数。sendPrompt 提出可编辑草稿，用户显式发送；已有草稿和不确定发送的来源不被覆盖。openLink 只提出无凭据 HTTPS 链接，用户在宿主打开。沙箱不能直接访问工具、宿主存储或审批；状态和已声明动作由下述宿主边界处理。

CSP sandbox 与 iframe allow-scripts 保持 opaque origin；禁止父 DOM、宿主存储、外部网络、子框架、表单动作与媒体权限，初始化先禁用 Worker/WebRTC 构造器。直接导航仅返回文字替代。CDN importmap 与外部库不在本地适配器承诺内；需要的原生图表依赖固定打包。旧自定义 HTML 与旧组件只保留历史文字替代，不执行历史代码；不删除或重写数据库记录。

选择保留现有执行/持久化而不整体引入 CopilotKit/LangGraph，以免产生双重调度、内存检查点或第二个模型入口。流式预览位于冻结最终交付之外，不新增持久协议。


## 流式预览与正式交付

具体需求是让 Web 用户在模型生成 UI 时看到逐步到达的内容，覆盖 Main 直接调用与 Code Mode 嵌套调用。消费者仅为当前已授权的 Web 会话；非 Web 渠道、历史消息、审批和持久重放仍使用原有正式产物。预览是私有、可丢弃的适配器状态，不是工具完成回执、表单身份或稳定公共协议。

直接调用时，Provider 为工具参数增量附带调用身份；Bridge 在客户端通过内部 RPC 参数显式启用 `ui_preview` 时，才将 `builtin__intelligent_ui_show` 的参数按大小/时间合并为 `ui_delta`（旧 v1 客户端不接收新事件；旧服务端可继续使用暂存后的预览；该选项不进入冻结 GenerationRequest），其他工具参数、Code Mode 源码、推理与 Worker 内容不进入浏览器。Loop 对 JSON 前缀做有界解析，仅采用完整的数组项；当前组件对象及 HTML/CSS 字符串可逐步展开。表格行、图表点和卡片仍由原 catalog 校验；缺失的标题、来源等元数据暂为空，不补造数据。表单/时间选择仅在完整参数通过校验后显示；所有预览控件均 inert，不可聚焦、提交、导出或触发动作。

Code Mode 不解析或提前执行流式 JavaScript。既有沙箱实际调用 UI 工具、宿主验证并完成 staging 事务后，context 中的进程内观察者通知同一个预览通道，无需等待 Code Mode 返回或最后的文字回复。该通知不改变 ToolInvocation 终态提交顺序。恢复等待中的 Main 时，在当前 claim 下只读重新投影已成功且未绑定回复的 UI 产物，不重新执行工具。staging 成功而工具结果提交失败时，预览可撤销，恢复仍沿用原幂等产物。

Web 复用 `/api/conversation/stream`，新增 `ui-preview` 快照事件。会话与绑定校验、定期撤权检查及写超时与文字流相同。关联键由宿主匿名化，浏览器收不到 Run/Invocation ID 或原始参数。当前预览最多 16 份、序列化快照至多 512 KiB；单份 JSON 前缀至多 128 KiB。大小、合并频率和默认高度是当前资源策略，不稳定化为模型输入或跨渠道契约。慢客户端直接读取最新快照，重连可重建当前预览，不积压事件队列。

原生组件复用已有 React catalog 和主题。生成式 HTML/SVG 从同源只读 `/ui/preview` 读取当前匿名会话的内存快照；服务端用已有 HTML 解析器仅保留展示节点并清理脚本、事件属性、导航和嵌入内容，再放入 opaque origin、无 `allow-scripts` 的 iframe；预览响应独立 CSP 禁止脚本、外部网络、表单与 base 变更，仅允许共享内联主题和 data 字体；不放宽宿主 CSP，也不使用会继承宿主样式限制的 srcdoc。独立页面导航只得到文字标题，预览结束后的地址返回 404。JS 函数/表达式、动作声明及正式提交身份不会进入预览。正式交付后仍由现有沙箱安装 Websandbox、恢复状态并执行通过校验的完整脚本，既有宿主确认与审批保持不变。

取消、失去执行权、失败、等待或退出执行时撤下预览；迟到事件不能复活已结束的会话。临时参数预览在生成结束时撤下，由实际 staging 的内容接续。`DeliverReply` 成功后发送提交关联键，前端等权威历史中的对应正式消息出现再移除预览，避免重复气泡。浏览历史且暂缓加载最新消息时撤下已提交预览，沿用原新消息提示。断线、隐藏页面与撤权清空本地临时视图；最终交付和已保存交互状态继续可恢复。所有预览增量与生命周期事件均不写 RunTrace、Checkpoint、Conversation、Delivery 或 Outbox。

保留完整 JSON 校验、执行 fencing、资源边界与禁用交互属于正确性所需复杂性；拒绝第二套执行器、部分 JavaScript 执行、任意工具参数透传、持久化每个 token 和通用跨产品 UI patch 协议。当前只开放已有组件契约与受控展示观察点，不稳定化解析器、DOM 差量或预览存储格式。

## 交互状态与 Steward 扩展

具体消费者为原生表格/图表/表单、卡片轮播和生成式沙箱。它们需要刷新恢复、向 Main 提供当前选择，以及使用现有工具结果更新视图。展示产物不可变、输入快照不可变、执行与审批继续归 Run/ToolInvocation 是不变量；组件目录、过滤规则、最近八份上下文与展示模式是当前私有策略。冻结的 `ui_state` 明确说明内容来自已保存组件，可能属于旧话题，不表示活动页面或当前请求主题；不从更新时间推断用户焦点。此说明只进入新接纳的输入快照，历史快照保持原文。

保留六个上游名称和原始 props，新增五个显式前缀名称：`steward.lineChart`、`steward.scatterChart`、`steward.form`、`steward.cards`、`steward.action`。它们沿用 name/props 容器，不声称是上游原生组件。折线/散点图使用有界数据；表单字段为 text、textarea、number、date、email、select、multiselect、boolean；卡片图片只接受当前授权 Artifact，链接仅 HTTPS。完整逐字段校验在 UI 工具边界，类型/候选/范围也在回答接纳时复查。

`ui_view_state` 保存组件私有 JSON 和独立的小型模型上下文，按确切产物引用和组件位置授权，使用版本 CAS。状态至多 16 KiB、模型上下文至多 4 KiB，限制深度与节点数。保存不会生成 Run 或改写冻结投影。标准输入和 output 按稳定键自动恢复；自定义绘制状态使用 getState/setState。客户端将同一组件写入串行化，保存冲突保留本地值并显示错误，不覆盖其他窗口。新用户消息接纳前采集当前控件、排空待保存数据；接纳事务把最多八份、总计 16 KiB 的当前上下文冻结进 `main_ui_context`，作为该输入的 scoped evidence 进入既有 AgentMessage。后续更改不影响已经接纳的生成，历史记录与 system 规则不被重写。

`steward.action` 冻结动作 ID、用户标签、工具名、默认参数及可选绑定。绑定只通过 JSON Pointer 读取同一产物内其他组件的已保存状态，在接纳事务中填入指定参数；不支持表达式、任意引用或客户端传入工具参数。解析后的参数再次按该 Main 的冻结工具 Schema 校验，并随请求不可变保存，重试不重新绑定后来的状态。宿主按钮接纳普通用户意图，验证当前 Main 工具目录与精确参数，关联 `ui_action_request`；既有 Actor、模型、ToolExecutor、审批、取消和重放继续执行。API 只回传该输入步骤范围内、工具名与规范化参数匹配的真实持久结果；没有执行时返回明确失败，不能把 Main 的普通文字当成工具结果。丢失 HTTP 回执复用同一来源，刷新继续查询既有请求。轮询读到的持久回执优先于迟到的 POST 成功或错误；网络确认不能覆盖已知结果、误示重试或回退到 pending。生成代码的 callTool 只提出同一产物内已声明动作，由宿主按钮确认发起；iframe 消息无法伪造用户点击或审批。该请求机制不承诺模型必然执行所提工具，也不暴露通用工具 RPC、任意参数或执行权。

表格支持数值排序、筛选和选择；同一产物内表格/图表共享分类选择与查询，图表可按范围探索。表单草稿自动保存，已接纳回答从现有渠道来源账本恢复；无脚本页面复用同一 typed Answer 校验和原生提交。卡片采用横向轮播和选择。宿主菜单集中筛选、重置、CSV、SVG、PNG 及适用的 HTML 快照导出；CSV 处理公式前缀，图形在本地导出，不加载远程库。HTML 导出为当前展示快照。

展示容器保留同一 DOM/React 根，提供 inline、fullscreen、原生 dialog 和悬浮视图；模式变化不重新挂载 iframe。浮层复用共享菜单，表单复用共享 Select/控件皮肤；关闭和 Escape 恢复焦点，移动端保持触控尺寸。页面只保留内容、控件和必要状态，不展示功能介绍、实现说明、权限教程或重复文字版本。

迁移 45 新增独立状态、输入上下文和动作请求表，不修改现有消息、交付或调用记录；旧二进制不能打开升级后的库。这里的复杂性用于并发、来源归属、恢复和真实工具结果关联；拒绝直接从 iframe 调用提供商、按组件复制执行器，或把任意 JS/组件内部状态稳定化为公共 API。当前扩展与桥接均为 Steward 私有契约，后续组件只按真实复用需求提炼。

## 升级与完成判定

旧 `steward-result` 文本协议不再解析，普通文字不会产生组件。不热改旧快照，不为旧多段 Prompt 提供恢复分支；升级前结束或取消旧执行，使用新 Run。保留历史数据，不重写已接受请求或不可变投影。新 schema 数据库不能由旧二进制打开。

UI 是用户交付而非 Task 完成证明。进度只是已知事实摘要；表单和 Widget 提问均不批准外部动作。非 Web 渠道和无脚本页面保留语义替代。真实模型的工具选择、审美和数据解释质量需独立评测。

## 代码与依赖

- [UI 工具和契约](../../internal/delivery/ui/widget.go)
- [持久化与消息绑定](../../internal/agent/sqlite/ui_widgets.go)
- [Web Sandbox](../../internal/interaction/web/widgets.go)
- [React Widget](../../web/src/widgets.tsx)
- [内建 Skill](../../internal/skill/builtin/intelligent_ui/SKILL.md)
- [系统回归](../../tests/system/verify_intelligent_ui.py)

<a id="contract-42-统一的渠道无关输出"></a>

#### 4.2 统一的渠道无关输出

Main 的语义文字、UI 工具的展示产物与完整文字替代通过同一 Conversation/Delivery 生命周期交付。Web 提供原生组件与隔离交互，飞书与 TUI 保留可读替代；具体外部动作继续使用原审批管线。

## 组件呈现与全局控件

OpenIntelligentUI 名称/参数和持久交互契约保持；Field/Input/Card、图表容器和提示框采用 shadcn/ui 的使用子集，控件映射现有字体与语义主题。全局 Base UI Select/Combobox/Menu/Button 同时服务设置、任务、开发者页面与会话组件。原生表格补齐数字对齐和选中/悬停，图表使用轻量网格、无轴线刻度和紧凑提示框；表单的日期/数字字段仅在组件内部空间足够时并排。页面导航、内容列、字体字号和气泡不由组件库接管。

沙箱保留现有权限与协议，共享控件样式对齐宿主；不会把无权限生成代码变成宿主 React 组件或开放网络库加载。
