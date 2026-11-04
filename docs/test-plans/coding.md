# 文件、命令与代码工作：测试方案

[功能与验收目标](../features/coding.md) · [技术方案](../designs/coding.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

F-026 验证四工具在真实隔离环境与统一 ToolExecutor 中的能力，不以 Fake 模型结果宣称模型开发质量。本页列维护中的验证；实际结果以对应版本的 PR 或 CI 证据为准。

| 层级 / 入口 | 验证内容 |
| --- | --- |
| [environment 组件](../../internal/computer/environment/local_test.go) | 实际内核沙箱：四工具互通、重复精确编辑、宿主文件和环境变量不可见、链接与越界拒绝、超时、输出上限、二进制 Artifact、无沙箱时不回退 |
| [共享进程输出](../../internal/computer/sandbox/process_test.go) / [OCR 取消](../../internal/media/ocr/engine_test.go) | io.Copy 与后续写入不能绕过输出上限；真实沙箱启动后取消需等待退出并清理作业；原生工具继续由上行验证 SIGTERM 清理与 unknown 效应行为 |
| [SQLite 组件](../../internal/agent/sqlite/environment_test.go) | 真实状态库和审批；原生调用复用 Computer、checkpoint 和 execution claim；无 JavaScript 依赖；完成和 unknown 不重发；旧 Workspace 无运行时别名 |
| [CLI capabilities](../../tests/system/verify_capabilities.py) | 真实 red → Worker 精确修改多个文件 → bash go test → diff → Artifact；连续脏文件编辑/新建/过时哈希；审批拒绝不执行；SIGKILL 后 unknown 不重发；保留 MCP/Skill/角色回归 |
| [memory maintenance](../../tests/system/verify_memory_maintenance.py) | Reviewer 只采纳真实 bash 的精确测试命令、退出码与完整输出证据；保留独立 Run 与晋升约束 |

## 需求场景矩阵

| 需求 | 可观察行为与验证方式 | 覆盖边界 |
| --- | --- | --- |
| [F-026](../features/coding.md#f-026) | 隔离 fixture 仓库含未提交修改，两个 Task 并行准备、修改和验证；检查修改集分离、原文件保留、准备失败隔离、重启复用与证据真实。远端受控 peer 验证幂等/unknown；真实 GitHub 授权及模型代码质量独立验证，沿用现有 coding 系统入口。 | 本地 CLI、SQLite、内核 scope 与进程重启回归已接通。远端交付与 Live/效果评测待补 |

## 仍需独立验证

真实 Lume/SSH Guest 命令与桌面文件互通仍须在相应平台验证。真实模型质量、联网依赖、真实仓库导入与远端交付按实际接入单独验收。

## 执行与完成判定

macOS 本机 Seatbelt 回归运行真实四工具和 Task 目录/恢复场景，覆盖原生解释器发现、超时子进程清理及拒绝沙箱外和独立沙箱的信号。

沿用既有 make test/lint、system runner 与证据目录；不另建脚本框架或 golden case。状态、审批、冻结协议或取消变化需要相关 race/恢复检查。F-026 的通用开发交付目标仍保持部分完成。

本地 Task 目录通过 `verify_capabilities.py --scenario task-code-environments` 驱动真实 CLI、Task、四工具、审批和 Artifact。该工作负载使用 16K 测试模型窗口以保留五项调用和确切回执，独立的小窗口裁决不变。`--scenario task-code-recovery` 在发布前 SIGTERM 与 SIGKILL，并在发布后 SIGKILL。重启比较持久 Task 位置和源脏内容，证明未发布可重备，已发布不重新导入。两个场景进入既有 Ubuntu checks，使用同一证据上传与失败门禁。

Go 回归验证 Main/Task 沙箱访问边界、Child 继承同一 Task、精确审批、准备失败隔离、链接及无效 UTF-8 文件名拒绝、非 UTF-8 补丁省略且保留原字节摘要、空文件和正文相对补丁、报告 Task 归属及 Forget 屏障。真实远端 PR 和模型判断仍需单独验收。

`task-code-environments` 在实际 Task 工作副本生成后注入规范化的成功 watch fixture，经所有者 CLI 绑定 observe 模式，验证 `report-pr` 保存仓库 ID、确切 head、观察时间和原本地证据。错误 Task、错误 head 与撤销绑定必须拒绝，原报告和已保存的历史关联可继续打开。SQLite 测试在报告完成 staging 后注入同 head 刷新或 Forget，确认提交不留下 Artifact。真实远端发布与修订等价性不由此场景证明。
