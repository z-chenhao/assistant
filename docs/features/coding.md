# 文件、命令与代码工作

[需求主索引](../feature-map.md) · [技术方案](../designs/coding.md) · [测试方案](../test-plans/coding.md)

## 功能说明

Agent 默认能够在专属隔离环境里读取、新建、精确修改文件，运行命令、验证工作并生成可审查的 Artifact。四工具属于文件/执行层，Task、Memory、Artifact 等项目能力继续保留。

## 需求与验收

<a id="f-026"></a>

### F-026 · 代码理解、修改与交付

**当前实现**：默认 `builtin__read/bash/edit/write`，支持新建文件、连续修改脏文件、精确内容哈希校验、通用命令、限量输出和文件 Artifact。Main/Worker 共用统一审批与重放；配置 Computer 时与桌面浏览器共用 Guest。

**边界与剩余工作**：F-026 保持部分完成。仓库导入、依赖供应、真实模型开发质量与远端 PR/服务授权仍依赖具体环境和接入；本地默认沙箱离线，不会自动提供 VM 或浏览器。

**验收目标**：真实多文件修改完成理解、修改、测试和 diff 交付，保留用户内容；命令批准、取消和未知结果保持可追溯，不重复执行未知副作用。

## 如何使用

四工具默认存在。默认目录是数据目录旁的 `<app.data_root>.computer`，首次调用创建；自定义只需设置执行范围：

```yaml
execution:
  root: /absolute/private/agent-workspace
  toolchain_roots: [/absolute/read-only/compiler-install]
```

工具链可省略。工作区需独占、权限 0700，与应用数据、Skills、MCP 和可信运行时分开，不放凭据。Linux 需要系统 Python 3、bash、bubblewrap 及 user namespaces；macOS 本地执行需要系统 Python 3 和 Seatbelt。`erii doctor` 检查执行依赖。配置已有 macOS Computer 时无需 execution，四工具改用同一 Guest。

先 read 项目 AGENTS.md 和文件，再 edit（当前 before_hash、唯一 old_text、new_text）。write 创建时省略 before_hash；覆盖时必须提供当前哈希。用 bash 执行搜索、Git 和测试；每次在工作区开始。bash/edit/write 都会请求具体动作批准。读文件时指定 `as_artifact:true` 保存完整产物，再用 Artifact publish 选为交付内容；二进制读取自动生成 Artifact。

## 升级

删除旧 workspace 配置，按需设置 execution；旧 audit/edit/protected 不再是配置项。升级前先用旧版本排空旧 Workspace Run 和待决副作用，新版本不为旧工具提供别名。详见 [迁移边界](../designs/coding.md#默认设置与迁移)。
