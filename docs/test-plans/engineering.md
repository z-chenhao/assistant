# 开发、验证与交付：测试方案

[功能说明](../features/engineering.md) · [技术方案](../designs/engineering.md) · [产品测试方案](product.md)

## 验收场景

F-038：固定依赖和构建入口可用；文档改动只运行适用检查；代码/未知变更运行完整 PR 路径；失败和取消保留失败状态与清理；原生打包不自动发布正式版本。

## 已有入口

[工程辅助回归](../../tests/harness/test_project_layout.py)、[Makefile](../../Makefile)、[PR CI](../../.github/workflows/ci.yml) 与 [扩展回归](../../.github/workflows/regression.yml)。运行方式及边界见产品测试方案。实际运行结果以对应 PR 或 CI 证据为准。

## 工程基础重构回归

F-039：`internal/agent/sqlite/foundation_test.go` 验证流暂停时状态查询与跨进程回收、Web Run 失败后的下一条请求、流式预览无逐片状态写入、trace 写失败诊断和内部错误 cause。`internal/app/runtime_test.go` 验证实际能力依赖修复后可重试且只发布成功 registry。Task/CodeMode 相关既有暂停、Child、重放、未知副作用和 SIGKILL 测试必须继续通过；使用 race 检查受影响并发边界。

[渠道观察器回归](../../tests/harness/test_channels.py) 使用真实 SQLite WAL 连接，在两次查询之间提交 Run 完成和新投递，验证等待条件使用一致快照，并验证成功、查询异常均关闭连接。系统测试中的连接关闭继续保留事务提交/回滚语义。

调度查询优化复用 `internal/agent/sqlite/scheduler_test.go` 的默认容量、跨进程额度、过期所有权和公平性回归，以及 capacity/scheduling system gates。容量 gate 在同一持久化目录执行三批各 1,000 个 Child，记录逐批耗时并检查唯一身份、既有并发上限、预设业务失败和租约回收；不新增不稳定的绝对时间断言。

Race 入口在 SQLite 分片之前分别运行 Bridge 与诊断记录器包，避免压力分片挤占 RPC / 100 ms 诊断写入契约的 CPU 时间。测试断言、生产时限与 `-race -count=1 -timeout=15m` 保持；harness 核对完整入口和两个分组入口都覆盖全部包且无遗漏或重复。失败日志尾部直接出现在 CI 输出中，被取消的兄弟分片不覆盖原始失败摘要；完整日志继续留在 artifact。

## Go 模块化重构验收（F-040）

| 范围 | 必须保持的行为 / 已有入口 |
| --- | --- |
| 包和接口边界 | `go test ./...` / `go vet ./...` 验证调用方编译；Computer 新适配器实现 LifecycleStore；App 不暴露 SQLite DTO 和执行访问器。 |
| CLI 语法与生命周期 | `cmd/steward/command_test.go` 独立列出合法/非法形式，非法命令在创建状态前拒绝；已有 metadata/control/barrier 用例；startup/main/account/operations system 场景驱动实际 CLI。 |
| Computer 事务 | `agent/sqlite/computer_test.go`、`computer_use_test.go` 通过新适配器验证过期 Code owner、重复计划、恢复/取消、checkpoint 与 fenced event；该跨模块集成测试留在事务权威处。 |
| 运维只读报表 | `agent/sqlite/operations_test.go` 验证真实状态库的计数、trace 和脱敏字段；operations system gate 验证实际 CLI。 |
| Task 接纳与执行 | 既有 tasks/task_control/task_completion 测试验证 receipt 重用、epoch、continue 新 Root、暂停/取消与失败 rollback；拆 helper 不新增事务。 |
| Web 和模型设置 | web package tests 与 channels system gate 验证认证、CSRF、JSON/form/multipart、模型选择、账户与关闭；HTTP 路由/字段不变。 |
| 能力组装 | `app/runtime_test.go` 验证失败不发布部分 registry、修复后可重试；core 工具/CodeMode/Child 场景验证冻结能力与真实调用路径。 |

跨模块变更运行既有 `make test lint` 和相关 core system gates；Computer/Task claim 与取消相关场景运行定向 race。环境导致的未执行、失败和跳过独立报告，不以编译成功代替行为验证。不为拆文件新增测试框架。

开源治理验收：`make lint` 检查 GitHub workflow；HTML 图片链接回归防止 README 引用失效，正式 Ghost 图标通过真实 Chromium 首帧像素核对；startup gate 实际打包、解压、安装并核对项目/Web/Go notices。`make audit` 记录当前漏洞库结果，网络失败与漏洞发现均不能记为通过。远端分支清理需要保存原 SHA、合并证据或所有者明确退役授权、经独立恢复验证的 Git bundle 和带 lease 的删除记录；管理权限不足应明确报告。

手动执行 native packages 需实际通过四个平台的启动契约、安装诊断、打包和重新下载后的 checksum 校验；Linux runner 必须安装 Bubblewrap，不能跳过 doctor 的缺失依赖断言。Actions 更新固定完整提交，并核对官方输入/输出兼容性；手动分支构建仍不得创建或发布 release。

## 当前验证限制

容量门禁的模型元数据持久化偶发失败根因尚未确认，追踪证据见 [PR #118](https://github.com/z-chenhao/steward/pull/118)。后续通过不等于该问题已修复；再次出现时保留具体失败位置、上下文及存储等待证据，不放宽生产时限、写入或恢复保护。

文档检查复用 `scripts/check_docs.py`：页面需可从 README 导航到达，本地链接有效，且遵循当前主题布局、稳定文件名和非日志式章节规则。对应 harness 覆盖违规文档的 CLI 失败与真实协议日期、代码示例的合法使用。内容准确性和有效约束仍须人工审查。
