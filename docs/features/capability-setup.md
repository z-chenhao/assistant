# 能力连接与就绪引导

让用户发现、配置、验证和修复可选能力。

[需求主索引](../feature-map.md) · [技术方案](../designs/capability-setup.md) · [测试方案](../test-plans/capability-setup.md)

## 功能说明

用户能在设置中看见记忆、embedding、MCP、Skills、代码工作区及 Computer 是否已连接，并完成配置验证。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-030"></a>

### F-030 · 统一能力设置与诊断体验

**需求**：用户能在设置中看见记忆、embedding、MCP、Skills、代码工作区及 Computer 是否已连接，并完成配置验证。

**当前实现**：模型设置已有界面；其他能力主要通过 YAML 与外部准备启用，尚无统一工作台。

**边界与剩余工作**：定义能力配置/就绪投影及设置页面；不增加模型可见的第二套能力发现 API。

**验收目标**：用户能识别未配置、不可用与已就绪，完成真实连接和修复；新设置不热改旧 Run 快照。

## 如何使用

目前按各能力指南编辑配置并准备依赖；可通过模型诊断与相关控制命令定位问题。

统一能力页面尚不可用，不存在可直接点击的连接向导。本功能落地后将在 Settings 提供入口，具体控件和配置写入方式待实施设计。

## 详细操作参考

- [MCP、Skills 与工作区约定](../designs/integrations.md)
- [记忆维护操作](memory-context.md)
- [Computer 生命周期](../designs/computer-use.md)

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/capability-setup.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/capability-setup.md)。
