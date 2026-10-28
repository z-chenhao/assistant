# Developer Observability 执行检查器

在 Erii Web 检查完整执行关系、逐轮模型上下文和工具轨迹。

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

**当前实现**：Web 侧栏的可折叠 Developer 区域提供 Observability。可按 Main/Root/Child、Run 状态及 UTC 创建时间筛选并翻页；选择 Main 查看原始已提交 Task 行为回执，选择 Task 或 Worker 查看创建来源、分 epoch 的 Root/Child 树及直接 Parent/Root 导航。

**边界**：只读关系元数据和最多 240 字符的 Task 标题，不显示原始指令、工具参数/结果、凭据、模型请求或内部所有权 token。Main 创建 Task 是因果关联，Root 没有 Main parent。历史或自动化入口没有 Main 创建回执时明确说明来源未记录，不推断。每页默认 40、最多 100 条；关系分页可能从一棵树的中间开始，Parent 可直接打开。跨页状态不是冻结快照；按页读取当前已提交状态。返回 Conversation 和 Ongoing work 的链接保留，旧 Task 可能不在 Ongoing work 的近期窗口内。

**验收目标**：真实工作能从入口定位到 Main、Task、Root 和 Child；Root 不被误标为 Main 的 Child。

<a id="f-034"></a>

### F-034 · 逐轮上下文、来源与 Diff

**需求**：展开任意模型轮次查看 system/user/assistant/tool、工具定义、响应与上下文来源，并比较相邻轮次。

**当前实现**：model_generation 已保存冻结请求与结果；召回、压缩有独立记录，缺聚合查询与页面。

**边界与剩余工作**：实现完整有序消息查看、来源关联、压缩省略说明、相邻轮次新增/删除/变化展示。

**验收目标**：UI 与被冻结的 Host 请求逐项一致，工具调用/结果对应正确，来源和省略可追溯，长文本可完整查看。

<a id="f-035"></a>

### F-035 · 供应商最终请求观测

**需求**：开发者能区分 Host 冻结请求与 Bridge/SDK 转换后实际发送的请求。

**当前实现**：Bridge 会合并 system、转换工具并加入图片；当前缺完整发送边界观测记录。

**边界与剩余工作**：在最终发送边界保存受控脱敏记录，标识发送尝试与转换版本；凭据和图片 Base64 不进入普通轨迹。

**验收目标**：HTTP peer 接收的请求与观测记录对应，差异明确标记为脱敏或媒体引用；请求头凭据不可见。

<a id="f-036"></a>

### F-036 · 调用、等待、恢复与用量时间线

**需求**：开发者能看出何时执行、等待审批或 Child、失败、重新尝试、重放或复用结果，并查看已有用量。

**当前实现**：调用、trace、generation 与 usage 数据已有，CLI 只提供有限投影。

**边界与剩余工作**：聚合模型/工具时间线、状态、已有 Token/耗时；区分 Generation、dispatch attempt 和结果复用。

**验收目标**：受控中断恢复后，UI 不把复用结果显示成新外部调用；未知结果、缺失 usage 和未 dispatch 等待如实展示。

## 如何使用

登录现有 Web，展开侧栏 **Developer → Observability**。页面与 API 沿用工作区 token/session 和 Web binding 授权；不需要另行配置开发者服务或选定模型。

1. 在 Execution list 中按角色、状态和创建时间查找。时间筛选使用 UTC，起点包含、终点不包含；Apply filters 重置分页与选择，Older executions 读取下一页。
2. 打开 Main，沿 Task action timeline 查看原始创建、追加或控制回执。时间标记为来源调用的接纳时间，不冒充外部 dispatch 时间。点击 Task 查看执行树。
3. 打开 Root/Child，核对实际 Parent、Root 和 Task epoch；Parent/Root/创建者链接可相互跳转。More relationships 读取后续节点或回执，刷新页面获取最新状态。
4. Back to conversation 和 Ongoing work 返回日常工作入口。该页面没有重放、审批、取消或外部发送动作；关闭 JavaScript 仍可使用导航、筛选和分页。

`erii ops trace RUN_ID` 继续提供既有简化事件。完整逐轮 context、Diff、供应商最终请求与调用/用量时间线尚未实现，分别由 F-034/F-035/F-036 维护。

## 详细操作参考

- [运维操作详情](operations.md)

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/observability.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/observability.md)。
