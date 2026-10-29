# Skills 与经验沉淀

按需使用方法库，并把被验证的方法沉淀为可复用经验。

[需求主索引](../feature-map.md) · [技术方案](../designs/skills-experience.md) · [测试方案](../test-plans/skills-experience.md)

## 功能说明

Agent 可发现可用方法，再按需读取正文和资源。

用户纠正、恢复失败或重复成功能进入候选、验证和晋升流程。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-022"></a>

### F-022 · 受版本控制的 Skill 读取

**需求**：Agent 可发现可用方法，再按需读取正文和资源。

**当前实现**：受控目录 list/view 与全目录指纹冻结已实现。

**边界与剩余工作**：普通 Main/Worker 不管理目录；变化不热替换旧 Run 的冻结 Skill。

**验收目标**：验证资源范围、版本漂移拒绝、二进制资源引用与上下文按需加载。

<a id="f-023"></a>

### F-023 · 执行后的自动经验闭环

**需求**：用户纠正、恢复失败或重复成功能进入候选、验证和晋升流程。

**当前实现**：可信 Reviewer 有 propose/verify/promote/reject/retire；目前由 CLI 显式驱动，缺自动触发与编排。

**边界与剩余工作**：接入纠正/结果事件与可信 Reviewer；当前验证证据限于经审批的 bash 精确测试命令，需明确其他方法的证据标准。

**验收目标**：从真实纠正或执行结果自动形成候选、按证据验证并可再次调用；普通 Worker 仍不能随意改 Skill。

## 如何使用

配置受控 Skills 目录后，Agent 按需 list/view 使用已有方法。操作员可经 `collie skill manage` 从 stdin 提交 Reviewer 请求。

当前不会仅因一次任务成功就自动形成可复用 Skill；F-023 的触发、证据和晋升编排尚未闭环。

## 详细操作参考

- [MCP、Skills 与工作区约定](../designs/integrations.md)
- [记忆维护操作](memory-context.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

The Experience Reviewer is a trusted local control workflow. Ordinary Main and
Worker Skill capabilities remain list/view. `skill manage` reads a JSON operation:
`propose`, `verify`, `promote`, `reject`, `retire` or `list`. A proposal includes
`name`, `description`, a short reusable `procedure`, `reason` (`correction`,
`recovered_failure`, `success`) and accessible `source_refs`.

Verification supplies a `candidate_ref`, current `version` and `verification_ref`
(`invocation:REF`). It must resolve to a successful real builtin__bash invocation with the exact accepted command
in a completed authorized Run, reporting exit_code=0, timed_out=false and untruncated output for `go test -race ./...`.
Ordinary success requires two independent Runs; correction/recovered-failure
candidates require one. Correction candidates require an original user source;
recovered-failure candidates require a failed Run source. Later independent
verification Runs add their own provenance without rewriting the candidate.
The procedure is immutable and binds all verification
records. Promotion is impossible before that gate. Publication is staged, protects
existing files and is recoverable if the process dies before the status commit.
Retirement retains an audit copy. New catalog versions affect new Runs; existing
frozen resources fail closed on drift. Raw logs and secrets are rejected as Skill
procedures. Mechanical evidence validates the test provenance, not the semantic
quality of the written method; model-quality review remains a release check.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/skills-experience.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/skills-experience.md)。
