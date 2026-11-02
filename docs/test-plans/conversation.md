# 自然语言对话与共享循环：测试方案

[功能与验收目标](../features/conversation.md) · [技术方案](../designs/conversation.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-003](../features/conversation.md#f-003) | 验证输入去重、Steering、工具续轮、单 Main 所有权与回复恢复。通过真实对话入口启动受控慢工具，同时提交新问题与 Task 干预，检查响应、调用关联及最终交付；覆盖满负载、审批等待、重启及迟到结果。 | 真实 SQLite/Actor 及 CLI 已有回归；实际模型延迟与效果仍需独立验证，不以脚本模型得出性能结论 |

## 层级与关键路径

覆盖同源输入去重、模型生成中追加、工具提交后中断、已提交回复复用与并发 Main fencing。原始历史与模型工作上下文分别断言。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_main.py](../../tests/system/verify_main.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_terminal_input.py](../../tests/system/verify_terminal_input.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

现有入口仍需在后续相关实现变更时按风险运行；需求范围变化时同步扩展场景。

## 持续响应与恢复验证

[responsive_test.go](../../internal/agent/sqlite/responsive_test.go) 使用真实 SQLite、Actor 和 ToolExecutor 验证慢工具仍运行时先回复、数据工具槽位占满时查询 Task、等待审批的新输入、模型费用等待不取消已接纳调用、费用拒绝后的重启恢复不重试未知副作用、完成/unknown 结果接续，以及协议迁移原子性。既有 Loop/Tool 测试继续验证冻结生成、所有权丢失、取消、权限和调用结果核实。

[verify_main.py](../../tests/system/verify_main.py) 通过实际 CLI 持有已接纳的工具调用，在释放前提交第二条消息并观察回复；读回唯一 Main、pending_tools、调用身份及运行状态，再验证释放后的结果和 SIGKILL 后恢复。调用暂停点位于事务外，不以锁住数据库伪造工具耗时。观察断言小于 5 秒；结果仅证明脚本模型和本机机制，不构成真实模型 SLA。

验证命令与证据目录：

```sh
go test ./internal/agent/... ./internal/tool/... ./internal/llm/...
go test -race ./internal/agent/... ./internal/tool/...
python3 tests/system/runner.py --gate main --gate tools --gate tasks --evidence evidence/f003-core-03
```

测试结果由对应执行日志与 runner 报告保存；全量回归中的执行沙箱/OCR 平台失败和未运行检查必须单列，不能按 F-003 的目标链路通过而宣称全仓通过。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="main-loop-evidence-and-limits"></a>

### Evidence and limits

Run `make test`, `make test-race`, `make lint` and the system runner. Commands and
coverage are in the [product test plan](product.md); requirement status is in the
[Feature Map](../feature-map.md). `verify_main.py` owns isolated
launch, readiness, concurrent CLI input, drive, direct SQLite readback, nine
SIGKILL boundaries, SIGTERM, profile switching and cleanup. Its explicit
`-test-bridge main` and `-test-main-hold` hooks are never selected by production
configuration. `evidence/main.json` includes frozen requests/results and final
history, ToolInvocation, usage and delivery rows. The local/account harnesses
also execute this path through actual Node/Pi against owned HTTP fixtures.

GC-01/09/10 Main-only L0 fragments use versioned inputs and scripted Fake
responses. Reports keep their full Cases `not_applicable`, with no semantic
score. They prove protocol assertions, not real-model quality, live account
eligibility or current restaurant information. These M1 fragments alone do not
establish Security/Approval, Worker/Child or JavaScript Durable Replay; use the
[feature map](product.md) for those implemented paths and gates.
Current Runs use their frozen compaction policy; an irreducible context still
returns a context-length error. [Memory maintenance](../features/memory-context.md)
documents compaction, retained originals and privacy. Live account inference remains a separate
L2 requirement, and Device Authorization remains deferred by ADR 001.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
