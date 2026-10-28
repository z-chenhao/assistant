# 文件、命令与代码工作：测试方案

[功能与验收目标](../features/coding.md) · [技术方案](../designs/coding.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

F-026 验证四工具在真实隔离环境与统一 ToolExecutor 中的能力，不以 Fake 模型结果宣称模型开发质量。本页列维护中的验证；实际结果以本轮执行报告为准。

| 层级 / 入口 | 验证内容 |
| --- | --- |
| [environment 组件](../../internal/computer/environment/local_test.go) | 实际内核沙箱：四工具互通、重复精确编辑、宿主文件和环境变量不可见、链接与越界拒绝、超时、输出上限、二进制 Artifact、无沙箱时不回退 |
| [共享进程输出](../../internal/computer/sandbox/process_test.go) / [OCR 取消](../../internal/media/ocr/engine_test.go) | io.Copy 与后续写入不能绕过输出上限；真实沙箱启动后取消需等待退出并清理作业；原生工具继续由上行验证 SIGTERM 清理与 unknown 效应行为 |
| [SQLite 组件](../../internal/agent/sqlite/environment_test.go) | 真实状态库和审批；原生调用复用 Computer、checkpoint 和 execution claim；无 JavaScript 依赖；完成和 unknown 不重发；旧 Workspace 无运行时别名 |
| [CLI capabilities](../../tests/system/verify_capabilities.py) | 真实 red → Worker 精确修改多个文件 → bash go test → diff → Artifact；连续脏文件编辑/新建/过时哈希；审批拒绝不执行；SIGKILL 后 unknown 不重发；保留 MCP/Skill/角色回归 |
| [memory maintenance](../../tests/system/verify_memory_maintenance.py) | Reviewer 只采纳真实 bash 的精确测试命令、退出码与完整输出证据；保留独立 Run 与晋升约束 |

## 仍需独立验证

macOS Seatbelt、本轮新增的真实 Lume/SSH Guest 命令与桌面文件互通，必须在对应平台做 Live 检查；Linux 软件回归和协议 Fake 不能代替。真实模型质量、联网依赖、真实仓库导入与远端交付按实际接入单独验收。

## 执行与完成判定

沿用既有 make test/lint、system runner 与证据目录；不另建脚本框架或 golden case。状态、审批、冻结协议或取消变化需要相关 race/恢复检查。F-026 的通用开发交付目标仍保持部分完成。
