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

**当前实现**：Web 无此 Tab；已有领域记录与简化 ops trace。

**边界与剩余工作**：实现列表、筛选、Main 创建 Task 的关联和 Root/Child 树，保留返回对话或任务的导航。

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

当前可用 `erii ops trace RUN_ID` 读取简化事件；它不提供完整逐轮上下文。Web 尚无 Observability 页面。

已确认的产品方向是在现有 Web 增加可折叠 Developer 区域，首个 Tab 为 Observability。目标操作为选择执行 → 展开关联 Main/Task/Root/Child → 选择模型轮次 → 查看上下文、工具、响应及相邻轮次差异。

## 详细操作参考

- [运维操作详情](operations.md)

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/observability.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/observability.md)。
