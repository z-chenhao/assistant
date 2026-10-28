# 开发、验证与交付：测试方案

[功能说明](../features/engineering.md) · [技术方案](../designs/engineering.md) · [产品测试方案](product.md)

## 验收场景

F-038：固定依赖和构建入口可用；文档改动只运行适用检查；代码/未知变更运行完整 PR 路径；失败和取消保留失败状态与清理；原生打包不自动发布正式版本。

## 已有入口

[工程辅助回归](../../tests/harness/test_project_layout.py)、[Makefile](../../Makefile)、[PR CI](../../.github/workflows/ci.yml) 与 [扩展回归](../../.github/workflows/regression.yml)。运行方式及边界见产品测试方案。本次文档迁移不宣称这些流水线已重新执行。

## 工程基础重构回归

F-039：`internal/agent/sqlite/foundation_test.go` 验证流暂停时状态查询与跨进程回收、Web Run 失败后的下一条请求、流式预览无逐片状态写入、trace 写失败诊断和内部错误 cause。`internal/app/runtime_test.go` 验证实际能力依赖修复后可重试且只发布成功 registry。Task/CodeMode 相关既有暂停、Child、重放、未知副作用和 SIGKILL 测试必须继续通过；使用 race 检查受影响并发边界。

## Go 模块化重构验收（F-040）

| 范围 | 必须保持的行为 / 已有入口 |
| --- | --- |
| 包和接口边界 | `go test ./...` / `go vet ./...` 验证调用方编译；Computer 新适配器实现 LifecycleStore；App 不暴露 SQLite DTO 和执行访问器。 |
| CLI 语法与生命周期 | `cmd/erii/command_test.go` 独立列出合法/非法形式，非法命令在创建状态前拒绝；已有 metadata/control/barrier 用例；startup/main/account/operations system 场景驱动实际 CLI。 |
| Computer 事务 | `agent/sqlite/computer_test.go`、`computer_use_test.go` 通过新适配器验证过期 Code owner、重复计划、恢复/取消、checkpoint 与 fenced event；该跨模块集成测试留在事务权威处。 |
| 运维只读报表 | `agent/sqlite/operations_test.go` 验证真实状态库的计数、trace 和脱敏字段；operations system gate 验证实际 CLI。 |
| Task 接纳与执行 | 既有 tasks/task_control/task_completion 测试验证 receipt 重用、epoch、continue 新 Root、暂停/取消与失败 rollback；拆 helper 不新增事务。 |
| Web 和模型设置 | web package tests 与 channels system gate 验证认证、CSRF、JSON/form/multipart、模型选择、账户与关闭；HTTP 路由/字段不变。 |
| 能力组装 | `app/runtime_test.go` 验证失败不发布部分 registry、修复后可重试；core 工具/CodeMode/Child 场景验证冻结能力与真实调用路径。 |

跨模块变更运行既有 `make test lint` 和相关 core system gates；Computer/Task claim 与取消相关场景运行定向 race。环境导致的未执行、失败和跳过独立报告，不以编译成功代替行为验证。不为拆文件新增测试框架。
