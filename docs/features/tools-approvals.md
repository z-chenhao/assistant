# 统一工具执行与审批

统一校验、执行和恢复工具调用，冻结具体外部动作的审批。

[需求主索引](../feature-map.md) · [技术方案](../designs/tools-approvals.md) · [测试方案](../test-plans/tools-approvals.md)

## 功能说明

用户能确认具体外部动作；重复请求或重启不能盲目重复副作用。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-007"></a>

### F-007 · 工具、审批与未知副作用处理

**需求**：用户能确认具体外部动作；重复请求或重启不能盲目重复副作用。

**当前实现**：Direct Tools、嵌套工具、Child 与 Computer 共用 Executor；调用与审批持久化，unknown 走核实路径。

**边界与剩余工作**：审批只约束具体冻结动作；无法可信核实的 unknown 必须保留，不承诺外部 exactly-once。

**验收目标**：验证拒绝/过期/失效审批不 dispatch、取消 fencing、崩溃窗口及 query-only 恢复。

## 如何使用

工具名使用 `builtin__task_search`、`builtin__artifact_manage` 这类完整 snake_case 名称；MCP 使用 `mcp__browser_use__click_button`，保留 server 与工具名内部的单下划线。旧名称不作别名，升级前需完成旧 Main 和未决外部效果核实；具体兼容边界见[命名规范与升级说明](../designs/tools-approvals.md#contract-61-工具命名规范)。

在终端或 Web 查看待审批动作及最终参数，明确批准或拒绝。CLI 使用 `erii approval list` 和 `erii approval approve APPROVAL_ID FROZEN_ARGUMENTS_HASH`。

`erii approval reconcile` 核实未知结果。修改要求后未执行的旧审批会失效；批准 Task 不代表授权全部子动作。

## 详细操作参考

- [工具与审批操作](#tools-tool-execution-and-approval)
- [交付与回执协议](../designs/channels.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="tools-tool-execution-and-approval"></a>

## Tool execution and approval

Every Direct Tool enters `tool.Executor`: persisted provider response → frozen
admission → input validation → security/approval → bounded dispatch → normalized
result commit. MCP stdio, progressive Skills and the controlled coding fixture
use this path; see [capability operations](../designs/integrations.md). [Code Mode](../designs/codemode.md),
[Child Runs](../designs/delegation.md) and the configured [Computer adapter](../designs/computer-use.md) use
the same pipeline. No implicit Fake is enabled.

`approval list` shows the local user's approval requests, complete final arguments,
frozen hash, status and expiry. To decide the displayed action:

```sh
erii approval list
erii approval approve APPROVAL_ID FROZEN_ARGUMENTS_HASH
# Or: erii approval deny APPROVAL_ID FROZEN_ARGUMENTS_HASH
erii chat work
erii approval reconcile
```

These are local user control commands. The principal comes from the private data
root's Agent identity; neither a model nor tool arguments can select it.
[Web/Feishu channels](channels.md) authenticate the principal before calling the
same decision boundary. IDs, hashes and provider keys do not enter model-visible tool schemas.
Approval is bound to one invocation, principal and request hash. A matching
callback can repeat after completion; a conflicting decision fails. Expiry,
denial and steering fail closed. New Main input invalidates every unexecuted
approval in that Main Run; Task append does the same for its active Root. This
includes approval granted before dispatch. A changed action requires a new
invocation and approval. Already dispatched effects cannot be revoked by steering.

| Invocation transition | Meaning |
| --- | --- |
| queued → running | Frozen arguments, policy, key and accepted Outbox event already committed; dispatch checks approval and resources atomically |
| queued/running → waiting(approval/resource/retry_backoff) | Persisted wake condition; executor and Run workers are released |
| waiting → running | Approval decision/expiry, available capacity, or due transient retry |
| running → succeeded/failed | Independent normalized result committed before end event; final batch delivery retains original call order |
| running → unknown | No trustworthy outcome for an unsafe dispatched call |
| unknown → succeeded/failed | Trusted provider query supplies authoritative outcome; evidence recorded with result |
| queued/waiting/running → cancelled/unknown | Controlled Task stop joins or fences owners; unexecuted calls cancel, unsafe dispatched calls stay unknown |

`unknown` is never dispatched again, even when a query later says absent.
`approval reconcile` queries unknown calls by their original key and may settle
only authoritative success/failure. It does not revive Runs, fabricate a new
approval, or send another action. The scheduler also runs this query-only scan. The CLI prints each unknown or
reconciled action with its observed status and safe result/receipt.
A Root with any unresolved effect in its current tree reports `unknown` and keeps
its Task active. A previously delivered unknown message is a historical observation; reconciliation
does not rewrite Conversation history or claim a new user delivery.
An oversized, inconsistent or nonterminal recovery query result is inconclusive.
The executor persists `unknown` before delivering `UNKNOWN_SIDE_EFFECT` to the
Worker, so the call remains available for later authoritative reconciliation.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/tools-approvals.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/tools-approvals.md)。
