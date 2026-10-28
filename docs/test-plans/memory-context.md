# 长期记忆、召回与上下文：测试方案

[功能与验收目标](../features/memory-context.md) · [技术方案](../designs/memory-context.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-009](../features/memory-context.md#f-009) | 验证同义事实归并、明确纠正替代、证据不足保留低置信、遗忘后不可继续使用旧上下文。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |
| [F-010](../features/memory-context.md#f-010) | 验证可见范围、来源、时间、冻结恢复及索引故障不产生不完整成功结果。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-011](../features/memory-context.md#f-011) | 用长对话验证关键事实与待办保留、来源可定位及模型窗口约束。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |

## 层级与关键路径

覆盖多语同义、明确矛盾与纠正、低置信保留、独立证据晋升、遗忘与旧 owner、索引故障/并发变化、压缩后目标和待办保留。确定性存储证明与语义质量评测分别报告。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_memory_maintenance.py](../../tests/system/verify_memory_maintenance.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_recall.py](../../tests/system/verify_recall.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

- **F-009 剩余覆盖**：补语义去重、冲突识别和过时信息维护；保留证据与隐私边界，不将模型推测直接升级为事实。
- **F-011 剩余覆盖**：用外部模型评测验证长对话中的目标、约束、决策和待办保留质量；软件测试只证明上下文选择、冻结、隔离与恢复。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="recall-verification"></a>

### Verification

```sh
make build
python3 tests/system/verify_recall.py --binary bin/erii --evidence evidence/recall.json
go test -race ./internal/recall/... ./internal/memory/... ./internal/conversation/...
go test -race ./internal/agent/sqlite -run TestRecall
```

The harness owns Launch (isolated data and explicit HTTP embedding/LLM Fakes),
Doctor (profile probe/activation), Drive (CLI chat, Task, Root/Child and real Tool
calls), Evidence (both indexes, scope exclusions, provenance and requests), Cleanup
(reaped CLI children, joined HTTP thread, deleted roots). It SIGKILLs the Go process
after recall commit, changes the corpus, and proves restart reuses the old turn
while a later turn sees new data. An embedding outage proves no partial recall or
model admission; restoring the same endpoint permits recovery. The current system runner and CI select this check; repetitions require an explicit runner option. Fresh/pending-v2 and populated-v1
multipart CLI scenarios prove an 80,001-byte original remains unchanged, all bytes
reach bounded HTTP requests, nine generations/four Runs complete, and a repeated
drain is inert. The actual HTTP adapter regression also covers UTF-8 boundaries,
the maximum 128 × 64 KiB original, mid-chunk failure/recovery and vector reuse.

The success fixtures control the legal Main/Root ordering with explicit test-only
CLI barriers: Main waits after Task commit until Root freezes its first recall;
Root's model dispatch waits until Main publishes and completes. Every original
assertion and command deadline remains. Unexpected CLI failures still fail once,
without retry. A sixth scenario forces the opposite ordering through the real
CLI and HTTP adapter: both Root query embeddings wait after catch-up while Main
appends its reply. That drain must fail closed with one pending assistant vector,
no Worker recall snapshot and no Worker model request. A separate explicit caller
then recovers all original Main/Root/Child assertions without rewriting frozen
recall or Conversation originals. Failed embedding barriers cannot count as a
successful conflict reproduction. Both orderings run in every five-run series.

Contracts cover lexical/vector-only fusion, ranking/time boundaries, user/Agent
scope, provenance, independent concurrent retrieval, competing supersession,
deletion, mutation/index rollback, old-owner rejection and migration. This is SC-08
retrieval/storage evidence, not full automatic formation or a product-quality
score. Deployment model quality remains release-time Live Shadow evidence.

References: [SQLite FTS5](https://www.sqlite.org/fts5.html),
[Ollama OpenAI-compatible embeddings](https://github.com/ollama/ollama/blob/main/docs/api/openai-compatibility.mdx).

```sh
make build
python3 tests/system/verify_memory_maintenance.py --binary bin/erii --evidence evidence/memory-maintenance.json
go test -race ./internal/context ./internal/memory/... ./internal/conversation/...
go test -race ./internal/agent/sqlite -run 'TestForget|TestContextLength|TestExperience'
```

The harness owns isolated Launch, profile/Fake Doctor, CLI/Tool Drive, database and
request Evidence, and process/thread/filesystem Cleanup. It covers health/goal/
finance/learning fragments, correction, encryption, restart, privacy revocation,
long history provenance and verified Skill promotion/retirement. It does not score
full GC-01–07, Computer cases or release-time Live Shadow behavior.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。

## 工程基础重构回归

F-010/F-039：`TestEncryptedRecallReusesVectorsAndInvalidatesScopeAndForget` 统计 embedding 调用；同查询第二次仅嵌入 query，追加一条只补一条向量；Forget 和换 scope 清除工作索引，持久库无私有向量。既有 scope、敏感明文不落盘、隐私 epoch 和每轮冻结证据回归继续执行。

## 上下文装配与语义压缩回归

`TestContextPromptBytes` 与 `TestSummaryPromptBytes` 对照文案迁移前的完整模型消息 hash，覆盖 Main/Root/Child、全部能力指导、冻结项目/Skill 资源、显式 Artifact 输入及三种摘要目标大小；从空临时目录调用，验证内置文案不依赖运行目录。这是确定性的消息兼容回归，预期 hash 仅在明确调整模型输入时更新，不评价模型效果。

[internal/agent/context_test.go](../../internal/agent/context_test.go) 检查角色组合、冻结项目/Skill 来源和显式子任务；[internal/context/working_test.go](../../internal/context/working_test.go) 检查目标与未知事实保留、工具交换边界、媒体与传输预算；[internal/agent/sqlite/context_test.go](../../internal/agent/sqlite/context_test.go) 通过真实存储和共享 Loop 检查：

- Task 续跑排除 Child 私有中间轨迹，保留未知操作状态。
- Root 最终答复已接受但尚未提交 report 时暂停并续跑，保留该答复；同一边界的内部压缩摘要不作为业务答复交接。
- 摘要结果已接受但未写 checkpoint 时恢复，按 generation 复用，不重新执行历史工具。
- 摘要继续使用原 ModelSnapshot、没有 Tools、不流式交付用户；原文通过私有 Artifact 可读取。
- 同 Task 的 Child 无法读取 Parent 私有摘要原文；新 Main 通过业务接续显式授权读取上一 Main 原文。
- 项目/Skill prompt 资源复制及冻结，读取当前资源不改写旧 capability。

这些用例不评价真实模型总结质量。现有取消、Forget、审批、Worker 树和 legacy 请求恢复测试继续适用。
