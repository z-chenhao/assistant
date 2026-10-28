# 长期记忆、召回与上下文

形成和管理个人记忆，检索历史，并维护每轮可控上下文。

[需求主索引](../feature-map.md) · [技术方案](../designs/memory-context.md) · [测试方案](../test-plans/memory-context.md)

## 功能说明

后台从用户事实与反馈形成记忆，能处理纠正、重复和过时信息。

每轮或按需检索可访问的 Memory 和原始 Conversation。

长对话保留目标、决策、约束和待办，省略内容可追溯。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-009"></a>

### F-009 · 自动记忆形成与语义归并

**需求**：后台从用户事实与反馈形成记忆，能处理纠正、重复和过时信息。

**当前实现**：remember/forget、隐私屏障、后台维护已实现；提取与稳定性判定采用规则，归并主要按指纹。

**边界与剩余工作**：补语义去重、冲突识别和过时信息维护；保留证据与隐私边界，不将模型推测直接升级为事实。

**验收目标**：验证同义事实归并、明确纠正替代、证据不足保留低置信、遗忘后不可继续使用旧上下文。

<a id="f-010"></a>

### F-010 · 按作用域召回记忆与对话

**需求**：每轮或按需检索可访问的 Memory 和原始 Conversation。

**当前实现**：显式 embedding 配置下，全文与向量结果融合；召回结果按轮冻结并带来源。

**边界与剩余工作**：未配置 embedding 时不可用；不宣称大规模 ANN 性能或中文语义质量已验证。

**验收目标**：验证可见范围、来源、时间、冻结恢复及索引故障不产生不完整成功结果。

<a id="f-011"></a>

### F-011 · 长期上下文压缩

**需求**：长对话保留目标、决策、约束和待办，省略内容可追溯。

**当前实现**：有分级压缩、原文引用及持久记录；当前总结是有界摘录。

**边界与剩余工作**：补面向任务与对话的语义整理；不得改写原文或在旧 Run 中静默变更冻结请求。

**验收目标**：用长对话验证关键事实与待办保留、来源可定位及模型窗口约束。

## 如何使用

在配置中提供 `memory.encryption_key_ref` 开启记忆维护，提供显式 embedding 配置开启搜索与自动召回。加密 Memory 当前要求本地 loopback embedding 端点。

用户可说“记住这个偏好”“之前的信息改为……”或“忘记这类信息”。CLI `erii memory status` 查看维护状态；`erii memory manage REQUEST_KEY` 接受 JSON 控制请求。压缩自动发生。

## 详细操作参考

- [记忆维护操作](#memory-maintenance-memory-context-and-verified-experience)
- [召回配置与协议](../designs/memory-context.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="recall-configuration-and-privacy"></a>

### Configuration and privacy

```yaml
embedding:
  provider: openai_compatible
  endpoint: http://127.0.0.1:11434/v1
  model: nomic-embed-text
  dimensions: 768
  # api_key_ref: reference to a credential already in SecretStore
```

This is an explicit deployment example, not a bundled/downloaded/default model.
Serve the model separately. Authorized record text and queries go to exactly this
endpoint; choose a local server for local-only processing. Remote endpoints require
HTTPS. Credentials are resolved from SecretStore only. Redirects, malformed
responses, dimension mismatch, zero/nonfinite vectors and provider failures fail
closed with sanitized errors. There is no provider switch or FTS-only success.

Without embedding configuration, ordinary chat remains available and recall is
absent. This state does not constitute configured M6 vector recall. Adding it
affects new Runs only. Existing capability snapshots gain no new Tool or automatic
recall. Removing/changing it rejects a Run that already has recall capability.
Endpoint/model/dimension are hashed into the capability; the model sees neither
credentials nor endpoint strings. Credential rotation does not change vector space.

<a id="memory-maintenance-memory-context-and-verified-experience"></a>

## Memory, context and verified experience

Enable maintenance explicitly with `memory.encryption_key_ref`, an existing
SecretStore reference to a 32-byte AES key. Store the key in the native credential
store or use an explicit `env:NAME` reference. YAML contains only the reference.
Keep the same key across process restarts and backups; key loss fails closed.
There is no implicit credential backend or per-record approval.

```yaml
memory:
  encryption_key_ref: env:ERII_MEMORY_KEY
context:
  threshold_percentage: 0.75
  recent_messages: 6
  summary_bytes: 2048
```

The Curator runs for the lifetime of TUI/default service/foreground `chat work`,
including idle service time, and is joined before databases close on shutdown. `memory work` drains a bounded serial batch for maintenance/diagnosis;
`memory status` shows durable receipts, applied/rejected state and sanitized error
codes. Transient failures stay pending with internal retry backoff. Permanent
conflicts are recorded as rejected, not retried forever or announced to the user.
The shared Event Outbox provides wakeups; the SQLite journal owns acceptance and
recovery. A crash between the Memory commit and journal acknowledgement reuses
the same event and its Memory receipt.

Main always discovers `builtin__memory__manage` with `remember`/`forget`. The Host resolves
scope and provenance. Without `memory.encryption_key_ref`, it returns
`PROVIDER_UNAVAILABLE` without accepting a mutation or changing privacy state.
Main and Workers always discover `builtin__memory__search`; it requires an
explicit embedding configuration. Without that configuration, search returns
`PROVIDER_UNAVAILABLE`, and automatic recall is disabled. Tool descriptions name
the missing configuration. No provider or encryption credential is implicit. Workers retain only read-only search; their results become
Curator source observations. The authenticated local equivalent is:

```sh
# Each command reads one JSON object on stdin. Reuse REQUEST_KEY to recover a receipt.
bin/erii -config config.yaml memory manage REQUEST_KEY
bin/erii -config config.yaml memory work
bin/erii -config config.yaml memory status
```

Remember input: `{"operation":"remember","content":"I prefer unsweetened tea"}`.
Correction adds `"supersedes":"memory:REF"` from an authorized search result.
Forget input: `{"operation":"forget","memory_ref":"memory:REF"}`.
Acceptance is durable intent, not a claim that asynchronous merging has finished.
For a durable opt-out, use `{"operation":"forget","category":"health"}` (also
finance, identity, relationships, goals, learning, preferences, facts, or all).
This tombstones current matching records and blocks future automatic extraction
in that category. `remember` with a category resumes future extraction; it does
not restore tombstones. Explicit remember of one fact can override an opt-out for
that fact. Event-frozen deletion targets and idempotent policy receipts prevent a
replayed old policy event from deleting fresh evidence or undoing a later choice.
Unknown/cross-scope references are rejected. Reusing a control key with changed
parameters conflicts. Memory writes never use Security Approval.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/memory-context.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/memory-context.md)。
