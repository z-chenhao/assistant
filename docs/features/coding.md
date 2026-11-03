# 文件、命令与代码工作

[需求主索引](../feature-map.md) · [技术方案](../designs/coding.md) · [测试方案](../test-plans/coding.md)

## 功能说明

Agent 默认能够在专属隔离环境里读取、新建、精确修改文件，运行命令、验证工作并生成可审查的 Artifact。四工具属于文件/执行层，Task、Memory、Artifact 等项目能力继续保留。

## 需求与验收

<a id="f-026"></a>

### F-026 · 代码理解、修改与交付

**需求**：用户能委派仓库理解、修改和验证，每项代码任务保留明确工作目录、修改集、真实验证证据与远端交付关联，结果可供审查。

**当前实现**：默认 `builtin__read/bash/edit/write` 支持新建文件、连续修改脏文件、精确内容哈希校验、通用命令、限量输出和文件 Artifact。Main/Worker 共用统一审批与重放；配置 Computer 时与桌面浏览器共用 Guest。

**边界与剩余工作**：按 Task 准备 checkout、保留修改集及关联远端交付的流程尚未完成。首版面向一个实际仓库，可采用独立 checkout/worktree，保护用户原始脏文件，准备失败只影响本任务。交付包含可访问 diff、实际验证命令/结果与成果引用；联网依赖和 PR 授权依赖 F-025。目录隔离不等于安全隔离，沿用执行沙箱、具体审批和 unknown 语义；本地默认沙箱离线，不自动提供 VM 或浏览器。仓库导入、依赖供应和真实模型开发质量仍需验证。

**验收目标**：真实多文件修改完成理解、修改、测试和 diff 交付；两个任务的修改互不污染，保留用户原有未提交内容；失败/重启后可识别原目录与交付，验证失败如实展示；命令批准、取消和未知结果可追溯，不重复执行未知副作用或重发结果未知的 PR 创建操作，链接生成不等于交付完成或用户验收。

## 如何使用

四工具默认存在。默认目录是数据目录旁的 `<app.data_root>.computer`，首次调用创建；自定义只需设置执行范围：

```yaml
execution:
  root: /absolute/private/agent-workspace
  toolchain_roots: [/absolute/read-only/compiler-install]
```

工具链可省略。工作区需独占、权限 0700，与应用数据、Skills、MCP 和可信运行时分开，不放凭据。Linux 需要系统 Python 3、bash、bubblewrap 及 user namespaces；macOS 本地执行需要系统 Python 3 和 Seatbelt。`steward doctor` 检查执行依赖。配置已有 macOS Computer 时无需 execution，四工具改用同一 Guest。

先 read 项目 AGENTS.md 和文件，再 edit（当前 before_hash、唯一 old_text、new_text）。write 创建时省略 before_hash；覆盖时必须提供当前哈希。用 bash 执行搜索、Git 和测试；每次在工作区开始。bash/edit/write 都会请求具体动作批准。读文件时指定 `as_artifact:true` 保存完整产物，再用 Artifact publish 选为交付内容；二进制读取自动生成 Artifact。

## 升级

删除旧 workspace 配置，按需设置 execution；旧 audit/edit/protected 不再是配置项。升级前先用旧版本排空旧 Workspace Run 和待决副作用，新版本不为旧工具提供别名。详见 [迁移边界](../designs/coding.md#默认设置与迁移)。
