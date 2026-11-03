# MCP 与真实外部能力：测试方案

[功能与验收目标](../features/integrations.md) · [技术方案](../designs/integrations.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导特定版本已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-024](../features/integrations.md#f-024) | 真实协议验证发现/调用、漂移拒绝、取消回收与外部效果审批。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-025](../features/integrations.md#f-025) | 在明确服务上完成真实读取/授权动作/失效恢复；无凭据泄漏，审批及 unknown 语义保持一致。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |
| [F-042](../features/integrations.md#f-042) | 受控来源重复/乱序、补读、重启、撤权、自身回流、stop/Forget fencing 与真实故障状态；完整扫描后才提交事件和成功边界。 | 已接通监听→SQLite→CLI；Main/Task/飞书接续、真实 GitHub 权限及历史保留边界需后续入口与 Live 验证 |

## 层级与关键路径

本地 stdio 测试协议与沙箱、允许列表及 schema 漂移；远程能力落地时增加真实认证生命周期、服务失败与副作用未知测试。线上服务质量不由本地 MCP Fake 证明。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 覆盖用途与限制 |
| --- | --- |
| [tests/system/verify_capabilities.py](../../tests/system/verify_capabilities.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 |
| [internal/observer/github/client_test.go](../../internal/observer/github/client_test.go) | 实际 HTTP 协议 peer 驱动分页、认证头、自身份排除、评论更新身份、review/CI、恶意 Link/redirect、撤权及异常响应；不需要真实 PAT |
| [internal/agent/sqlite/github_test.go](../../internal/agent/sqlite/github_test.go) | 真实 SQLite 去重、观察/Outbox 原子接受、故障边界、重启补读、repository ID pin、并发/停止/Forget fencing 与当前 epoch 读取 |
| [cmd/steward/github_test.go](../../cmd/steward/github_test.go) | 真实 CLI 调度与重开 data root，在执行依赖不可用时保持状态/停止/恢复/配置移除可用，缺凭据失败可诊断 |

- **F-025 剩余覆盖**：真实 PAT 权限、到期/更换与 GitHub 保留历史需 Live；OAuth、授权写动作、其他服务及对应入口仍未实现。
- **F-042 剩余覆盖**：来源→Main/Task→飞书的关联消费尚未实现；不把本轮持久观察当作该闭环完成。真实来源删除、旧 head、中间修改与平台历史保留不由协议 peer 证明。

## 详细测试入口与证明边界

本功能协议在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="capabilities-reproduction-and-evidence"></a>

### Reproduction and evidence

```sh
make build
python3 tests/system/verify_capabilities.py --binary bin/steward --evidence evidence/capabilities.json
go test -race ./internal/tool/... ./internal/skill ./internal/computer/... ./internal/config
```

The harness launches private data/provider/Skills/checkout roots, doctors an
explicit Fake model, drives ordinary chat/Task/approval CLI commands, reads SQLite
and file hashes, kills after a real edit before result commit, restarts, and
cleans every child and disposable root. The system runner selects it; repeat count is explicit.
The coding fixture starts with a real failing `--status paused` regression,
edits only its CLI through a Worker, then passes its real race tests. Audit reads
the deliberately defective retry ordering without changing any byte. Go
regressions additionally probe host credential reads/writes, environment/network
denial, link escapes, catalog drift and executable Git config.

This extends the M3 Tool/Approval/Artifact fragments in `verify_tools.py` and
`verify_artifacts.py`; it does not assign a semantic Golden score. GC-19 is a
coding prerequisite here. This fixture marks GC-20 Child/recovery and full
GC-19/20 Durable cases `not_applicable`; the [Core gate](product.md) covers
their additional mechanics. Production Computer has [separate verification](../designs/computer-use.md).
The GC-11 checkpoint now drives both original turns in six locked variants,
reads all three actual provider quotes with services and user weights, preserves
oral-cashback uncertainty, delivers two scripted replies and reopens both
comparison Artifacts. It also rejects purchase/approval/Task effects and proves
duplicate-input state equality. The same five-round capability gate includes
every variant. Its deterministic scripted model does not establish live-model
reasoning quality; see [release evidence and remaining obligations](product.md).

The local MCP path needs no new domain states: schema v12 already freezes arbitrary
ToolSpecs, Skill set hashes, policy and invocation outcomes. GitHub observation
adds schema v33 watch/observation records separately and does not extend ToolSpecs. Empty/prior schema
migration tests remain in the existing SQLite gate. Old capabilities without
these optional fields keep their old tools; enabling an adapter never expands an
already frozen Run. Changing a configured scope fails old-Run selection closed.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
